package trayapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/getlantern/systray"

	"work-activity-tracker/internal/trayconfig"
)

type Status struct {
	Started             bool          `json:"started"`
	CanContinueDay      bool          `json:"can_continue_day"`
	SessionStartedAt    time.Time     `json:"session_started_at"`
	TotalActive         time.Duration `json:"total_active"`
	TotalInactive       time.Duration `json:"total_inactive"`
	TotalAdded          time.Duration `json:"total_added"`
	CurrentActivityType string        `json:"current_activity_type"`
	Running             bool          `json:"running"`
	PausedManually      bool          `json:"paused_manually"`
	Locked              bool          `json:"locked"`
	BlockedByWindow     bool          `json:"blocked_by_window"`
	LastStateChange     time.Time     `json:"last_state_change"`
	Ended               bool          `json:"ended"`
}

type App struct {
	cfg    trayconfig.Config
	client *http.Client

	activeItem       *systray.MenuItem
	inactiveItem     *systray.MenuItem
	stateItem        *systray.MenuItem
	activityTypeItem *systray.MenuItem
	errorItem        *systray.MenuItem

	refreshItem     *systray.MenuItem
	startItem       *systray.MenuItem
	pauseItem       *systray.MenuItem
	newDayItem      *systray.MenuItem
	continueDayItem *systray.MenuItem
	add30Item       *systray.MenuItem
	add1hItem       *systray.MenuItem
	add2hItem       *systray.MenuItem
	sub10Item       *systray.MenuItem
	sub20Item       *systray.MenuItem
	sub30Item       *systray.MenuItem
	endItem         *systray.MenuItem
	quitItem        *systray.MenuItem

	lastStatus *Status
}

func New(cfg trayconfig.Config) *App {
	return &App{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.RequestTimeout.Duration,
		},
	}
}

func (a *App) Run() {
	systray.Run(a.onReady, nil)
}

func (a *App) onReady() {
	systray.SetTitle("WAT")
	systray.SetTooltip("Work Activity Tracker Tray")
	systray.SetIcon(idleIcon())

	a.activeItem = systray.AddMenuItem("Активность: ...", "")
	a.activeItem.Disable()
	a.inactiveItem = systray.AddMenuItem("Неактивность: ...", "")
	a.inactiveItem.Disable()
	a.stateItem = systray.AddMenuItem("Состояние: ...", "")
	a.stateItem.Disable()
	a.activityTypeItem = systray.AddMenuItem("Тип активности: ...", "")
	a.activityTypeItem.Disable()
	a.errorItem = systray.AddMenuItem("", "")
	a.errorItem.Disable()
	a.errorItem.Hide()

	systray.AddSeparator()

	a.refreshItem = systray.AddMenuItem("Обновить", "Обновить статус")
	a.startItem = systray.AddMenuItem("Старт / Возобновить", "Старт или возобновление через API")
	a.pauseItem = systray.AddMenuItem("Пауза", "Пауза через API")
	a.newDayItem = systray.AddMenuItem("Начать новый день", "Начать новый день")
	a.continueDayItem = systray.AddMenuItem("Продолжить день", "Продолжить последний день")
	a.add30Item = systray.AddMenuItem("Добавить 30м", "Добавить 30 минут")
	a.add1hItem = systray.AddMenuItem("Добавить 1ч", "Добавить 1 час")
	a.add2hItem = systray.AddMenuItem("Добавить 2ч", "Добавить 2 часа")
	a.sub10Item = systray.AddMenuItem("Вычесть 10м", "Перенести 10 минут в неактивное")
	a.sub20Item = systray.AddMenuItem("Вычесть 20м", "Перенести 20 минут в неактивное")
	a.sub30Item = systray.AddMenuItem("Вычесть 30м", "Перенести 30 минут в неактивное")
	a.endItem = systray.AddMenuItem("Завершить день", "Завершить день")

	systray.AddSeparator()
	a.quitItem = systray.AddMenuItem("Выход", "Закрыть tray")

	go a.pollLoop()
	go a.handleActions()
	go a.refresh()
}

func (a *App) pollLoop() {
	ticker := time.NewTicker(a.cfg.PollInterval.Duration)
	defer ticker.Stop()

	for range ticker.C {
		a.refresh()
	}
}

