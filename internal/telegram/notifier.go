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

type Notifier struct {
	bot     *tgbotapi.BotAPI
	chatID  int64
	tracker *tracker.Tracker

	mu                sync.Mutex
	controlsMessageID int
}

func New(token string, chatID int64, tr *tracker.Tracker) (*Notifier, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		bot:     bot,
		chatID:  chatID,
		tracker: tr,
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
	s := n.tracker.Summary()
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

	blockReason := ""
	if s.Window.BlockedByRule {
		blockReason = fmt.Sprintf(
			"\nСовпадение: поле=%s, подстрока=%s",
			tracker.EmptyFallback(s.Window.MatchedField, "-"),
			tracker.EmptyFallback(s.Window.MatchedSubstring, "-"),
		)
	}

	return fmt.Sprintf(
		"📅 Сессия\nСтарт: %s\nСостояние: %s\nИтого активности: %s\nИтого неактивности: %s\nОкно: %s\nGTK_APPLICATION_ID: %s\nKDE_NET_WM_DESKTOP_FILE: %s\nWM_CLASS: %s%s",
		s.SessionStartedAt.Format(time.RFC3339),
		tracker.StateText(s),
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

func (n *Notifier) handleMessage(msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.Text)

	switch {
	case text == "/start":
		n.SendOrReplaceControls()

	case text == "/status":
		s := n.tracker.Summary()
		reply := tgbotapi.NewMessage(
			msg.Chat.ID,
			fmt.Sprintf(
				"Состояние: %s\nИтого активности: %s\nИтого неактивности: %s\nОкно: %s\nGTK_APPLICATION_ID: %s\nKDE_NET_WM_DESKTOP_FILE: %s\nWM_CLASS: %s",
				tracker.StateText(s),
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
		n.tracker.AddTime(d, "telegram command")
		n.RefreshControls()

	case text == "/pause":
		n.tracker.SetManualPause(true)

	case text == "/resume":
		n.tracker.SetManualPause(false)

	case text == "/end":
		n.tracker.EndSession("telegram command")

	default:
		_, _ = n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "Команды: /start, /status, /add 1h30m, /pause, /resume, /end"))
	}
}

func (n *Notifier) handleCallback(q *tgbotapi.CallbackQuery) {
	data := q.Data

	switch {
	case data == "noop":
	case data == "pause":
		n.tracker.SetManualPause(true)
	case data == "start":
		n.tracker.SetManualPause(false)
	case data == "end":
		n.tracker.EndSession("telegram button")
	case strings.HasPrefix(data, "add:"):
		d, err := time.ParseDuration(strings.TrimPrefix(data, "add:"))
		if err == nil {
			n.tracker.AddTime(d, "telegram button")
		}
	}

	answer := tgbotapi.NewCallback(q.ID, "OK")
	_, _ = n.bot.Request(answer)
	n.RefreshControls()
}
