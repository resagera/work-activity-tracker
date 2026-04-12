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

	"work-activity-tracker/internal/activity"
	"work-activity-tracker/internal/config"
	"work-activity-tracker/internal/history"
	"work-activity-tracker/internal/inactivity"
	"work-activity-tracker/internal/logging"
	"work-activity-tracker/internal/platform"
	"work-activity-tracker/internal/telegram"
	"work-activity-tracker/internal/tracker"
)

type App struct {
	cfg                   config.Config
	env                   platform.Environment
	tracker               *tracker.Tracker
	history               *history.Store
	activityTypes         *activity.Store
	customActivityTypes   []activity.TypeDefinition
	inactivityTypes       *inactivity.Store
	customInactivityTypes []inactivity.TypeDefinition

	continuedFromHistory bool
}

func New(cfg config.Config, env platform.Environment) *App {
	return &App{
		cfg:             cfg,
		env:             env,
		tracker:         tracker.New(cfg, env.SendDesktopNotification),
		history:         history.New(cfg.HistoryFile),
		activityTypes:   activity.New(cfg.ActivityTypesFile),
		inactivityTypes: inactivity.New(cfg.InactivityTypesFile),
	}
}

func (a *App) Run(ctx context.Context) error {
	printConfig(a.cfg)
	if err := a.loadActivityTypes(); err != nil {
		return fmt.Errorf("load activity types: %w", err)
	}
	if err := a.loadInactivityTypes(); err != nil {
		return fmt.Errorf("load inactivity types: %w", err)
	}
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
		writeJSON(w, http.StatusOK, a.Summary())
	})

	mux.HandleFunc("/window-triggers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"title_triggers": a.cfg.ExcludedWindowTitleSubstrings,
			"app_triggers":   a.cfg.ExcludedAppSubstrings,
		})
	})

	mux.HandleFunc("/history", func(w http.ResponseWriter, r *http.Request) {
		records, err := a.history.LoadAll()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		for i := range records {
			records[i].Periods = a.enrichHistoryPeriods(records[i].Periods)
		}
		writeJSON(w, http.StatusOK, records)
	})

	mux.HandleFunc("/activity-types", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"types":         a.ActivityTypeDefinitions(),
			"default_type":  activity.DefaultType(a.cfg.DefaultActivityType),
			"current_type":  a.Summary().CurrentActivityType,
			"current_color": a.Summary().CurrentActivityColor,
		})
	})

	mux.HandleFunc("/activity-types/add", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		color := r.URL.Query().Get("color")
		types, err := a.AddActivityTypeWithColor(name, color)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"types": types})
	})

	mux.HandleFunc("/activity-type/set", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if err := a.SetCurrentActivityType(name); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, a.Summary())
	})

	mux.HandleFunc("/activity-type/color", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		color := r.URL.Query().Get("color")
		types, err := a.SetActivityTypeColor(name, color)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"types": types})
	})

	mux.HandleFunc("/inactivity-types", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"types":         a.InactivityTypeDefinitions(),
			"current_type":  a.Summary().CurrentInactivityType,
			"current_color": a.Summary().CurrentInactivityColor,
		})
	})

	mux.HandleFunc("/inactivity-types/add", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		color := r.URL.Query().Get("color")
		types, err := a.AddInactivityTypeWithColor(name, color)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"types": types})
	})

	mux.HandleFunc("/inactivity-type/set", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if err := a.SetCurrentInactivityType(name); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, a.Summary())
	})

	mux.HandleFunc("/inactivity-type/color", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		color := r.URL.Query().Get("color")
		types, err := a.SetInactivityTypeColor(name, color)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"types": types})
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
		writeJSON(w, http.StatusOK, a.Summary())
	})

	mux.HandleFunc("/subtract", func(w http.ResponseWriter, r *http.Request) {
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

		a.MoveActiveToInactive(time.Duration(minutes)*time.Minute, "http api")
		writeJSON(w, http.StatusOK, a.Summary())
	})

	mux.HandleFunc("/pause", func(w http.ResponseWriter, r *http.Request) {
		a.SetManualPause(true)
		writeJSON(w, http.StatusOK, a.Summary())
	})

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		s := a.Summary()
		if !s.Started || s.Ended {
			writeJSON(w, http.StatusOK, a.StartNewDay("http api"))
			return
		}
		a.SetManualPause(false)
		writeJSON(w, http.StatusOK, a.Summary())
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
			info, err := a.env.ActiveWindowInfo(a.cfg.ExcludedWindowTitleSubstrings, a.cfg.ExcludedAppSubstrings)
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
	s := a.tracker.Summary()
	s.Periods = a.enrichHistoryPeriods(s.Periods)
	s.CurrentActivityColor = activity.FindColor(a.ActivityTypeDefinitions(), s.CurrentActivityType)
	s.CurrentInactivityColor = inactivity.FindColor(a.InactivityTypeDefinitions(), s.CurrentInactivityType)
	return s
}

func (a *App) AddTime(d time.Duration, source string) {
	a.tracker.AddTime(d, source)
}

func (a *App) MoveActiveToInactive(d time.Duration, source string) {
	a.tracker.MoveActiveToInactive(d, source)
}

func (a *App) AllInactivityTypes() []string {
	return inactivity.Names(a.InactivityTypeDefinitions())
}

func (a *App) AllActivityTypes() []string {
	items := append([]activity.TypeDefinition{}, a.customActivityTypes...)
	defaultType := activity.DefaultType(a.cfg.DefaultActivityType)
	if !contains(activity.Names(items), defaultType) {
		items = append(items, activity.TypeDefinition{Name: defaultType})
	}
	return activity.Names(activity.Merge(items))
}