func (a *App) handleActions() {
	for {
		select {
		case <-a.refreshItem.ClickedCh:
			go a.refresh()
		case <-a.startItem.ClickedCh:
			go a.callAction("/start", nil)
		case <-a.pauseItem.ClickedCh:
			go a.callAction("/pause", nil)
		case <-a.newDayItem.ClickedCh:
			go a.callAction("/new-day", nil)
		case <-a.continueDayItem.ClickedCh:
			go a.callAction("/continue-day", nil)
		case <-a.add30Item.ClickedCh:
			go a.callAction("/add?minutes=30", nil)
		case <-a.add1hItem.ClickedCh:
			go a.callAction("/add?minutes=60", nil)
		case <-a.add2hItem.ClickedCh:
			go a.callAction("/add?minutes=120", nil)
		case <-a.sub10Item.ClickedCh:
			go a.callAction("/subtract?minutes=10", nil)
		case <-a.sub20Item.ClickedCh:
			go a.callAction("/subtract?minutes=20", nil)
		case <-a.sub30Item.ClickedCh:
			go a.callAction("/subtract?minutes=30", nil)
		case <-a.endItem.ClickedCh:
			go a.callAction("/end", nil)
		case <-a.quitItem.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *App) refresh() {
	status, err := a.fetchStatus()
	if err != nil {
		a.renderError(err)
		return
	}

	a.lastStatus = &status
	a.errorItem.Hide()
	a.activeItem.SetTitle("Активность: " + formatDuration(status.TotalActive))
	a.inactiveItem.SetTitle("Неактивность: " + formatDuration(status.TotalInactive))
	a.stateItem.SetTitle("Состояние: " + stateText(status))
	a.activityTypeItem.SetTitle("Тип активности: " + emptyFallback(status.CurrentActivityType, "-"))
	a.updateMenuAvailability(status)
	a.updateIcon(status)
}

func (a *App) renderError(err error) {
	systray.SetIcon(errorIcon())
	a.errorItem.SetTitle("Ошибка API: " + err.Error())
	a.errorItem.Show()
	a.activeItem.SetTitle("Активность: недоступно")
	a.inactiveItem.SetTitle("Неактивность: недоступно")
	a.stateItem.SetTitle("Состояние: ошибка API")
	a.activityTypeItem.SetTitle("Тип активности: недоступно")
	a.continueDayItem.Disable()
}

func (a *App) updateMenuAvailability(s Status) {
	a.startItem.Enable()
	a.pauseItem.Enable()
	a.newDayItem.Enable()
	a.add30Item.Enable()
	a.add1hItem.Enable()
	a.add2hItem.Enable()
	a.sub10Item.Enable()
	a.sub20Item.Enable()
	a.sub30Item.Enable()
	a.endItem.Enable()

	if !s.Started || s.Ended {
		a.pauseItem.Disable()
		a.add30Item.Disable()
		a.add1hItem.Disable()
		a.add2hItem.Disable()
		a.sub10Item.Disable()
		a.sub20Item.Disable()
		a.sub30Item.Disable()
		a.endItem.Disable()
	}

	if s.CanContinueDay && (!s.Started || s.Ended) {
		a.continueDayItem.Show()
		a.continueDayItem.Enable()
	} else {
		a.continueDayItem.Hide()
	}
}

func (a *App) updateIcon(s Status) {
	switch {
	case !s.Started || s.Ended:
		systray.SetIcon(idleIcon())
	case s.Running:
		systray.SetIcon(activeIcon())
	default:
		systray.SetIcon(pausedIcon())
	}
}

func (a *App) fetchStatus() (Status, error) {
	var status Status
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, strings.TrimRight(a.cfg.APIBaseURL, "/")+"/status", nil)
	if err != nil {
		return status, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return status, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return status, readAPIError(resp.Body)
	}

	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return status, err
	}
	return status, nil
}

func (a *App) callAction(path string, body []byte) {
	method := http.MethodPost
	if strings.HasPrefix(path, "/add?") {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		method,
		strings.TrimRight(a.cfg.APIBaseURL, "/")+path,
		bytes.NewReader(body),
	)
	if err != nil {
		a.renderError(err)
		return
	}

	resp, err := a.client.Do(req)
	if err != nil {
		a.renderError(err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		a.renderError(readAPIError(resp.Body))
		return
	}

	go a.refresh()
}

func readAPIError(r io.Reader) error {
	var payload map[string]any
	if err := json.NewDecoder(r).Decode(&payload); err == nil {
		if message, ok := payload["error"].(string); ok && message != "" {
			return fmt.Errorf("%s", message)
		}
	}
	return fmt.Errorf("request failed")
}

func stateText(s Status) string {
	switch {
	case !s.Started && s.CanContinueDay:
		return "можно продолжить день"
	case !s.Started:
		return "день не начат"
	case s.Ended && s.CanContinueDay:
		return "день завершен, можно продолжить"
	case s.Ended:
		return "сессия завершена"
	case s.PausedManually:
		return "ручная пауза"
	case s.Locked:
		return "экран заблокирован"
	case s.BlockedByWindow:
		return "остановлено по окну"
	case s.Running:
		return "идет подсчет"
	default:
		return "ожидание активности"
	}
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

func emptyFallback(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
