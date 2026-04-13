package tracker

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"work-activity-tracker/internal/activity"
	"work-activity-tracker/internal/config"
	"work-activity-tracker/internal/history"
	"work-activity-tracker/internal/inactivity"
	"work-activity-tracker/internal/platform"
)

type Notifier interface {
	SendLog(text string)
	RefreshControls()
}

type SessionSummary struct {
	Started                bool                    `json:"started"`
	CanContinueDay         bool                    `json:"can_continue_day"`
	SessionName            string                  `json:"session_name"`
	SessionStartedAt       time.Time               `json:"session_started_at"`
	TotalActive            time.Duration           `json:"total_active"`
	TotalInactive          time.Duration           `json:"total_inactive"`
	TotalAdded             time.Duration           `json:"total_added"`
	WindowCount            int                     `json:"window_count,omitempty"`
	AppCount               int                     `json:"app_count,omitempty"`
	WindowStats            []history.ActivityStat  `json:"window_stats,omitempty"`
	AppStats               []history.ActivityStat  `json:"app_stats,omitempty"`
	Periods                []history.SessionPeriod `json:"periods,omitempty"`
	CurrentActivityType    string                  `json:"current_activity_type"`
	CurrentActivityColor   string                  `json:"current_activity_color"`
	CurrentInactivityType  string                  `json:"current_inactivity_type"`
	CurrentInactivityColor string                  `json:"current_inactivity_color"`

	Running         bool      `json:"running"`
	PausedManually  bool      `json:"paused_manually"`
	Locked          bool      `json:"locked"`
	BlockedByWindow bool      `json:"blocked_by_window"`
	LastStateChange time.Time `json:"last_state_change"`
	Ended           bool      `json:"ended"`

	Window platform.WindowInfo `json:"window"`
}

type Tracker struct {
	mu sync.Mutex

	cfg config.Config

	started          bool
	sessionStartedAt time.Time
	lastStateAt      time.Time

	running       bool
	workStartedAt time.Time
	activeTotal   time.Duration

	inactiveStartedAt time.Time
	inactiveTotal     time.Duration

	manualAdded time.Duration

	pausedManually bool
	locked         bool
	warned         bool
	ended          bool

	blockedByWindow bool
	windowInfo      platform.WindowInfo

	notifier Notifier
	notifyFn func(title, body string) error

	resumeRecord        *history.SessionRecord
	sessionName         string
	currentActivityType string
	manualInactiveType  string
	periods             []history.SessionPeriod
	currentPeriod       *history.SessionPeriod
	windowActive        map[string]time.Duration
	appActive           map[string]time.Duration
	activeWindowAt      time.Time
}

func New(cfg config.Config, notifyFn func(title, body string) error) *Tracker {
	now := time.Now()
	return &Tracker{
		cfg:                 cfg,
		lastStateAt:         now,
		notifyFn:            notifyFn,
		currentActivityType: activity.DefaultType(cfg.DefaultActivityType),
		windowActive:        map[string]time.Duration{},
		appActive:           map[string]time.Duration{},
	}
}

func (t *Tracker) SetNotifier(n Notifier) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.notifier = n
}

func (t *Tracker) Summary() SessionSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.summaryLocked(time.Now())
}

func (t *Tracker) SetResumeRecord(record *history.SessionRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resumeRecord = record
}

func (t *Tracker) Logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Println(msg)

	notifier := t.getNotifier()
	if notifier != nil {
		notifier.SendLog(msg)
	}
}

func (t *Tracker) StartWork(reason string) {
	var msg string
	var notifier Notifier

	t.mu.Lock()
	if !t.started || t.ended || t.running || t.pausedManually || t.locked || t.blockedByWindow {
		t.mu.Unlock()
		return
	}

	now := time.Now()
	t.closeInactiveLocked(now)
	t.running = true
	t.workStartedAt = now
	t.activeWindowAt = now
	t.lastStateAt = now
	t.warned = false

	msg = fmt.Sprintf(
		"▶️ Подсчет рабочего времени запущен (%s). Активность: %s, неактивность: %s",
		reason,
		FormatDuration(t.currentActiveLocked(now)),
		FormatDuration(t.currentInactiveLocked(now)),
	)

	notifier = t.notifier
	t.mu.Unlock()

	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}
}

