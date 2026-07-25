package logic

import (
	"context"

	"tdl-filegram/enum"
	"tdl-filegram/internal/dto/res"
	"tdl-filegram/pkg/telegram"
)

// ChatLogic 聊天列表业务编排。
type ChatLogic struct {
	engine *telegram.Engine
}

func NewChatLogic(engine *telegram.Engine) *ChatLogic {
	return &ChatLogic{engine: engine}
}

func (l *ChatLogic) List(ctx context.Context) ([]res.ChatRes, error) {
	if !l.engine.IsReady() {
		return nil, enum.ErrTelegramNotReady
	}
	if !l.engine.IsAuthenticated(ctx) {
		return nil, enum.ErrUnauthorized
	}
	items, err := l.engine.ListChats(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]res.ChatRes, 0, len(items))
	for _, item := range items {
		result = append(result, res.ChatRes{
			ID:          item.ID,
			Type:        item.Type,
			Title:       item.Title,
			Username:    item.Username,
			UnreadCount: item.UnreadCount,
		})
	}
	return result, nil
}

func (l *ChatLogic) Messages(ctx context.Context, chatType string, chatID int64, maxID, limit int) ([]res.ChatMessageRes, error) {
	if !l.engine.IsReady() {
		return nil, enum.ErrTelegramNotReady
	}
	if !l.engine.IsAuthenticated(ctx) {
		return nil, enum.ErrUnauthorized
	}
	items, err := l.engine.ListChatMessages(ctx, chatType, chatID, maxID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]res.ChatMessageRes, 0, len(items))
	for _, item := range items {
		result = append(result, res.ChatMessageRes{ID: item.ID, Title: item.Title, Date: item.Date, MediaType: item.MediaType, Size: item.Size, MIME: item.MIME, SourceURL: item.SourceURL, HasMedia: item.HasMedia, HasPreview: item.HasPreview})
	}
	return result, nil
}

func (l *ChatLogic) Thumbnail(ctx context.Context, chatType string, chatID int64, messageID int) ([]byte, string, error) {
	if !l.engine.IsReady() {
		return nil, "", enum.ErrTelegramNotReady
	}
	if !l.engine.IsAuthenticated(ctx) {
		return nil, "", enum.ErrUnauthorized
	}
	return l.engine.DownloadChatThumbnail(ctx, chatType, chatID, messageID)
}
