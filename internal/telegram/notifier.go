package telegram

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"work-activity-tracker/internal/tracker"
)

type appController interface {
	Summary() tracker.SessionSummary
	AllInactivityTypes() []string
	AddTime(d time.Duration, source string)
	MoveActiveToInactive(d time.Duration, source string)
	AddInactivityType(name string) ([]string, error)
	SetCurrentInactivityType(name string) error
	SetManualPause(paused bool)
	StartNewDay(reason string) tracker.SessionSummary
	ContinueDay(reason string) tracker.SessionSummary
	EndSession(reason string) tracker.SessionSummary
}

type Notifier struct {
	bot    *tgbotapi.BotAPI
	chatID int64
	app    appController

	mu                sync.Mutex
	controlsMessageID int
}

func New(token string, chatID int64, controller appController) (*Notifier, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		bot:    bot,
		chatID: chatID,
		app:    controller,
	}, nil
}

func (n *Notifier) SendLog(text string) {
	msg := tgbotapi.NewMessage(n.chatID, text)
	_, _ = n.bot.Send(msg)
}

func (n *Notifier) RefreshControls() {
	n.SendOrReplaceControls()
}

func (n *Notifier) SendOrReplaceControls() {
	s := n.app.Summary()
	text := n.sessionText(s)
	markup := n.controlsMarkup(s)

	n.mu.Lock()
	msgID := n.controlsMessageID
	n.mu.Unlock()

	if msgID == 0 {
		msg := tgbotapi.NewMessage(n.chatID, text)
		msg.ReplyMarkup = markup
		sent, err := n.bot.Send(msg)
		if err == nil {
			n.mu.Lock()
			n.controlsMessageID = sent.MessageID
			n.mu.Unlock()
		}
		return
	}

	edit := tgbotapi.NewEditMessageTextAndMarkup(n.chatID, msgID, text, markup)
	_, err := n.bot.Send(edit)
	if err != nil {
		msg := tgbotapi.NewMessage(n.chatID, text)
		msg.ReplyMarkup = markup
		sent, err2 := n.bot.Send(msg)
		if err2 == nil {
			n.mu.Lock()
			n.controlsMessageID = sent.MessageID
			n.mu.Unlock()
		}
	}
}

func (n *Notifier) Run(ctx context.Context) {
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
				if upd.Message.Chat == nil || upd.Message.Chat.ID != n.chatID {
					continue
				}
				n.handleMessage(upd.Message)
			}

			if upd.CallbackQuery != nil {
				if upd.CallbackQuery.Message == nil || upd.CallbackQuery.Message.Chat == nil || upd.CallbackQuery.Message.Chat.ID != n.chatID {
					continue
				}
				n.handleCallback(upd.CallbackQuery)
			}
		}
	}
}

func (n *Notifier) sessionText(s tracker.SessionSummary) string {
	title := tracker.EmptyFallback(s.Window.Title, "(не определено)")
	gtkAppID := tracker.EmptyFallback(s.Window.GTKApplicationID, "-")
	kdeDesktopFile := tracker.EmptyFallback(s.Window.KDEDesktopFile, "-")
	wmClass := tracker.EmptyFallback(s.Window.WMClass, "-")
	startedAt := "-"
	if s.Started {
		startedAt = s.SessionStartedAt.Format(time.RFC3339)
	}

	blockReason := ""
	if s.Window.BlockedByRule {
		blockReason = fmt.Sprintf(
			"\nСовпадение: поле=%s, подстрока=%s",
			tracker.EmptyFallback(s.Window.MatchedField, "-"),
			tracker.EmptyFallback(s.Window.MatchedSubstring, "-"),
		)
	}

	return fmt.Sprintf(
		"📅 Сессия\nСтарт: %s\nСостояние: %s\nТип неактивности: %s\nИтого активности: %s\nИтого неактивности: %s\nОкно: %s\nGTK_APPLICATION_ID: %s\nKDE_NET_WM_DESKTOP_FILE: %s\nWM_CLASS: %s%s",
		startedAt,
		tracker.StateText(s),
		tracker.EmptyFallback(s.CurrentInactivityType, "-"),
		tracker.FormatDuration(s.TotalActive),
		tracker.FormatDuration(s.TotalInactive),
		title,
		gtkAppID,
		kdeDesktopFile,
		wmClass,
		blockReason,
	)
}

