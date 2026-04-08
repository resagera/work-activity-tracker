package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/godbus/dbus/v5"

	"work-activity-tracker/pkg/version"
)

const (
	defaultIdleWarnAfter = 2 * time.Minute
	defaultStopAfterWarn = 1 * time.Minute
	defaultPollInterval  = 5 * time.Second
	defaultConfigName    = "config.json"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}

	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("parse duration %q: %w", s, err)
		}
		d.Duration = parsed
		return nil
	}

	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		d.Duration = time.Duration(n)
		return nil
	}

	return fmt.Errorf("duration must be string like \"2m\" or integer nanoseconds")
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

type Config struct {
	TelegramToken            string   `json:"telegram_token"`
	TelegramChatID           int64    `json:"telegram_chat_id"`
	HTTPPort                 int      `json:"http_port"`
	IdleWarnAfter            Duration `json:"idle_warn_after"`
	StopAfterWarn            Duration `json:"stop_after_warn"`
	PollInterval             Duration `json:"poll_interval"`
	ExcludedWindowSubstrings []string `json:"excluded_window_substrings"`
	ShowVersion              bool     `json:"show_version"`
}

type WindowInfo struct {
	WindowID          string `json:"window_id"`
	Title             string `json:"title"`
	GTKApplicationID  string `json:"gtk_application_id"`
	KDEDesktopFile    string `json:"kde_net_wm_desktop_file"`
	WMClass           string `json:"wm_class"`
	MatchedField      string `json:"matched_field"`
	MatchedSubstring  string `json:"matched_substring"`
	BlockedByRule     bool   `json:"blocked_by_rule"`
	LastRawXpropBlock string `json:"-"`
}

type SessionSummary struct {
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

	Window WindowInfo `json:"window"`
}

type Tracker struct {
	mu sync.Mutex

	cfg Config

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
	windowInfo      WindowInfo

	tele *TelegramNotifier
}

func NewTracker(cfg Config) *Tracker {
	now := time.Now()
	return &Tracker{
		cfg:              cfg,
		sessionStartedAt: now,
		lastStateAt:      now,
	}
}

func (t *Tracker) SetTelegram(n *TelegramNotifier) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tele = n
}

func (t *Tracker) getTelegram() (*TelegramNotifier, int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tele, t.cfg.TelegramChatID
}

func (t *Tracker) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Println(msg)

	tele, chatID := t.getTelegram()
	if tele != nil && chatID != 0 {
		tele.SendLog(chatID, msg)
	}
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
	total := t.activeTotal + t.manualAdded
	if t.running {
		total += now.Sub(t.workStartedAt)
	}
	return total
}

func (t *Tracker) currentInactiveLocked(now time.Time) time.Duration {
	total := t.inactiveTotal
	if !t.running && !t.inactiveStartedAt.IsZero() {
		total += now.Sub(t.inactiveStartedAt)
	}
	return total
}

func (t *Tracker) summaryLocked(now time.Time) SessionSummary {
	return SessionSummary{
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

func (t *Tracker) Summary() SessionSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.summaryLocked(time.Now())
}

func (t *Tracker) startWork(reason string) {
	var msg string
	var tele *TelegramNotifier
	var chatID int64

	t.mu.Lock()
	if t.ended || t.running || t.pausedManually || t.locked || t.blockedByWindow {
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
		formatDuration(t.currentActiveLocked(now)),
		formatDuration(t.currentInactiveLocked(now)),
	)

	tele = t.tele
	chatID = t.cfg.TelegramChatID
	t.mu.Unlock()

	log.Println(msg)
	if tele != nil && chatID != 0 {
		tele.SendLog(chatID, msg)
		tele.RefreshControls(t)
	}
}

func (t *Tracker) stopWork(reason string) {
	var msg string
	var tele *TelegramNotifier
	var chatID int64

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
		formatDuration(t.currentActiveLocked(now)),
		formatDuration(t.currentInactiveLocked(now)),
	)

	tele = t.tele
	chatID = t.cfg.TelegramChatID
	t.mu.Unlock()

	log.Println(msg)
	if tele != nil && chatID != 0 {
		tele.SendLog(chatID, msg)
		tele.RefreshControls(t)
	}
}

