package tracker

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"work-activity-tracker/internal/config"
	"work-activity-tracker/internal/history"
	"work-activity-tracker/internal/platform"
)

type Notifier interface {
	SendLog(text string)
	RefreshControls()
}

type SessionSummary struct {
	Started          bool          `json:"started"`
	CanContinueDay   bool          `json:"can_continue_day"`
	SessionStartedAt time.Time     `json:"session_started_at"`
	TotalActive      time.Duration `json:"total_active"`
	TotalInactive    time.Duration `json:"total_inactive"`
	TotalAdded       time.Duration `json:"total_added"`

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

	resumeRecord *history.SessionRecord
}

func New(cfg config.Config, notifyFn func(title, body string) error) *Tracker {
	now := time.Now()
	return &Tracker{
		cfg:         cfg,
		lastStateAt: now,
		notifyFn:    notifyFn,
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

	if !t.locked && !t.blockedByWindow {
		t.running = true
		t.workStartedAt = now
	} else {
		t.ensureInactiveStartedLocked(now)
	}

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

	if !t.locked && !t.blockedByWindow {
		t.running = true
		t.workStartedAt = now
	} else {
		t.ensureInactiveStartedLocked(now)
	}

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
	t.activeTotal += now.Sub(t.workStartedAt)
	t.running = false
	t.ensureInactiveStartedLocked(now)
	t.lastStateAt = now
	t.warned = false

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
			t.activeTotal += now.Sub(t.workStartedAt)
			t.running = false
		}
		t.pausedManually = true
		t.ensureInactiveStartedLocked(now)
		msg = "⏸ Ручная пауза включена"
	} else {
		t.pausedManually = false
		if !t.locked && !t.blockedByWindow && !t.running {
			t.closeInactiveLocked(now)
			t.running = true
			t.workStartedAt = now
		}
		msg = "▶️ Ручная пауза снята"
	}

	t.lastStateAt = now
	t.warned = false
	notifier = t.notifier
	t.mu.Unlock()

	log.Println(msg)
	if notifier != nil {
		notifier.SendLog(msg)
		notifier.RefreshControls()
	}
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
			t.activeTotal += now.Sub(t.workStartedAt)
			t.running = false
		} else {
			t.ensureInactiveStartedLocked(now)
		}
		t.closeInactiveLocked(now)
		t.pausedManually = false
		t.ended = true
		t.lastStateAt = now

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

	t.windowInfo = info
	t.blockedByWindow = info.BlockedByRule
	started := t.started
	ended := t.ended

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
			now := time.Now()
			t.ensureInactiveStartedLocked(now)
			t.lastStateAt = now
		}
	}

	if started && !ended && !info.BlockedByRule && prevBlocked {
		logMsg = fmt.Sprintf("✅ Блокировка по окну снята: %q", info.Title)
		if !t.pausedManually && !t.locked && !t.running {
			needStart = true
			startReason = "сменилось активное окно"
		}
	}

	if !info.BlockedByRule && !prevBlocked && (info.WindowID != prevID || info.Title != prevTitle) {
		t.lastStateAt = time.Now()
	}

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

func (t *Tracker) summaryLocked(now time.Time) SessionSummary {
	return SessionSummary{
		Started:          t.started,
		CanContinueDay:   t.resumeRecord != nil,
		SessionStartedAt: t.sessionStartedAt,
		TotalActive:      t.currentActiveLocked(now),
		TotalInactive:    t.currentInactiveLocked(now),
		TotalAdded:       t.manualAdded,

		Running:         t.running,
		PausedManually:  t.pausedManually,
		Locked:          t.locked,
		BlockedByWindow: t.blockedByWindow,
		LastStateChange: t.lastStateAt,
		Ended:           t.ended,

		Window: t.windowInfo,
	}
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
