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
	"work-activity-tracker/internal/history"
	"work-activity-tracker/internal/logging"
	"work-activity-tracker/internal/platform"
	"work-activity-tracker/internal/telegram"
	"work-activity-tracker/internal/tracker"
)

type App struct {
	cfg     config.Config
	env     platform.Environment
	tracker *tracker.Tracker
	history *history.Store

	continuedFromHistory bool
}

func New(cfg config.Config, env platform.Environment) *App {
	return &App{
		cfg:     cfg,
		env:     env,
		tracker: tracker.New(cfg, env.SendDesktopNotification),
		history: history.New(cfg.HistoryFile),
	}
}

func (a *App) Run(ctx context.Context) error {
	printConfig(a.cfg)
	if err := a.loadResumeCandidate(); err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	if a.cfg.TelegramToken != "" && a.cfg.TelegramChatID != 0 {
		notifier, err := telegram.New(a.cfg.TelegramToken, a.cfg.TelegramChatID, a)
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
		a.StartNewDay("старт программы")
	} else {
		a.tracker.Logf("📅 Автостарт дня отключен. Ожидание команды на начало нового дня")
	}

	<-ctx.Done()
	summary := a.EndSession("остановка программы")
	printSessionSummary(summary)
	return nil
}

func (a *App) runHTTP(ctx context.Context) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(webUIHTML))
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/history", func(w http.ResponseWriter, r *http.Request) {
		records, err := a.history.LoadAll()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, records)
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

		a.AddTime(time.Duration(minutes)*time.Minute, "http api")
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/pause", func(w http.ResponseWriter, r *http.Request) {
		a.SetManualPause(true)
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		s := a.tracker.Summary()
		if !s.Started || s.Ended {
			writeJSON(w, http.StatusOK, a.StartNewDay("http api"))
			return
		}
		a.SetManualPause(false)
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/new-day", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.StartNewDay("http api"))
	})

	mux.HandleFunc("/continue-day", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.ContinueDay("http api"))
	})

	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.EndSession("http api"))
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
	a.tracker.Logf("🖥 Web UI: http://127.0.0.1:%d/", a.cfg.HTTPPort)

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
	logging.Stdoutf("CONFIG:\n%s\n", string(b))
}

func printSessionSummary(s tracker.SessionSummary) {
	startedAt := "-"
	if s.Started {
		startedAt = s.SessionStartedAt.Format(time.RFC3339)
	}

	logging.Stdoutf(
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

func (a *App) Summary() tracker.SessionSummary {
	return a.tracker.Summary()
}

func (a *App) AddTime(d time.Duration, source string) {
	a.tracker.AddTime(d, source)
}

func (a *App) SetManualPause(paused bool) {
	a.tracker.SetManualPause(paused)
}

func (a *App) StartNewDay(reason string) tracker.SessionSummary {
	a.continuedFromHistory = false
	a.tracker.SetResumeRecord(nil)
	return a.tracker.StartNewDay(reason)
}

func (a *App) ContinueDay(reason string) tracker.SessionSummary {
	a.continuedFromHistory = true
	return a.tracker.ContinueDay(reason)
}

func (a *App) EndSession(reason string) tracker.SessionSummary {
	before := a.tracker.Summary()
	summary := a.tracker.EndSession(reason)
	if before.Started && !before.Ended && summary.Started && summary.Ended {
		record := history.SessionRecord{
			SessionStartedAt: summary.SessionStartedAt,
			SessionEndedAt:   time.Now(),
			TotalActive:      int64(summary.TotalActive),
			TotalInactive:    int64(summary.TotalInactive),
			TotalAdded:       int64(summary.TotalAdded),
		}
		if err := a.history.Save(record, a.continuedFromHistory); err != nil {
			a.tracker.Logf("history save error: %v", err)
		}
		a.continuedFromHistory = false
		if canContinueDay(&record, time.Now()) {
			a.tracker.SetResumeRecord(&record)
		} else {
			a.tracker.SetResumeRecord(nil)
		}
		return a.tracker.Summary()
	}
	return summary
}

func (a *App) loadResumeCandidate() error {
	record, err := a.history.Last()
	if err != nil {
		return err
	}
	if canContinueDay(record, time.Now()) {
		a.tracker.SetResumeRecord(record)
	}
	return nil
}

func canContinueDay(record *history.SessionRecord, now time.Time) bool {
	if record == nil {
		return false
	}

	startLocal := record.SessionStartedAt.In(now.Location())
	nowLocal := now.In(now.Location())
	sameDay := startLocal.Year() == nowLocal.Year() && startLocal.YearDay() == nowLocal.YearDay()
	if sameDay {
		return true
	}

	return now.Sub(record.SessionEndedAt) < 6*time.Hour
}