func (t *Tracker) StartNewDay(reason string) SessionSummary {
	var msg string
	var notifier Notifier
	var summary SessionSummary

	t.mu.Lock()
	now := time.Now()

	t.started = true
	t.sessionStartedAt = now
	t.sessionName = defaultSessionName(now)
	t.lastStateAt = now
	t.running = false
	t.workStartedAt = time.Time{}
	t.activeTotal = 0
	t.inactiveStartedAt = time.Time{}
	t.inactiveTotal = 0
	t.manualAdded = 0
	t.pausedManually = false
	t.warned = false
	t.ended = false
	t.resumeRecord = nil
	t.currentActivityType = activity.DefaultType(t.cfg.DefaultActivityType)
	t.manualInactiveType = ""
	t.periods = nil
	t.currentPeriod = nil
	t.windowActive = map[string]time.Duration{}
	t.appActive = map[string]time.Duration{}
	t.activeWindowAt = time.Time{}

	if !t.locked && !t.blockedByWindow {
		t.running = true
		t.workStartedAt = now
		t.activeWindowAt = now
	} else {
		t.ensureInactiveStartedLocked(now)
	}
	t.syncPeriodLocked(now)

	msg = fmt.Sprintf(
		"📅 Начат новый день (%s). Состояние: %s",
		reason,
		StateText(t.summaryLocked(now)),
	)
	summary = t.summaryLocked(now)
	notifier = t.notifier
	t.mu.Unlock()

	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}

	return summary
}

func (t *Tracker) ContinueDay(reason string) SessionSummary {
	var msg string
	var notifier Notifier
	var summary SessionSummary

	t.mu.Lock()
	now := time.Now()
	if t.resumeRecord == nil {
		summary = t.summaryLocked(now)
		t.mu.Unlock()
		return summary
	}

	record := *t.resumeRecord
	t.started = true
	t.sessionStartedAt = record.SessionStartedAt
	t.sessionName = EmptyFallback(record.SessionName, defaultSessionName(record.SessionStartedAt))
	t.lastStateAt = now
	t.running = false
	t.workStartedAt = time.Time{}
	t.activeTotal = time.Duration(record.TotalActive)
	t.inactiveStartedAt = time.Time{}
	t.inactiveTotal = time.Duration(record.TotalInactive)
	t.manualAdded = time.Duration(record.TotalAdded)
	t.pausedManually = false
	t.warned = false
	t.ended = false
	t.resumeRecord = nil
	t.currentActivityType = activity.DefaultType(t.cfg.DefaultActivityType)
	t.manualInactiveType = ""
	t.periods = append([]history.SessionPeriod{}, record.Periods...)
	t.currentPeriod = nil
	t.windowActive = history.MetadataUsageMap(record.Metadata, history.MetadataWindowUsageKey)
	t.appActive = history.MetadataUsageMap(record.Metadata, history.MetadataAppUsageKey)
	t.activeWindowAt = time.Time{}

	if !t.locked && !t.blockedByWindow {
		t.running = true
		t.workStartedAt = now
		t.activeWindowAt = now
	} else {
		t.ensureInactiveStartedLocked(now)
	}
	t.syncPeriodLocked(now)

	msg = fmt.Sprintf(
		"🔄 Продолжен день (%s). Активность: %s, неактивность: %s",
		reason,
		FormatDuration(t.currentActiveLocked(now)),
		FormatDuration(t.currentInactiveLocked(now)),
	)
	summary = t.summaryLocked(now)
	notifier = t.notifier
	t.mu.Unlock()

	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}

	return summary
}