func (t *Tracker) setManualPause(paused bool) {
	var msg string
	var tele *TelegramNotifier
	var chatID int64

	t.mu.Lock()
	if t.ended {
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
	tele = t.tele
	chatID = t.cfg.TelegramChatID
	t.mu.Unlock()

	log.Println(msg)
	if tele != nil && chatID != 0 {
		tele.SendLog(chatID, msg)
		tele.RefreshControls(t)
	}
}

func (t *Tracker) addTime(d time.Duration, source string) {
	if d <= 0 {
		return
	}

	var tele *TelegramNotifier
	var chatID int64
	var active, inactive string

	t.mu.Lock()
	t.manualAdded += d
	now := time.Now()
	t.lastStateAt = now
	active = formatDuration(t.currentActiveLocked(now))
	inactive = formatDuration(t.currentInactiveLocked(now))
	tele = t.tele
	chatID = t.cfg.TelegramChatID
	t.mu.Unlock()

	msg := fmt.Sprintf(
		"➕ Добавлено времени: %s (%s). Активность: %s, неактивность: %s",
		formatDuration(d), source, active, inactive,
	)
	log.Println(msg)
	if tele != nil && chatID != 0 {
		tele.SendLog(chatID, msg)
		tele.RefreshControls(t)
	}
}

func (t *Tracker) endSession(reason string) SessionSummary {
	var msg string
	var tele *TelegramNotifier
	var chatID int64
	var summary SessionSummary

	t.mu.Lock()
	now := time.Now()
	if !t.ended {
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
			formatDuration(t.currentActiveLocked(now)),
			formatDuration(t.currentInactiveLocked(now)),
		)
	}
	summary = t.summaryLocked(now)
	tele = t.tele
	chatID = t.cfg.TelegramChatID
	t.mu.Unlock()

	if msg != "" {
		log.Println(msg)
		if tele != nil && chatID != 0 {
			tele.SendLog(chatID, msg)
			tele.RefreshControls(t)
		}
	}

	return summary
}

func (t *Tracker) HandleActivity(idle time.Duration) {
	now := time.Now()

	if idle < time.Second {
		t.mu.Lock()
		paused := t.pausedManually
		locked := t.locked
		ended := t.ended
		blocked := t.blockedByWindow
		wasWarned := t.warned
		t.warned = false
		t.lastStateAt = now
		t.mu.Unlock()

		if ended {
			return
		}
		if wasWarned {
			t.logf("✅ Активность возобновилась")
		}
		if !paused && !locked && !blocked {
			t.startWork("обнаружена активность")
		}
		return
	}

	t.mu.Lock()
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
		t.stopWork("нет активности")
	}
}

func (t *Tracker) SetLocked(locked bool) {
	t.mu.Lock()
	if t.ended {
		t.mu.Unlock()
		return
	}

	already := t.locked == locked
	t.locked = locked
	if locked {
		t.warned = false
	}
	t.mu.Unlock()

	if already {
		return
	}

	if locked {
		t.logf("🔒 Экран заблокирован")
		t.stopWork("экран заблокирован")
		return
	}

	now := time.Now()
	t.mu.Lock()
	t.ensureInactiveStartedLocked(now)
	t.lastStateAt = now
	t.mu.Unlock()

	t.logf("🔓 Экран разблокирован")
}

