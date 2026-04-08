package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"work-activity-tracker/internal/config"
	"work-activity-tracker/internal/platform"
	"work-activity-tracker/internal/telegram"
	"work-activity-tracker/internal/tracker"
)

type App struct {
	cfg     config.Config
	env     platform.Environment
	tracker *tracker.Tracker
}

func New(cfg config.Config, env platform.Environment) *App {
	return &App{
		cfg:     cfg,
		env:     env,
		tracker: tracker.New(cfg, env.SendDesktopNotification),
	}
}

func (a *App) Run(ctx context.Context) error {
	printConfig(a.cfg)

	if a.cfg.TelegramToken != "" && a.cfg.TelegramChatID != 0 {
		notifier, err := telegram.New(a.cfg.TelegramToken, a.cfg.TelegramChatID, a.tracker)
		if err != nil {
			return fmt.Errorf("telegram init: %w", err)
		}
		a.tracker.SetNotifier(notifier)
		notifier.SendOrReplaceControls()
		go notifier.Run(ctx)
	}

	if a.cfg.HTTPPort > 0 {
		go a.runHTTP(ctx)
	}

	go a.runIdlePolling(ctx)
	go a.runLockPolling(ctx)
	go a.runLockSignalWatcher(ctx)
	go a.runActiveWindowPolling(ctx)

	if a.cfg.AutoStartDay {
		a.tracker.StartNewDay("старт программы")
	} else {
		a.tracker.Logf("📅 Автостарт дня отключен. Ожидание команды на начало нового дня")
	}

	<-ctx.Done()
	summary := a.tracker.EndSession("остановка программы")
	printSessionSummary(summary)
	return nil
}

func (a *App) runHTTP(ctx context.Context) {
	mux := http.NewServeMux()

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/add", func(w http.ResponseWriter, r *http.Request) {
		minutesStr := r.URL.Query().Get("minutes")
		if minutesStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "minutes is required"})
			return
		}

		minutes, err := strconv.Atoi(minutesStr)
		if err != nil || minutes <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "minutes must be positive integer"})
			return
		}

		a.tracker.AddTime(time.Duration(minutes)*time.Minute, "http api")
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/pause", func(w http.ResponseWriter, r *http.Request) {
		a.tracker.SetManualPause(true)
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		s := a.tracker.Summary()
		if !s.Started || s.Ended {
			writeJSON(w, http.StatusOK, a.tracker.StartNewDay("http api"))
			return
		}
		a.tracker.SetManualPause(false)
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/new-day", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.tracker.StartNewDay("http api"))
	})

	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.tracker.EndSession("http api"))
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.cfg.HTTPPort),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	a.tracker.Logf("🌐 HTTP API запущен на :%d", a.cfg.HTTPPort)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		a.tracker.Logf("HTTP API ошибка: %v", err)
	}
}

func (a *App) runIdlePolling(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.PollInterval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			idle, err := a.env.IdleDuration()
			if err != nil {
				a.tracker.Logf("idle check error: %v", err)
				continue
			}
			a.tracker.HandleActivity(idle)
		}
	}
}

func (a *App) runLockPolling(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.PollInterval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			locked, err := a.env.IsScreenLocked()
			if err != nil {
				a.tracker.Logf("lock check error: %v", err)
				continue
			}
			a.tracker.SetLocked(locked)
		}
	}
}

func (a *App) runLockSignalWatcher(ctx context.Context) {
	if err := a.env.WatchScreenLock(ctx, a.tracker.SetLocked); err != nil {
		a.tracker.Logf("lock signal watcher error: %v", err)
	}
}

func (a *App) runActiveWindowPolling(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.PollInterval.Duration)
	defer ticker.Stop()

	var lastFingerprint string

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := a.env.ActiveWindowInfo(a.cfg.ExcludedWindowSubstrings)
			if err != nil {
				a.tracker.Logf("active window check error: %v", err)
				continue
			}

			fingerprint := strings.Join([]string{
				info.WindowID,
				info.Title,
				info.GTKApplicationID,
				info.KDEDesktopFile,
				info.WMClass,
				strconv.FormatBool(info.BlockedByRule),
				info.MatchedField,
				info.MatchedSubstring,
			}, "\x00")

			if fingerprint != lastFingerprint {
				lastFingerprint = fingerprint
				a.tracker.SetActiveWindowInfo(info)
			}
		}
	}
}

func printConfig(cfg config.Config) {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Printf("CONFIG:\n%s\n", string(b))
}

func printSessionSummary(s tracker.SessionSummary) {
	startedAt := "-"
	if s.Started {
		startedAt = s.SessionStartedAt.Format(time.RFC3339)
	}

	fmt.Printf(
		"SESSION SUMMARY:\n  started_at: %s\n  state: %s\n  active: %s\n  inactive: %s\n  added: %s\n",
		startedAt,
		tracker.StateText(s),
		tracker.FormatDuration(s.TotalActive),
		tracker.FormatDuration(s.TotalInactive),
		tracker.FormatDuration(s.TotalAdded),
	)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