func (a *App) AddActivityType(name string) ([]string, error) {
	types, err := a.activityTypes.Add(name, "")
	if err != nil {
		return nil, err
	}
	custom, err := a.activityTypes.LoadAll()
	if err == nil {
		a.customActivityTypes = custom
	}
	return activity.Names(types), nil
}

func (a *App) AddActivityTypeWithColor(name, color string) ([]activity.TypeDefinition, error) {
	types, err := a.activityTypes.Add(name, color)
	if err != nil {
		return nil, err
	}
	custom, err := a.activityTypes.LoadAll()
	if err == nil {
		a.customActivityTypes = custom
	}
	return types, nil
}

func (a *App) ActivityTypeDefinitions() []activity.TypeDefinition {
	items := append([]activity.TypeDefinition{}, a.customActivityTypes...)
	defaultType := activity.DefaultType(a.cfg.DefaultActivityType)
	if !contains(activity.Names(items), defaultType) {
		items = append(items, activity.TypeDefinition{Name: defaultType})
	}
	return activity.Merge(items)
}

func (a *App) SetActivityTypeColor(name, color string) ([]activity.TypeDefinition, error) {
	types, err := a.activityTypes.SetColor(name, color)
	if err != nil {
		return nil, err
	}
	custom, err := a.activityTypes.LoadAll()
	if err == nil {
		a.customActivityTypes = custom
	}
	return types, nil
}

func (a *App) SetCurrentActivityType(name string) error {
	name = activity.NormalizeName(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if !contains(a.AllActivityTypes(), name) {
		return fmt.Errorf("unknown activity type: %s", name)
	}
	return a.tracker.SetCurrentActivityType(name)
}

func (a *App) AddInactivityType(name string) ([]string, error) {
	types, err := a.inactivityTypes.Add(name, "")
	if err != nil {
		return nil, err
	}
	custom, err := a.inactivityTypes.LoadAll()
	if err == nil {
		a.customInactivityTypes = custom
	}
	return inactivity.Names(types), nil
}

func (a *App) AddInactivityTypeWithColor(name, color string) ([]inactivity.TypeDefinition, error) {
	types, err := a.inactivityTypes.Add(name, color)
	if err != nil {
		return nil, err
	}
	custom, err := a.inactivityTypes.LoadAll()
	if err == nil {
		a.customInactivityTypes = custom
	}
	return types, nil
}

func (a *App) InactivityTypeDefinitions() []inactivity.TypeDefinition {
	return inactivity.Merge(a.customInactivityTypes)
}

func (a *App) SetInactivityTypeColor(name, color string) ([]inactivity.TypeDefinition, error) {
	types, err := a.inactivityTypes.SetColor(name, color)
	if err != nil {
		return nil, err
	}
	custom, err := a.inactivityTypes.LoadAll()
	if err == nil {
		a.customInactivityTypes = custom
	}
	return types, nil
}

func (a *App) SetCurrentInactivityType(name string) error {
	name = inactivity.NormalizeName(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if !contains(a.AllInactivityTypes(), name) {
		return fmt.Errorf("unknown inactivity type: %s", name)
	}
	return a.tracker.SetCurrentInactivityType(name)
}

func (a *App) SetManualPause(paused bool) {
	a.tracker.SetManualPause(paused)
}

func (a *App) StartNewDay(reason string) tracker.SessionSummary {
	a.continuedFromHistory = false
	a.tracker.SetResumeRecord(nil)
	a.tracker.StartNewDay(reason)
	return a.Summary()
}

func (a *App) ContinueDay(reason string) tracker.SessionSummary {
	a.continuedFromHistory = true
	a.tracker.ContinueDay(reason)
	return a.Summary()
}

func (a *App) EndSession(reason string) tracker.SessionSummary {
	before := a.tracker.Summary()
	summary := a.tracker.EndSession(reason)
	if before.Started && !before.Ended && summary.Started && summary.Ended {
		windowCount, topWindows, appCount, topApps, metadata := a.tracker.ActivityStats()
		record := history.SessionRecord{
			Version:          3,
			SessionStartedAt: summary.SessionStartedAt,
			SessionEndedAt:   time.Now(),
			TotalActive:      int64(summary.TotalActive),
			TotalInactive:    int64(summary.TotalInactive),
			TotalAdded:       int64(summary.TotalAdded),
			WindowCount:      windowCount,
			AppCount:         appCount,
			TopWindows:       topWindows,
			TopApps:          topApps,
			Periods:          a.enrichHistoryPeriods(a.tracker.HistoryPeriods()),
			Metadata:         metadata,
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
		return a.Summary()
	}
	return a.Summary()
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

func (a *App) loadActivityTypes() error {
	items, err := a.activityTypes.LoadAll()
	if err != nil {
		return err
	}
	a.customActivityTypes = items
	return nil
}

func (a *App) loadInactivityTypes() error {
	items, err := a.inactivityTypes.LoadAll()
	if err != nil {
		return err
	}
	a.customInactivityTypes = items
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

func (a *App) enrichHistoryPeriods(periods []history.SessionPeriod) []history.SessionPeriod {
	out := make([]history.SessionPeriod, 0, len(periods))
	activityColors := make(map[string]string, len(a.ActivityTypeDefinitions()))
	for _, item := range a.ActivityTypeDefinitions() {
		activityColors[item.Name] = item.Color
	}
	inactivityColors := make(map[string]string, len(a.InactivityTypeDefinitions()))
	for _, item := range a.InactivityTypeDefinitions() {
		inactivityColors[item.Name] = item.Color
	}

	for _, period := range periods {
		if strings.TrimSpace(period.Color) == "" {
			switch period.Kind {
			case "activity":
				period.Color = activityColors[period.Type]
			case "inactivity":
				period.Color = inactivityColors[period.Type]
			}
		}
		out = append(out, period)
	}
	return out
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