func (t *Tracker) StopWork(reason string) {
	var msg string
	var notifier Notifier

	t.mu.Lock()
	if !t.running {
		t.ensureInactiveStartedLocked(time.Now())
		t.mu.Unlock()
		return
	}

	now := time.Now()
	t.captureActiveWindowLocked(now)
	t.activeTotal += now.Sub(t.workStartedAt)
	t.running = false
	t.activeWindowAt = time.Time{}
	t.ensureInactiveStartedLocked(now)
	t.lastStateAt = now
	t.warned = false
	t.syncPeriodLocked(now)

	msg = fmt.Sprintf(
		"⏸ Подсчет рабочего времени остановлен (%s). Активность: %s, неактивность: %s",
		reason,
		FormatDuration(t.currentActiveLocked(now)),
		FormatDuration(t.currentInactiveLocked(now)),
	)

	notifier = t.notifier
	t.mu.Unlock()

	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}
}

func (t *Tracker) SetManualPause(paused bool) {
	var msg string
	var notifier Notifier

	t.mu.Lock()
	if !t.started || t.ended {
		t.mu.Unlock()
		return
	}

	now := time.Now()
	if paused {
		if t.running {
			t.captureActiveWindowLocked(now)
			t.activeTotal += now.Sub(t.workStartedAt)
			t.running = false
			t.activeWindowAt = time.Time{}
		}
		t.pausedManually = true
		if strings.TrimSpace(t.manualInactiveType) == "" {
			t.manualInactiveType = inactivity.TypeManualPause
		}
		t.ensureInactiveStartedLocked(now)
		msg = "⏸ Ручная пауза включена"
	} else {
		t.pausedManually = false
		if !t.locked && !t.blockedByWindow && !t.running {
			t.closeInactiveLocked(now)
			t.running = true
			t.workStartedAt = now
			t.activeWindowAt = now
		}
		msg = "▶️ Ручная пауза снята"
	}

	t.lastStateAt = now
	t.warned = false
	t.syncPeriodLocked(now)
	notifier = t.notifier
	t.mu.Unlock()

	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}
}

func (t *Tracker) SetCurrentInactivityType(name string) error {
	name = inactivity.NormalizeName(name)
	if name == "" {
		return fmt.Errorf("inactivity type is required")
	}

	var notifier Notifier

	t.mu.Lock()
	if !t.started || t.ended {
		t.mu.Unlock()
		return fmt.Errorf("session is not active")
	}
	if !t.pausedManually {
		t.mu.Unlock()
		return fmt.Errorf("current inactivity type can be set only during manual pause")
	}
	t.manualInactiveType = name
	t.lastStateAt = time.Now()
	t.syncPeriodLocked(t.lastStateAt)
	notifier = t.notifier
	t.mu.Unlock()

	msg := fmt.Sprintf("🏷 Установлен тип неактивности: %s", name)
	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}
	return nil
}

func (t *Tracker) SetCurrentActivityType(name string) error {
	name = activity.NormalizeName(name)
	if name == "" {
		return fmt.Errorf("activity type is required")
	}

	var notifier Notifier

	t.mu.Lock()
	if !t.started || t.ended {
		t.mu.Unlock()
		return fmt.Errorf("session is not active")
	}
	t.currentActivityType = name
	t.lastStateAt = time.Now()
	t.syncPeriodLocked(t.lastStateAt)
	notifier = t.notifier
	t.mu.Unlock()

	msg := fmt.Sprintf("🏷 Установлен тип активности: %s", name)
	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}
	return nil
}

func (t *Tracker) SetSessionName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("session name is required")
	}

	var notifier Notifier

	t.mu.Lock()
	if !t.started || t.ended {
		t.mu.Unlock()
		return fmt.Errorf("session is not active")
	}
	t.sessionName = name
	t.lastStateAt = time.Now()
	notifier = t.notifier
	t.mu.Unlock()

	msg := fmt.Sprintf("🏷 Установлено имя сессии: %s", name)
	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}
	return nil
}