func (t *Tracker) SetActiveWindowInfo(info WindowInfo) {
	var needStop bool
	var needStart bool
	var stopReason string
	var startReason string
	var logMsg string

	t.mu.Lock()
	if t.ended {
		t.mu.Unlock()
		return
	}

	prevBlocked := t.blockedByWindow
	prevID := t.windowInfo.WindowID
	prevTitle := t.windowInfo.Title

	t.windowInfo = info
	t.blockedByWindow = info.BlockedByRule

	if info.BlockedByRule && !prevBlocked {
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

	if !info.BlockedByRule && prevBlocked {
		logMsg = fmt.Sprintf("✅ Блокировка по окну снята: %q", info.Title)
		if !t.pausedManually && !t.locked && !t.running {
			needStart = true
			startReason = "сменилось активное окно"
		}
	}

	if !info.BlockedByRule && !prevBlocked && (info.WindowID != prevID || info.Title != prevTitle) {
		now := time.Now()
		t.lastStateAt = now
	}

	t.mu.Unlock()

	if logMsg != "" {
		t.logf("%s", logMsg)
	}
	if needStop {
		t.stopWork(stopReason)
	}
	if needStart {
		t.startWork(startReason)
	}
}

func (t *Tracker) notifySoonPause(idle time.Duration) {
	text := fmt.Sprintf(
		"Нет активности уже %s. Через %s подсчет времени остановится.",
		formatDuration(idle),
		formatDuration(t.cfg.StopAfterWarn.Duration),
	)
	_ = sendDesktopNotification("Work Activity Tracker", text)
	t.logf("⚠️ %s", text)
}

type TelegramNotifier struct {
	bot *tgbotapi.BotAPI

	mu                sync.Mutex
	controlsMessageID int
}

func NewTelegramNotifier(token string) (*TelegramNotifier, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &TelegramNotifier{bot: bot}, nil
}

func (n *TelegramNotifier) SendLog(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = n.bot.Send(msg)
}

func (n *TelegramNotifier) sessionText(s SessionSummary) string {
	title := emptyFallback(s.Window.Title, "(не определено)")
	gtkAppID := emptyFallback(s.Window.GTKApplicationID, "-")
	kdeDesktopFile := emptyFallback(s.Window.KDEDesktopFile, "-")
	wmClass := emptyFallback(s.Window.WMClass, "-")

	blockReason := ""
	if s.Window.BlockedByRule {
		blockReason = fmt.Sprintf(
			"\nСовпадение: поле=%s, подстрока=%s",
			emptyFallback(s.Window.MatchedField, "-"),
			emptyFallback(s.Window.MatchedSubstring, "-"),
		)
	}

	return fmt.Sprintf(
		"📅 Сессия\nСтарт: %s\nСостояние: %s\nИтого активности: %s\nИтого неактивности: %s\nОкно: %s\nGTK_APPLICATION_ID: %s\nKDE_NET_WM_DESKTOP_FILE: %s\nWM_CLASS: %s%s",
		s.SessionStartedAt.Format(time.RFC3339),
		stateText(s),
		formatDuration(s.TotalActive),
		formatDuration(s.TotalInactive),
		title,
		gtkAppID,
		kdeDesktopFile,
		wmClass,
		blockReason,
	)
}

func (n *TelegramNotifier) controlsMarkup(s SessionSummary) tgbotapi.InlineKeyboardMarkup {
	stateBtnText := "⏸ Пауза"
	stateBtnData := "pause"

	if s.Ended {
		stateBtnText = "🚫 Завершено"
		stateBtnData = "noop"
	} else if s.PausedManually || !s.Running {
		stateBtnText = "▶️ Старт"
		stateBtnData = "start"
	}

	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(stateBtnText, stateBtnData),
			tgbotapi.NewInlineKeyboardButtonData("🏁 Завершить день", "end"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("➕ 30м", "add:30m"),
			tgbotapi.NewInlineKeyboardButtonData("➕ 1ч", "add:1h"),
			tgbotapi.NewInlineKeyboardButtonData("➕ 2ч", "add:2h"),
		},
	}

	return tgbotapi.NewInlineKeyboardMarkup(buttons...)
}

func (n *TelegramNotifier) SendOrReplaceControls(chatID int64, tracker *Tracker) {
	s := tracker.Summary()
	text := n.sessionText(s)
	markup := n.controlsMarkup(s)

	n.mu.Lock()
	msgID := n.controlsMessageID
	n.mu.Unlock()

	if msgID == 0 {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ReplyMarkup = markup
		sent, err := n.bot.Send(msg)
		if err == nil {
			n.mu.Lock()
			n.controlsMessageID = sent.MessageID
			n.mu.Unlock()
		}
		return
	}

	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, markup)
	_, err := n.bot.Send(edit)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ReplyMarkup = markup
		sent, err2 := n.bot.Send(msg)
		if err2 == nil {
			n.mu.Lock()
			n.controlsMessageID = sent.MessageID
			n.mu.Unlock()
		}
	}
}

func (n *TelegramNotifier) RefreshControls(tracker *Tracker) {
	tele, chatID := tracker.getTelegram()
	if tele == nil || chatID == 0 {
		return
	}
	n.SendOrReplaceControls(chatID, tracker)
}

func (n *TelegramNotifier) Run(ctx context.Context, tracker *Tracker, chatID int64) {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := n.bot.GetUpdatesChan(updateConfig)

	for {
		select {
		case <-ctx.Done():
			return
		case upd, ok := <-updates:
			if !ok {
				return
			}

			if upd.Message != nil {
				if upd.Message.Chat == nil || upd.Message.Chat.ID != chatID {
					continue
				}
				n.handleMessage(tracker, upd.Message)
			}

			if upd.CallbackQuery != nil {
				if upd.CallbackQuery.Message == nil || upd.CallbackQuery.Message.Chat == nil || upd.CallbackQuery.Message.Chat.ID != chatID {
					continue
				}
				n.handleCallback(tracker, upd.CallbackQuery)
			}
		}
	}
}

