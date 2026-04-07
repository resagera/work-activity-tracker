package main

import (
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
	TelegramToken  string   `json:"telegram_token"`
	TelegramChatID int64    `json:"telegram_chat_id"`
	HTTPPort       int      `json:"http_port"`
	IdleWarnAfter  Duration `json:"idle_warn_after"`
	StopAfterWarn  Duration `json:"stop_after_warn"`
	PollInterval   Duration `json:"poll_interval"`
}

type SessionSummary struct {
	SessionStartedAt time.Time     `json:"session_started_at"`
	TotalWorked      time.Duration `json:"total_worked"`
	TotalAdded       time.Duration `json:"total_added"`
	Running          bool          `json:"running"`
	PausedManually   bool          `json:"paused_manually"`
	Locked           bool          `json:"locked"`
	LastStateChange  time.Time     `json:"last_state_change"`
	Ended            bool          `json:"ended"`
}

type Tracker struct {
	mu sync.Mutex

	cfg Config

	sessionStartedAt time.Time
	workStartedAt    time.Time
	accumulated      time.Duration
	manualAdded      time.Duration

	running        bool
	pausedManually bool
	locked         bool
	warned         bool
	lastInputAt    time.Time
	lastStateAt    time.Time
	ended          bool

	tele *TelegramNotifier
}

func NewTracker(cfg Config) *Tracker {
	now := time.Now()
	return &Tracker{
		cfg:              cfg,
		sessionStartedAt: now,
		lastInputAt:      now,
		lastStateAt:      now,
	}
}

func (t *Tracker) SetTelegram(n *TelegramNotifier) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tele = n
}

func (t *Tracker) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Println(msg)
	t.mu.Lock()
	tele := t.tele
	chatID := t.cfg.TelegramChatID
	t.mu.Unlock()
	if tele != nil && chatID != 0 {
		tele.SendLog(chatID, msg)
	}
}

func (t *Tracker) startWork(reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended || t.running || t.pausedManually || t.locked {
		return
	}
	now := time.Now()
	t.running = true
	t.workStartedAt = now
	t.lastStateAt = now
	t.warned = false
	msg := fmt.Sprintf("▶️ Подсчет рабочего времени запущен (%s)", reason)
	log.Println(msg)
	if t.tele != nil && t.cfg.TelegramChatID != 0 {
		t.tele.SendLog(t.cfg.TelegramChatID, msg)
		t.tele.RefreshControls(t)
	}
}

func (t *Tracker) stopWork(reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return
	}
	now := time.Now()
	t.accumulated += now.Sub(t.workStartedAt)
	t.running = false
	t.lastStateAt = now
	t.warned = false
	msg := fmt.Sprintf("⏸ Подсчет рабочего времени остановлен (%s). Итого: %s", reason, formatDuration(t.accumulated+t.manualAdded))
	log.Println(msg)
	if t.tele != nil && t.cfg.TelegramChatID != 0 {
		t.tele.SendLog(t.cfg.TelegramChatID, msg)
		t.tele.RefreshControls(t)
	}
}

func (t *Tracker) setManualPause(paused bool) {
	t.mu.Lock()
	if t.ended {
		t.mu.Unlock()
		return
	}
	wasRunning := t.running
	now := time.Now()
	if paused {
		t.pausedManually = true
		if wasRunning {
			t.accumulated += now.Sub(t.workStartedAt)
			t.running = false
		}
	} else {
		t.pausedManually = false
		if !t.locked {
			t.running = true
			t.workStartedAt = now
		}
	}
	t.lastStateAt = now
	t.warned = false
	tele := t.tele
	chatID := t.cfg.TelegramChatID
	t.mu.Unlock()

	if paused {
		t.logf("⏸ Ручная пауза включена")
	} else {
		t.logf("▶️ Ручная пауза снята")
	}
	if tele != nil && chatID != 0 {
		tele.RefreshControls(t)
	}
}

func (t *Tracker) addTime(d time.Duration, source string) {
	if d <= 0 {
		return
	}
	t.mu.Lock()
	t.manualAdded += d
	t.lastStateAt = time.Now()
	total := t.currentTotalLocked(time.Now())
	t.mu.Unlock()
	t.logf("➕ Добавлено времени: %s (%s). Итого: %s", formatDuration(d), source, formatDuration(total))
}

func (t *Tracker) endSession(reason string) SessionSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return t.summaryLocked(time.Now())
	}
	now := time.Now()
	if t.running {
		t.accumulated += now.Sub(t.workStartedAt)
		t.running = false
	}
	t.ended = true
	t.lastStateAt = now
	msg := fmt.Sprintf("🏁 Сессия завершена (%s). Рабочее время: %s", reason, formatDuration(t.accumulated+t.manualAdded))
	log.Println(msg)
	if t.tele != nil && t.cfg.TelegramChatID != 0 {
		t.tele.SendLog(t.cfg.TelegramChatID, msg)
		t.tele.RefreshControls(t)
	}
	return t.summaryLocked(now)
}