func (n *Notifier) controlsMarkup(s tracker.SessionSummary) tgbotapi.InlineKeyboardMarkup {
	stateBtnText := "⏸ Пауза"
	stateBtnData := "pause"
	dayBtnText := "🏁 Завершить день"
	dayBtnData := "end"

	if !s.Started || s.Ended || s.PausedManually || !s.Running {
		stateBtnText = "▶️ Старт"
		stateBtnData = "start"
	}
	if !s.Started || s.Ended {
		dayBtnText = "📅 Начать новый день"
		dayBtnData = "newday"
	}

	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(stateBtnText, stateBtnData),
			tgbotapi.NewInlineKeyboardButtonData(dayBtnText, dayBtnData),
		},
	}
	if s.CanContinueDay && (!s.Started || s.Ended) {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("🔄 Продолжить день", "continue"),
		})
	}
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("➕ 30м", "add:30m"),
		tgbotapi.NewInlineKeyboardButtonData("➕ 1ч", "add:1h"),
		tgbotapi.NewInlineKeyboardButtonData("➕ 2ч", "add:2h"),
	})
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("➖ 10м", "sub:10m"),
		tgbotapi.NewInlineKeyboardButtonData("➖ 20м", "sub:20m"),
		tgbotapi.NewInlineKeyboardButtonData("➖ 30м", "sub:30m"),
	})

	return tgbotapi.NewInlineKeyboardMarkup(buttons...)
}

func (n *Notifier) handleMessage(msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.Text)

	switch {
	case text == "/start":
		n.SendOrReplaceControls()

	case text == "/status":
		s := n.app.Summary()
		reply := tgbotapi.NewMessage(
			msg.Chat.ID,
			fmt.Sprintf(
				"Состояние: %s\nТип неактивности: %s\nИтого активности: %s\nИтого неактивности: %s\nОкно: %s\nGTK_APPLICATION_ID: %s\nKDE_NET_WM_DESKTOP_FILE: %s\nWM_CLASS: %s",
				tracker.StateText(s),
				tracker.EmptyFallback(s.CurrentInactivityType, "-"),
				tracker.FormatDuration(s.TotalActive),
				tracker.FormatDuration(s.TotalInactive),
				tracker.EmptyFallback(s.Window.Title, "(не определено)"),
				tracker.EmptyFallback(s.Window.GTKApplicationID, "-"),
				tracker.EmptyFallback(s.Window.KDEDesktopFile, "-"),
				tracker.EmptyFallback(s.Window.WMClass, "-"),
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
		n.app.AddTime(d, "telegram command")
		n.RefreshControls()

	case strings.HasPrefix(text, "/sub "):
		v := strings.TrimSpace(strings.TrimPrefix(text, "/sub "))
		d, err := time.ParseDuration(v)
		if err != nil {
			_, _ = n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Используй формат: /sub 30m"))
			return
		}
		n.app.MoveActiveToInactive(d, "telegram command")
		n.RefreshControls()

	case text == "/itypes":
		types := n.app.AllInactivityTypes()
		_, _ = n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Типы неактивности:\n- "+strings.Join(types, "\n- ")))

	case strings.HasPrefix(text, "/itype "):
		name := strings.TrimSpace(strings.TrimPrefix(text, "/itype "))
		if err := n.app.SetCurrentInactivityType(name); err != nil {
			_, _ = n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, err.Error()))
			return
		}
		n.RefreshControls()

	case strings.HasPrefix(text, "/additype "):
		name := strings.TrimSpace(strings.TrimPrefix(text, "/additype "))
		types, err := n.app.AddInactivityType(name)
		if err != nil {
			_, _ = n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, err.Error()))
			return
		}
		_, _ = n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Добавлен тип неактивности.\n- "+strings.Join(types, "\n- ")))

	case text == "/pause":
		n.app.SetManualPause(true)

	case text == "/resume":
		s := n.app.Summary()
		if !s.Started || s.Ended {
			n.app.StartNewDay("telegram command")
		} else {
			n.app.SetManualPause(false)
		}

	case text == "/end":
		n.app.EndSession("telegram command")

	case text == "/newday":
		n.app.StartNewDay("telegram command")

	case text == "/continue":
		n.app.ContinueDay("telegram command")

	default:
		_, _ = n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Команды: /start, /status, /newday, /continue, /add 1h30m, /sub 30m, /itypes, /itype перекус, /additype прогулка, /pause, /resume, /end"))
	}
}

func (n *Notifier) handleCallback(q *tgbotapi.CallbackQuery) {
	data := q.Data

	switch {
	case data == "noop":
	case data == "pause":
		n.app.SetManualPause(true)
	case data == "start":
		s := n.app.Summary()
		if !s.Started || s.Ended {
			n.app.StartNewDay("telegram button")
		} else {
			n.app.SetManualPause(false)
		}
	case data == "end":
		n.app.EndSession("telegram button")
	case data == "newday":
		n.app.StartNewDay("telegram button")
	case data == "continue":
		n.app.ContinueDay("telegram button")
	case strings.HasPrefix(data, "add:"):
		d, err := time.ParseDuration(strings.TrimPrefix(data, "add:"))
		if err == nil {
			n.app.AddTime(d, "telegram button")
		}
	case strings.HasPrefix(data, "sub:"):
		d, err := time.ParseDuration(strings.TrimPrefix(data, "sub:"))
		if err == nil {
			n.app.MoveActiveToInactive(d, "telegram button")
		}
	}

	answer := tgbotapi.NewCallback(q.ID, "OK")
	_, _ = n.bot.Request(answer)
	n.RefreshControls()
}