func (n *TelegramNotifier) handleMessage(tracker *Tracker, msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.Text)

	switch {
	case text == "/start":
		n.SendOrReplaceControls(msg.Chat.ID, tracker)

	case text == "/status":
		s := tracker.Summary()
		reply := tgbotapi.NewMessage(
			msg.Chat.ID,
			fmt.Sprintf(
				"Состояние: %s\nИтого активности: %s\nИтого неактивности: %s\nОкно: %s\nGTK_APPLICATION_ID: %s\nKDE_NET_WM_DESKTOP_FILE: %s\nWM_CLASS: %s",
				stateText(s),
				formatDuration(s.TotalActive),
				formatDuration(s.TotalInactive),
				emptyFallback(s.Window.Title, "(не определено)"),
				emptyFallback(s.Window.GTKApplicationID, "-"),
				emptyFallback(s.Window.KDEDesktopFile, "-"),
				emptyFallback(s.Window.WMClass, "-"),
			),
		)
		_, _ = n.bot.Send(reply)

	case strings.HasPrefix(text, "/add "):
		v := strings.TrimSpace(strings.TrimPrefix(text, "/add "))
		d, err := time.ParseDuration(v)
		if err != nil {
			_, _ = n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Используй формат: /add 1h30m"))
			return
		}
		tracker.addTime(d, "telegram command")
		n.RefreshControls(tracker)

	case text == "/pause":
		tracker.setManualPause(true)

	case text == "/resume":
		tracker.setManualPause(false)

	case text == "/end":
		tracker.endSession("telegram command")

	default:
		_, _ = n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Команды: /start, /status, /add 1h30m, /pause, /resume, /end"))
	}
}

func (n *TelegramNotifier) handleCallback(tracker *Tracker, q *tgbotapi.CallbackQuery) {
	data := q.Data

	switch {
	case data == "noop":
	case data == "pause":
		tracker.setManualPause(true)
	case data == "start":
		tracker.setManualPause(false)
	case data == "end":
		tracker.endSession("telegram button")
	case strings.HasPrefix(data, "add:"):
		d, err := time.ParseDuration(strings.TrimPrefix(data, "add:"))
		if err == nil {
			tracker.addTime(d, "telegram button")
		}
	}

	answer := tgbotapi.NewCallback(q.ID, "OK")
	_, _ = n.bot.Request(answer)
	n.RefreshControls(tracker)
}

type App struct {
	cfg     Config
	tracker *Tracker
}

func NewApp(cfg Config) *App {
	return &App{
		cfg:     cfg,
		tracker: NewTracker(cfg),
	}
}

func (a *App) Run(ctx context.Context) error {
	printConfig(a.cfg)

	if a.cfg.TelegramToken != "" && a.cfg.TelegramChatID != 0 {
		tele, err := NewTelegramNotifier(a.cfg.TelegramToken)
		if err != nil {
			return fmt.Errorf("telegram init: %w", err)
		}
		a.tracker.SetTelegram(tele)
		tele.SendOrReplaceControls(a.cfg.TelegramChatID, a.tracker)
		go tele.Run(ctx, a.tracker, a.cfg.TelegramChatID)
	}

	if a.cfg.HTTPPort > 0 {
		go a.runHTTP(ctx)
	}

	go a.runIdlePolling(ctx)
	go a.runLockPolling(ctx)
	go a.runLockSignalWatcher(ctx)
	go a.runActiveWindowPolling(ctx)

	a.tracker.startWork("старт программы")

	<-ctx.Done()
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

		a.tracker.addTime(time.Duration(minutes)*time.Minute, "http api")
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/pause", func(w http.ResponseWriter, r *http.Request) {
		a.tracker.setManualPause(true)
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		a.tracker.setManualPause(false)
		writeJSON(w, http.StatusOK, a.tracker.Summary())
	})

	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.tracker.endSession("http api"))
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

	a.tracker.logf("🌐 HTTP API запущен на :%d", a.cfg.HTTPPort)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		a.tracker.logf("HTTP API ошибка: %v", err)
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
			idle, err := getIdleDuration()
			if err != nil {
				a.tracker.logf("idle check error: %v", err)
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
			locked, err := isScreenLocked()
			if err != nil {
				a.tracker.logf("lock check error: %v", err)
				continue
			}
			a.tracker.SetLocked(locked)
		}
	}
}

