package telegram

import (
	"context"
	"d3k-agent/internal/core/ports"
	"fmt"
	"strconv"
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

	go ui.listen() // 배경에서 사용자 응답 대기

	return ui, nil
}

func (ui *TelegramUI) listen() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := ui.Bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery == nil {
			continue
		}

		// 버튼 클릭 처리
		callback := update.CallbackQuery
		action := ports.UserAction(callback.Data)
		
		ui.mu.Lock()
		// 가장 최근의 대기 중인 채널에 응답 전달 (간순화를 위해)
		// 실제로는 메시지 ID 매핑이 필요하나, 1인용 봇이므로 마지막 대기열 사용
		for msgID, ch := range ui.channels {
			ch <- action
			delete(ui.channels, msgID)
			
			// 사용자 피드백
			callbackConfig := tgbotapi.NewCallback(callback.ID, "선택 완료: "+string(action))
			ui.Bot.Request(callbackConfig)
			
			// 버튼 제거
			edit := tgbotapi.NewEditMessageReplyMarkup(ui.ChatID, update.CallbackQuery.Message.MessageID, tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}})
			ui.Bot.Send(edit)
			break
		}
		ui.mu.Unlock()
	}
}

func (ui *TelegramUI) Confirm(ctx context.Context, title, body string) (ports.UserAction, error) {
	msgText := fmt.Sprintf("*[%s]*\n\n%s", title, body)
	msg := tgbotapi.NewMessage(ui.ChatID, msgText)
	msg.ParseMode = "Markdown"

	// 인라인 버튼 생성
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

	// 응답 대기용 채널 생성
	respCh := make(chan ports.UserAction)
	msgKey := fmt.Sprintf("%d", sentMsg.MessageID)
	
	ui.mu.Lock()
	ui.channels[msgKey] = respCh
	ui.mu.Unlock()

	// 결과 수신 대기
	select {
	case action := <-respCh:
		return action, nil
	case <-ctx.Done():
		return ports.ActionSkip, ctx.Err()
	}
}