func (t *Tracker) AddTime(d time.Duration, source string) {
	if d <= 0 {
		return
	}

	var notifier Notifier
	var active, inactive string

	t.mu.Lock()
	if !t.started || t.ended {
		t.mu.Unlock()
		return
	}
	t.manualAdded += d
	now := time.Now()
	t.lastStateAt = now
	active = FormatDuration(t.currentActiveLocked(now))
	inactive = FormatDuration(t.currentInactiveLocked(now))
	notifier = t.notifier
	t.mu.Unlock()

	msg := fmt.Sprintf(
		"➕ Добавлено времени: %s (%s). Активность: %s, неактивность: %s",
		FormatDuration(d), source, active, inactive,
	)
	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}
}

func (t *Tracker) MoveActiveToInactive(d time.Duration, source string) {
	if d <= 0 {
		return
	}

	var notifier Notifier
	var moved, active, inactive string

	t.mu.Lock()
	if !t.started || t.ended {
		t.mu.Unlock()
		return
	}

	now := time.Now()
	currentActive := t.currentActiveLocked(now)
	if currentActive <= 0 {
		t.mu.Unlock()
		return
	}
	if d > currentActive {
		d = currentActive
	}

	t.manualAdded -= d
	t.inactiveTotal += d
	t.lastStateAt = now

	moved = FormatDuration(d)
	active = FormatDuration(t.currentActiveLocked(now))
	inactive = FormatDuration(t.currentInactiveLocked(now))
	notifier = t.notifier
	t.mu.Unlock()

	msg := fmt.Sprintf(
		"➖ Перенесено из активного в неактивное: %s (%s). Активность: %s, неактивность: %s",
		moved, source, active, inactive,
	)
	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}
}

func (t *Tracker) EndSession(reason string) SessionSummary {
	var msg string
	var notifier Notifier
	var summary SessionSummary

	t.mu.Lock()
	now := time.Now()
	if t.started && !t.ended {
		if t.running {
			t.captureActiveWindowLocked(now)
			t.activeTotal += now.Sub(t.workStartedAt)
			t.running = false
			t.activeWindowAt = time.Time{}
		} else {
			t.ensureInactiveStartedLocked(now)
		}
		t.closeInactiveLocked(now)
		t.pausedManually = false
		t.ended = true
		t.lastStateAt = now
		t.syncPeriodLocked(now)

		msg = fmt.Sprintf(
			"🏁 Сессия завершена (%s). Активность: %s, неактивность: %s",
			reason,
			FormatDuration(t.currentActiveLocked(now)),
			FormatDuration(t.currentInactiveLocked(now)),
		)
	}
	summary = t.summaryLocked(now)
	notifier = t.notifier
	t.mu.Unlock()

	if msg != "" {
		log.Println(msg)
		if notifier != nil {
			notifier.SendLog(msg)
			notifier.RefreshControls()
		}
	}

	return summary
}

func (t *Tracker) HistoryPeriods() []history.SessionPeriod {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.syncPeriodLocked(now)
	out := append([]history.SessionPeriod{}, t.periods...)
	return out
}

func (t *Tracker) ActivityStats() (int, []history.ActivityStat, int, []history.ActivityStat, map[string]any) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	t.captureActiveWindowLocked(now)
	totalActive := t.currentActiveLocked(now)
	windowMap := copyDurationMap(t.windowActive)
	appMap := copyDurationMap(t.appActive)

	return len(windowMap), buildTopStats(windowMap, totalActive), len(appMap), buildTopStats(appMap, totalActive), map[string]any{
		history.MetadataWindowUsageKey: durationMapToNS(windowMap),
		history.MetadataAppUsageKey:    durationMapToNS(appMap),
	}
}

