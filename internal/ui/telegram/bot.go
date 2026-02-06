package telegram

import (
	"context"
	"d3k-agent/internal/core/ports"
	"fmt"
	"strconv"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramUI는 텔레그램을 통한 사용자 승인 인터페이스를 제공합니다.
type TelegramUI struct {
	Bot      *tgbotapi.BotAPI
	ChatID   int64
	channels map[string]chan ports.UserAction
	mu       sync.Mutex
}

func NewTelegramUI(token string, chatIDStr string) (*TelegramUI, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid chat id: %v", err)
	}

	ui := &TelegramUI{
		Bot:      bot,
		ChatID:   chatID,
		channels: make(map[string]chan ports.UserAction),
	}

	go ui.listen()
	return ui, nil
}

func (ui *TelegramUI) listen() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := ui.Bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery == nil { continue }

		callback := update.CallbackQuery
		action := ports.UserAction(callback.Data)
		
		ui.mu.Lock()
		for msgID, ch := range ui.channels {
			ch <- action
			delete(ui.channels, msgID)
			
			callbackConfig := tgbotapi.NewCallback(callback.ID, "선택 완료: "+string(action))
			ui.Bot.Request(callbackConfig)
			
			edit := tgbotapi.NewEditMessageReplyMarkup(ui.ChatID, update.CallbackQuery.Message.MessageID, tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}})
			ui.Bot.Send(edit)
			break
		}
		ui.mu.Unlock()
	}
}

func (ui *TelegramUI) Confirm(ctx context.Context, title, body string) (ports.UserAction, error) {
	// 마크다운 특수문자 이스케이프 처리 (Best Practice)
	safeTitle := escapeMarkdown(title)
	safeBody := escapeMarkdown(body)

	msgText := fmt.Sprintf("*[%s]*\n\n%s", safeTitle, safeBody)
	msg := tgbotapi.NewMessage(ui.ChatID, msgText)
	msg.ParseMode = "Markdown"

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ 승인", string(ports.ActionApprove)),
			tgbotapi.NewInlineKeyboardButtonData("🔄 재구성", string(ports.ActionRegenerate)),
			tgbotapi.NewInlineKeyboardButtonData("❌ 거절", string(ports.ActionSkip)),
		),
	)

	sentMsg, err := ui.Bot.Send(msg)
	if err != nil {
		return ports.ActionSkip, err
	}

	respCh := make(chan ports.UserAction)
	msgKey := fmt.Sprintf("%d", sentMsg.MessageID)
	
	ui.mu.Lock()
	ui.channels[msgKey] = respCh
	ui.mu.Unlock()

	select {
	case action := <-respCh:
		return action, nil
	case <-ctx.Done():
		return ports.ActionSkip, ctx.Err()
	}
}

// escapeMarkdown은 텔레그램 마크다운 파싱 에러를 방지하기 위해 특수문자를 이스케이프합니다.
func escapeMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"`", "\\`",
	)
	return replacer.Replace(text)
}