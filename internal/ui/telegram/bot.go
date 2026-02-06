package telegram

import (
	"context"
	"d3k-agent/internal/core/ports"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramUI struct {
	Bot      *tgbotapi.BotAPI
	ChatID   int64
	lastResp ports.UserAction
	respMu   sync.Mutex
	lastMsgID int
}

func NewTelegramUI(token string, chatIDStr string) (*TelegramUI, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil { return nil, err }

	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil { return nil, err }

	ui := &TelegramUI{
		Bot:    bot,
		ChatID: chatID,
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
		ui.respMu.Lock()
		// 사용자가 누른 버튼의 값을 보관
		ui.lastResp = ports.UserAction(callback.Data)
		ui.lastMsgID = callback.Message.MessageID
		ui.respMu.Unlock()

		// 버튼 클릭 피드백
		ui.Bot.Request(tgbotapi.NewCallback(callback.ID, "선택됨: "+callback.Data))
		
		// 버튼 제거
		edit := tgbotapi.NewEditMessageReplyMarkup(ui.ChatID, callback.Message.MessageID, tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}})
		ui.Bot.Send(edit)
	}
}

func (ui *TelegramUI) Confirm(ctx context.Context, title, body string) (ports.UserAction, error) {
	msgText := fmt.Sprintf("*[%s]*\n\n%s", escapeMarkdown(title), escapeMarkdown(body))
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
	if err != nil { return ports.ActionSkip, err }

	// 응답 대기 루프 (Polling 응답 대기)
	for {
		ui.respMu.Lock()
		// 방금 보낸 메시지 ID에 대한 응답인지 확인
		if ui.lastMsgID == sentMsg.MessageID && ui.lastResp != "" {
			action := ui.lastResp
			ui.lastResp = "" // 초기화
			ui.respMu.Unlock()
			return action, nil
		}
		ui.respMu.Unlock()

		select {
		case <-time.After(500 * time.Millisecond):
			continue
		case <-ctx.Done():
			return ports.ActionSkip, ctx.Err()
		}
	}
}

func escapeMarkdown(text string) string {
	replacer := strings.NewReplacer("_", "\\_", "*", "\\*", "[", "\\[", "`", "\\`", "(", "\\(", ")", "\\)")
	return replacer.Replace(text)
}