func (t *Tracker) HandleActivity(idle time.Duration) {
	now := time.Now()

	if idle < time.Second {
		t.mu.Lock()
		started := t.started
		paused := t.pausedManually
		locked := t.locked
		ended := t.ended
		blocked := t.blockedByWindow
		wasWarned := t.warned
		t.warned = false
		t.lastStateAt = now
		t.mu.Unlock()

		if !started || ended {
			return
		}
		if wasWarned {
			t.Logf("✅ Активность возобновилась")
		}
		if !paused && !locked && !blocked {
			t.StartWork("обнаружена активность")
		}
		return
	}

	t.mu.Lock()
	if !t.started {
		t.mu.Unlock()
		return
	}
	if t.ended || t.pausedManually || t.locked || t.blockedByWindow {
		if !t.running {
			t.ensureInactiveStartedLocked(now)
		}
		t.mu.Unlock()
		return
	}

	needWarn := !t.warned && idle >= t.cfg.IdleWarnAfter.Duration
	needStop := t.running && idle >= t.cfg.IdleWarnAfter.Duration+t.cfg.StopAfterWarn.Duration
	if needWarn {
		t.warned = true
	}
	t.mu.Unlock()

	if needWarn {
		t.notifySoonPause(idle)
	}
	if needStop {
		t.StopWork("нет активности")
	}
}

func (t *Tracker) SetLocked(locked bool) {
	t.mu.Lock()
	already := t.locked == locked
	t.locked = locked
	if locked {
		t.warned = false
	}
	started := t.started
	ended := t.ended
	t.mu.Unlock()

	if already {
		return
	}
	if !started || ended {
		return
	}

	if locked {
		t.Logf("🔒 Экран заблокирован")
		t.StopWork("экран заблокирован")
		return
	}

	now := time.Now()
	t.mu.Lock()
	t.ensureInactiveStartedLocked(now)
	t.lastStateAt = now
	t.mu.Unlock()

	t.Logf("🔓 Экран разблокирован")
}

func (t *Tracker) SetActiveWindowInfo(info platform.WindowInfo) {
	var needStop bool
	var needStart bool
	var stopReason string
	var startReason string
	var logMsg string

	t.mu.Lock()
	prevBlocked := t.blockedByWindow
	prevID := t.windowInfo.WindowID
	prevTitle := t.windowInfo.Title
	prevClass := t.windowInfo.WMClass
	started := t.started
	ended := t.ended
	now := time.Now()

	if started && !ended && t.running && (info.WindowID != prevID || info.Title != prevTitle || info.WMClass != prevClass) {
		t.captureActiveWindowLocked(now)
	}

	t.windowInfo = info
	t.blockedByWindow = info.BlockedByRule

	if started && !ended && info.BlockedByRule && !prevBlocked {
		logMsg = fmt.Sprintf(
			"🚫 Активность отключена из-за окна: title=%q gtk_app_id=%q kde_desktop_file=%q wm_class=%q (поле=%s, совпадение=%q)",
			info.Title,
			info.GTKApplicationID,
			info.KDEDesktopFile,
			info.WMClass,
			info.MatchedField,
			info.MatchedSubstring,
		)
		if t.running {
			needStop = true
			stopReason = "активно исключенное окно"
		} else {
			t.ensureInactiveStartedLocked(now)
			t.lastStateAt = now
			t.syncPeriodLocked(now)
		}
	}

	if started && !ended && !info.BlockedByRule && prevBlocked {
		logMsg = fmt.Sprintf("✅ Блокировка по окну снята: %q", info.Title)
		if !t.pausedManually && !t.locked && !t.running {
			needStart = true
			startReason = "сменилось активное окно"
		}
	}

	if !info.BlockedByRule && !prevBlocked && (info.WindowID != prevID || info.Title != prevTitle || info.WMClass != prevClass) {
		t.lastStateAt = now
	}
	if started && !ended && t.running {
		t.activeWindowAt = now
	}
	t.syncPeriodLocked(now)

	t.mu.Unlock()

	if logMsg != "" {
		t.Logf("%s", logMsg)
	}
	if needStop {
		t.StopWork(stopReason)
	}
	if needStart {
		t.StartWork(startReason)
	}
}

func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d == 0 {
		return "0s"
	}

	d = d.Round(time.Second)

	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	parts := make([]string, 0, 3)
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 || h > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}

	return strings.Join(parts, " ")
}