func (a *App) runLockSignalWatcher(ctx context.Context) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		a.tracker.logf("lock signal watcher error: %v", err)
		return
	}
	defer conn.Close()

	rule := "type='signal',interface='org.gnome.ScreenSaver',member='ActiveChanged'"
	call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule)
	if call.Err != nil {
		a.tracker.logf("lock signal watcher add match error: %v", call.Err)
		return
	}

	signals := make(chan *dbus.Signal, 10)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)

	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-signals:
			if sig == nil {
				continue
			}
			if sig.Name != "org.gnome.ScreenSaver.ActiveChanged" {
				continue
			}
			if len(sig.Body) < 1 {
				continue
			}
			locked, ok := sig.Body[0].(bool)
			if !ok {
				continue
			}
			a.tracker.SetLocked(locked)
		}
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
			info, err := getActiveWindowInfo(a.cfg.ExcludedWindowSubstrings)
			if err != nil {
				a.tracker.logf("active window check error: %v", err)
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

func getIdleDuration() (time.Duration, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	obj := conn.Object("org.gnome.Mutter.IdleMonitor", "/org/gnome/Mutter/IdleMonitor/Core")
	call := obj.Call("org.gnome.Mutter.IdleMonitor.GetIdletime", 0)
	if call.Err != nil {
		return 0, call.Err
	}

	var ms uint64
	if err := dbus.Store(call.Body, &ms); err != nil {
		return 0, err
	}

	return time.Duration(ms) * time.Millisecond, nil
}

func isScreenLocked() (bool, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false, err
	}
	defer conn.Close()

	obj := conn.Object("org.gnome.ScreenSaver", "/org/gnome/ScreenSaver")
	v, err := obj.GetProperty("org.gnome.ScreenSaver.Active")
	if err == nil {
		if active, ok := v.Value().(bool); ok {
			return active, nil
		}
	}

	return false, nil
}

func getActiveWindowInfo(excluded []string) (WindowInfo, error) {
	windowID, err := getFocusedWindowID()
	if err != nil {
		return WindowInfo{}, err
	}

	title, err := getFocusedWindowTitle()
	if err != nil {
		return WindowInfo{}, err
	}

	xpropOut, err := getWindowXProp(windowID)
	if err != nil {
		return WindowInfo{}, err
	}

	info := WindowInfo{
		WindowID:          windowID,
		Title:             title,
		GTKApplicationID:  parseXPropValue(xpropOut, "_GTK_APPLICATION_ID"),
		KDEDesktopFile:    parseXPropValue(xpropOut, "_KDE_NET_WM_DESKTOP_FILE"),
		WMClass:           parseWMClass(xpropOut),
		LastRawXpropBlock: xpropOut,
	}

	blocked, field, substr := matchWindowInfo(info, excluded)
	info.BlockedByRule = blocked
	info.MatchedField = field
	info.MatchedSubstring = substr

	return info, nil
}

func getFocusedWindowID() (string, error) {
	cmd := exec.Command("xdotool", "getwindowfocus")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getFocusedWindowTitle() (string, error) {
	cmd := exec.Command("xdotool", "getwindowfocus", "getwindowname")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getWindowXProp(windowID string) (string, error) {
	cmd := exec.Command("xprop", "-id", windowID)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText != "" {
			return "", fmt.Errorf("%w: %s", err, errText)
		}
		return "", err
	}

	return stdout.String(), nil
}

func parseXPropValue(xpropOutput, key string) string {
	lines := strings.Split(xpropOutput, "\n")
	prefix := key + "("
	altPrefix := key + " ="

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, prefix) || strings.HasPrefix(line, altPrefix) || strings.HasPrefix(line, key+" ") {
			if idx := strings.Index(line, "="); idx >= 0 {
				v := strings.TrimSpace(line[idx+1:])
				return trimQuoted(v)
			}
		}
	}

	return ""
}

func parseWMClass(xpropOutput string) string {
	lines := strings.Split(xpropOutput, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "WM_CLASS") {
			if idx := strings.Index(line, "="); idx >= 0 {
				v := strings.TrimSpace(line[idx+1:])
				return trimQuoted(v)
			}
		}
	}
	return ""
}

func trimQuoted(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"")
	return s
}