func (t *Tracker) HandleActivity(idle time.Duration) {
	now := time.Now()
	if idle < time.Second {
		t.mu.Lock()
		t.lastInputAt = now
		paused := t.pausedManually
		locked := t.locked
		ended := t.ended
		wasWarned := t.warned
		t.warned = false
		t.mu.Unlock()
		if ended {
			return
		}
		if wasWarned {
			t.logf("✅ Активность возобновилась")
		}
		if !paused && !locked {
			t.startWork("обнаружена активность")
		}
		return
	}

	t.mu.Lock()
	if t.ended || t.pausedManually || t.locked {
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
	paused := t.pausedManually
	t.mu.Unlock()
	if already {
		return
	}
	if locked {
		t.logf("🔒 Экран заблокирован")
		t.stopWork("экран заблокирован")
		return
	}
	t.logf("🔓 Экран разблокирован")
	if !paused {
		t.startWork("экран разблокирован")
	}
}

func (t *Tracker) notifySoonPause(idle time.Duration) {
	text := fmt.Sprintf("Нет активности уже %s. Через %s подсчет времени остановится.", formatDuration(idle), formatDuration(t.cfg.StopAfterWarn.Duration))
	_ = sendDesktopNotification("Worktime tracker", text)
	t.logf("⚠️ %s", text)
}

func (t *Tracker) Summary() SessionSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.summaryLocked(time.Now())
}

func (t *Tracker) summaryLocked(now time.Time) SessionSummary {
	return SessionSummary{
		SessionStartedAt: t.sessionStartedAt,
		TotalWorked:      t.currentTotalLocked(now),
		TotalAdded:       t.manualAdded,
		Running:          t.running,
		PausedManually:   t.pausedManually,
		Locked:           t.locked,
		LastStateChange:  t.lastStateAt,
		Ended:            t.ended,
	}
}

func (t *Tracker) currentTotalLocked(now time.Time) time.Duration {
	total := t.accumulated + t.manualAdded
	if t.running {
		total += now.Sub(t.workStartedAt)
	}
	return total
}

type TelegramNotifier struct {
	bot             *tgbotapi.BotAPI
	controlsMessage int
	mu              sync.Mutex
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

func (n *TelegramNotifier) SendSessionStart(chatID int64, t *Tracker) {
	s := t.Summary()
	text := fmt.Sprintf("📅 Сессия стартовала \nСтарт: %s \n	Текущее состояние: %s \n Итого: %s", s.SessionStartedAt.Format(time.RFC3339), stateText(s), formatDuration(s.TotalWorked))
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = n.controlsMarkup(s)
	sent, err := n.bot.Send(msg)
	if err == nil {
		n.mu.Lock()
		n.controlsMessage = sent.MessageID
		n.mu.Unlock()
	}
}

func (n *TelegramNotifier) RefreshControls(t *Tracker) {
	s := t.Summary()
	n.mu.Lock()
	msgID := n.controlsMessage
	n.mu.Unlock()
	if msgID == 0 || t.cfg.TelegramChatID == 0 {
		return
	}
	text := fmt.Sprintf("📅 Сессия \n Старт: %s \nСостояние: %s \nИтого: %s", s.SessionStartedAt.Format(time.RFC3339), stateText(s), formatDuration(s.TotalWorked))
	edit := tgbotapi.NewEditMessageTextAndMarkup(t.cfg.TelegramChatID, msgID, text, n.controlsMarkup(s))
	_, _ = n.bot.Send(edit)
}

func (n *TelegramNotifier) controlsMarkup(s SessionSummary) tgbotapi.InlineKeyboardMarkup {
	stateBtnText := "⏸ Пауза"
	stateBtnData := "pause"
	if s.PausedManually || !s.Running {
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

func (n *TelegramNotifier) Run(ctx context.Context, tracker *Tracker, chatID int64) {
	updates := n.bot.GetUpdatesChan(tgbotapi.NewUpdate(0))
	for {
		select {
		case <-ctx.Done():
			return
		case upd := <-updates:
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
		n.SendSessionStart(msg.Chat.ID, tracker)
	case text == "/status":
		s := tracker.Summary()
		reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("Состояние: %s \n	Итого: %s", stateText(s), formatDuration(s.TotalWorked)))
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
	return &App{cfg: cfg, tracker: NewTracker(cfg)}
}

func (a *App) Run(ctx context.Context) error {
	printConfig(a.cfg)

	if a.cfg.TelegramToken != "" && a.cfg.TelegramChatID != 0 {
		tele, err := NewTelegramNotifier(a.cfg.TelegramToken)
		if err != nil {
			return fmt.Errorf("telegram init: %w", err)
		}
		a.tracker.SetTelegram(tele)
		tele.SendSessionStart(a.cfg.TelegramChatID, a.tracker)
		go tele.Run(ctx, a.tracker, a.cfg.TelegramChatID)
	}

	if a.cfg.HTTPPort > 0 {
		go a.runHTTP(ctx)
	}

	go a.runIdlePolling(ctx)
	go a.runLockPolling(ctx)

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

	srv := &http.Server{Addr: fmt.Sprintf(":%d", a.cfg.HTTPPort), Handler: mux}
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
	fmt.Printf("CONFIG: \n	%s \n", string(b))
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
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
	parts = append(parts, fmt.Sprintf("%ds", s))
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
	case s.Running:
		return "идет подсчет"
	default:
		return "ожидание активности"
	}
}

func defaultConfig() Config {
	return Config{
		IdleWarnAfter: Duration{Duration: defaultIdleWarnAfter},
		StopAfterWarn: Duration{Duration: defaultStopAfterWarn},
		PollInterval:  Duration{Duration: defaultPollInterval},
	}
}

func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, defaultConfigName)
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
	return nil
}

func main() {
	cfg, err := loadConfigFromArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := overrideFromFlags(&cfg, os.Args); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app := NewApp(cfg)
	if err := app.Run(ctx); err != nil {
		log.Fatalf("run: %v", err)
	}
}