func StateText(s SessionSummary) string {
	switch {
	case !s.Started:
		return "день не начат"
	case s.Ended:
		return "сессия завершена"
	case s.PausedManually:
		return "ручная пауза"
	case s.Locked:
		return "экран заблокирован"
	case s.BlockedByWindow:
		return "остановлено по активному окну"
	case s.Running:
		return "идет подсчет"
	default:
		return "ожидание активности"
	}
}

func EmptyFallback(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (t *Tracker) getNotifier() Notifier {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.notifier
}

func (t *Tracker) ensureInactiveStartedLocked(now time.Time) {
	if !t.inactiveStartedAt.IsZero() {
		return
	}
	t.inactiveStartedAt = now
}

func (t *Tracker) desiredPeriodLocked() (kind string, periodType string, ok bool) {
	switch {
	case !t.started || t.ended:
		return "", "", false
	case t.running:
		return "activity", t.currentActivityTypeLocked(), true
	default:
		return "inactivity", t.currentInactivityTypeLocked(), true
	}
}

func (t *Tracker) syncPeriodLocked(now time.Time) {
	kind, periodType, ok := t.desiredPeriodLocked()
	if !ok {
		if t.currentPeriod != nil {
			t.currentPeriod.EndedAt = now
			t.periods = append(t.periods, *t.currentPeriod)
			t.currentPeriod = nil
		}
		return
	}

	if t.currentPeriod != nil && t.currentPeriod.Kind == kind && t.currentPeriod.Type == periodType {
		return
	}

	if t.currentPeriod != nil {
		t.currentPeriod.EndedAt = now
		t.periods = append(t.periods, *t.currentPeriod)
	}

	t.currentPeriod = &history.SessionPeriod{
		Kind:      kind,
		Type:      periodType,
		StartedAt: now,
	}
}

func (t *Tracker) closeInactiveLocked(now time.Time) {
	if t.inactiveStartedAt.IsZero() {
		return
	}
	t.inactiveTotal += now.Sub(t.inactiveStartedAt)
	t.inactiveStartedAt = time.Time{}
}

func (t *Tracker) currentActiveLocked(now time.Time) time.Duration {
	if !t.started {
		return 0
	}
	total := t.activeTotal + t.manualAdded
	if t.running {
		total += now.Sub(t.workStartedAt)
	}
	return total
}

func (t *Tracker) currentInactiveLocked(now time.Time) time.Duration {
	if !t.started {
		return 0
	}
	total := t.inactiveTotal
	if !t.running && !t.inactiveStartedAt.IsZero() {
		total += now.Sub(t.inactiveStartedAt)
	}
	return total
}

func (t *Tracker) captureActiveWindowLocked(now time.Time) {
	if !t.running || t.activeWindowAt.IsZero() {
		return
	}
	d := now.Sub(t.activeWindowAt)
	if d <= 0 {
		t.activeWindowAt = now
		return
	}

	windowName := strings.TrimSpace(t.windowInfo.Title)
	if windowName == "" {
		windowName = "(без заголовка)"
	}
	appName := strings.TrimSpace(t.windowInfo.WMClass)
	if appName == "" {
		appName = "(неизвестно)"
	}

	t.windowActive[windowName] += d
	t.appActive[appName] += d
	t.activeWindowAt = now
}

func (t *Tracker) summaryLocked(now time.Time) SessionSummary {
	t.syncPeriodLocked(now)
	t.captureActiveWindowLocked(now)
	periods := append([]history.SessionPeriod{}, t.periods...)
	windowMap := copyDurationMap(t.windowActive)
	appMap := copyDurationMap(t.appActive)
	return SessionSummary{
		Started:                t.started,
		CanContinueDay:         t.resumeRecord != nil,
		SessionName:            t.sessionName,
		SessionStartedAt:       t.sessionStartedAt,
		TotalActive:            t.currentActiveLocked(now),
		TotalInactive:          t.currentInactiveLocked(now),
		TotalAdded:             t.manualAdded,
		WindowCount:            len(windowMap),
		AppCount:               len(appMap),
		WindowStats:            buildUsageStats(windowMap),
		AppStats:               buildUsageStats(appMap),
		Periods:                periods,
		CurrentActivityType:    t.currentActivityTypeLocked(),
		CurrentActivityColor:   t.currentActivityColorLocked(),
		CurrentInactivityType:  t.currentInactivityTypeLocked(),
		CurrentInactivityColor: t.currentInactivityColorLocked(),

		Running:         t.running,
		PausedManually:  t.pausedManually,
		Locked:          t.locked,
		BlockedByWindow: t.blockedByWindow,
		LastStateChange: t.lastStateAt,
		Ended:           t.ended,

		Window: t.windowInfo,
	}
}

func defaultSessionName(startedAt time.Time) string {
	if startedAt.IsZero() {
		return "Сессия"
	}
	return "Сессия " + startedAt.Format("2006-01-02 15:04")
}

func copyDurationMap(items map[string]time.Duration) map[string]time.Duration {
	out := make(map[string]time.Duration, len(items))
	for k, v := range items {
		out[k] = v
	}
	return out
}

func durationMapToNS(items map[string]time.Duration) map[string]int64 {
	out := make(map[string]int64, len(items))
	for k, v := range items {
		out[k] = int64(v)
	}
	return out
}

func buildTopStats(items map[string]time.Duration, total time.Duration) []history.ActivityStat {
	if total <= 0 || len(items) == 0 {
		return nil
	}

	stats := make([]history.ActivityStat, 0, len(items))
	for name, d := range items {
		percent := float64(d) * 100 / float64(total)
		if percent <= 5 {
			continue
		}
		stats = append(stats, history.ActivityStat{
			Name:     name,
			ActiveNS: int64(d),
			Percent:  percent,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].ActiveNS == stats[j].ActiveNS {
			return stats[i].Name < stats[j].Name
		}
		return stats[i].ActiveNS > stats[j].ActiveNS
	})
	if len(stats) > 10 {
		stats = stats[:10]
	}
	return stats
}

func buildUsageStats(items map[string]time.Duration) []history.ActivityStat {
	if len(items) == 0 {
		return nil
	}

	stats := make([]history.ActivityStat, 0, len(items))
	for name, d := range items {
		stats = append(stats, history.ActivityStat{
			Name:     name,
			ActiveNS: int64(d),
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].ActiveNS == stats[j].ActiveNS {
			return stats[i].Name < stats[j].Name
		}
		return stats[i].ActiveNS > stats[j].ActiveNS
	})
	return stats
}

func (t *Tracker) currentActivityTypeLocked() string {
	if !t.started || t.ended {
		return ""
	}
	return EmptyFallback(t.currentActivityType, activity.DefaultType(t.cfg.DefaultActivityType))
}

func (t *Tracker) currentActivityColorLocked() string {
	return activity.FindColor(activity.Merge(nil), t.currentActivityTypeLocked())
}

func (t *Tracker) currentInactivityTypeLocked() string {
	switch {
	case !t.started || t.running || t.ended:
		return ""
	case t.locked:
		return inactivity.TypeLocked
	case t.blockedByWindow:
		return inactivity.TypeBlockedWindow
	case t.pausedManually:
		return EmptyFallback(t.manualInactiveType, inactivity.TypeManualPause)
	default:
		return inactivity.TypeIdle
	}
}

func (t *Tracker) currentInactivityColorLocked() string {
	return inactivity.FindColor(inactivity.Merge(nil), t.currentInactivityTypeLocked())
}

func (t *Tracker) notifySoonPause(idle time.Duration) {
	text := fmt.Sprintf(
		"Нет активности уже %s. Через %s подсчет времени остановится.",
		FormatDuration(idle),
		FormatDuration(t.cfg.StopAfterWarn.Duration),
	)
	if t.cfg.EnableDesktopNotifications && t.notifyFn != nil {
		_ = t.notifyFn("Work Activity Tracker", text)
	}
	t.Logf("⚠️ %s", text)
}