func matchWindowInfo(info WindowInfo, excluded []string) (bool, string, string) {
	fields := []struct {
		name  string
		value string
	}{
		{name: "title", value: info.Title},
		{name: "_GTK_APPLICATION_ID", value: info.GTKApplicationID},
		{name: "_KDE_NET_WM_DESKTOP_FILE", value: info.KDEDesktopFile},
		{name: "WM_CLASS", value: info.WMClass},
	}

	for _, sub := range excluded {
		sub = strings.TrimSpace(sub)
		if sub == "" {
			continue
		}

		for _, field := range fields {
			if strings.Contains(strings.ToLower(field.value), strings.ToLower(sub)) {
				return true, field.name, sub
			}
		}
	}

	return false, "", ""
}

func sendDesktopNotification(title, body string) error {
	cmd := exec.Command("notify-send", title, body)
	return cmd.Run()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func printConfig(cfg Config) {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Printf("CONFIG:\n%s\n", string(b))
}

func formatDuration(d time.Duration) string {
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

func stateText(s SessionSummary) string {
	switch {
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

func emptyFallback(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func defaultConfig() Config {
	return Config{
		IdleWarnAfter: Duration{Duration: defaultIdleWarnAfter},
		StopAfterWarn: Duration{Duration: defaultStopAfterWarn},
		PollInterval:  Duration{Duration: defaultPollInterval},
		ExcludedWindowSubstrings: []string{
			"Telegram",
			"Youtube",
		},
	}
}

func resolveConfigPath(explicit string) string {
	candidates := make([]string, 0, 3)

	if explicit != "" {
		candidates = append(candidates, explicit)
	} else {
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(cwd, defaultConfigName))
		}
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), defaultConfigName))
		}
		candidates = append(candidates, defaultConfigName)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	if path == "" {
		return cfg, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}

	if cfg.IdleWarnAfter.Duration <= 0 {
		cfg.IdleWarnAfter = Duration{Duration: defaultIdleWarnAfter}
	}
	if cfg.StopAfterWarn.Duration <= 0 {
		cfg.StopAfterWarn = Duration{Duration: defaultStopAfterWarn}
	}
	if cfg.PollInterval.Duration <= 0 {
		cfg.PollInterval = Duration{Duration: defaultPollInterval}
	}

	return cfg, nil
}

func loadConfigFromArgs(args []string) (Config, error) {
	configPath := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "-config" || arg == "--config":
			if i+1 >= len(args) {
				return Config{}, fmt.Errorf("%s requires value", arg)
			}
			configPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "-config="):
			configPath = strings.TrimPrefix(arg, "-config=")
		case strings.HasPrefix(arg, "--config="):
			configPath = strings.TrimPrefix(arg, "--config=")
		}
	}

	return loadConfig(resolveConfigPath(configPath))
}

func overrideFromFlags(cfg *Config, args []string) error {
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		configPath     = fs.String("config", "", "path to config.json")
		telegramToken  = fs.String("telegram-token", "", "telegram bot token")
		telegramChatID = fs.Int64("telegram-chat-id", 0, "telegram chat id")
		httpPort       = fs.Int("http-port", 0, "http api port, 0 disables api")
		idleWarn       = fs.Duration("idle-warn-after", 0, "time without activity before warning")
		stopAfter      = fs.Duration("stop-after-warn", 0, "time after warning before stop")
		pollInterval   = fs.Duration("poll-interval", 0, "idle/lock polling interval")
		showVersion    = fs.Bool("version", false, "show version")
	)

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if *configPath != "" {
		loaded, err := loadConfig(*configPath)
		if err != nil {
			return err
		}
		*cfg = loaded
	}

	if *telegramToken != "" {
		cfg.TelegramToken = *telegramToken
	}
	if *telegramChatID != 0 {
		cfg.TelegramChatID = *telegramChatID
	}
	if *httpPort != 0 {
		cfg.HTTPPort = *httpPort
	}
	if *idleWarn > 0 {
		cfg.IdleWarnAfter = Duration{Duration: *idleWarn}
	}
	if *stopAfter > 0 {
		cfg.StopAfterWarn = Duration{Duration: *stopAfter}
	}
	if *pollInterval > 0 {
		cfg.PollInterval = Duration{Duration: *pollInterval}
	}
	if *showVersion {
		cfg.ShowVersion = true
	}

	return nil
}

func main() {
	version.MajorVersion = "1"
	version.MinorVersion = "1"

	cfg, err := loadConfigFromArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := overrideFromFlags(&cfg, os.Args); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	fmt.Println("Version: " + version.Get().SemVer())
	if cfg.ShowVersion {
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app := NewApp(cfg)
	if err := app.Run(ctx); err != nil {
		log.Fatalf("run: %v", err)
	}
}
