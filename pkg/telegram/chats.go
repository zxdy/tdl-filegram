package telegram

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/iyear/tdl/core/util/tutil"
	"go.uber.org/zap"
)

// Chat 是 Telegram 对话的轻量表示，不包含消息内容。
type Chat struct {
	ID          int64
	Type        string
	Title       string
	Username    string
	UnreadCount int
}

// ChatMessage 是列表页展示用的消息摘要。
type ChatMessage struct {
	ID         int
	Title      string
	Date       time.Time
	MediaType  string
	Size       int64
	MIME       string
	SourceURL  string
	HasMedia   bool
	HasPreview bool
}

// ListChats 获取当前账号最近的对话列表（最多 100 个）。
func (e *Engine) ListChats(ctx context.Context) ([]Chat, error) {
	dialogs, err := e.pool.Default(ctx).MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		Limit:      100,
		OffsetPeer: &tg.InputPeerEmpty{},
	})
	if err != nil {
		return nil, err
	}

	modified, ok := dialogs.AsModified()
	if !ok {
		return []Chat{}, nil
	}

	users := make(map[int64]*tg.User)
	for _, item := range modified.GetUsers() {
		if user, ok := item.(*tg.User); ok {
			users[user.ID] = user
		}
	}
	chats := make(map[int64]tg.ChatClass)
	for _, item := range modified.GetChats() {
		switch chat := item.(type) {
		case *tg.Chat:
			chats[chat.ID] = chat
		case *tg.Channel:
			chats[chat.ID] = chat
		}
	}

	result := make([]Chat, 0, len(modified.GetDialogs()))
	for _, item := range modified.GetDialogs() {
		dialog, ok := item.(*tg.Dialog)
		if !ok {
			continue
		}
		chat, ok := chatFromPeer(dialog.Peer, users, chats)
		if !ok {
			continue
		}
		chat.UnreadCount = dialog.UnreadCount
		result = append(result, chat)
	}
	return result, nil
}

func chatFromPeer(peer tg.PeerClass, users map[int64]*tg.User, chats map[int64]tg.ChatClass) (Chat, bool) {
	switch value := peer.(type) {
	case *tg.PeerUser:
		user, ok := users[value.UserID]
		if !ok {
			return Chat{}, false
		}
		title := strings.TrimSpace(user.FirstName + " " + user.LastName)
		if title == "" {
			title = user.Username
		}
		return Chat{ID: user.ID, Type: "私聊", Title: title, Username: user.Username}, true
	case *tg.PeerChat:
		chat, ok := chats[value.ChatID].(*tg.Chat)
		if !ok {
			return Chat{}, false
		}
		return Chat{ID: chat.ID, Type: "群组", Title: chat.Title}, true
	case *tg.PeerChannel:
		channel, ok := chats[value.ChannelID].(*tg.Channel)
		if !ok {
			return Chat{}, false
		}
		kind := "频道"
		if channel.Megagroup {
			kind = "超级群"
		}
		return Chat{ID: channel.ID, Type: kind, Title: channel.Title, Username: channel.Username}, true
	default:
		return Chat{}, false
	}
}

// ListChatMessages 获取一个对话最近的消息摘要。
func (e *Engine) ListChatMessages(ctx context.Context, chatType string, chatID int64, maxID, limit int) ([]ChatMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	peer, username, err := e.inputPeer(ctx, chatType, chatID)
	if err != nil {
		return nil, err
	}
	history, err := e.pool.Default(ctx).MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer: peer, MaxID: maxID, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	modified, ok := history.AsModified()
	if !ok {
		return []ChatMessage{}, nil
	}
	result := make([]ChatMessage, 0, len(modified.GetMessages()))
	for _, item := range modified.GetMessages() {
		message, ok := item.(*tg.Message)
		if !ok {
			continue
		}
		entry := ChatMessage{ID: message.ID, Title: message.Message, Date: time.Unix(int64(message.Date), 0)}
		if media, ok := tmedia.GetMedia(message); ok {
			entry.HasMedia = true
			entry.Size = media.Size
			entry.Title = firstNonEmpty(entry.Title, media.Name)
			entry.MediaType, entry.MIME, entry.HasPreview = messageMediaInfo(message)
			if username != "" {
				entry.SourceURL = fmt.Sprintf("https://t.me/%s/%d", username, message.ID)
			}
		}
		if entry.Title == "" {
			entry.Title = "（无文本内容）"
		}
		result = append(result, entry)
	}
	return result, nil
}

// DownloadChatThumbnail 下载图片或视频的预览图，供浏览器直接显示。
func (e *Engine) DownloadChatThumbnail(ctx context.Context, chatType string, chatID int64, messageID int) ([]byte, string, error) {
	peer, _, err := e.inputPeer(ctx, chatType, chatID)
	if err != nil {
		return nil, "", err
	}
	message, err := tutil.GetSingleMessage(ctx, e.pool.Default(ctx), peer, messageID)
	if err != nil {
		return nil, "", err
	}
	media, mime, ok := thumbnailMedia(message)
	if !ok {
		return nil, "", fmt.Errorf("thumbnail not found")
	}
	var output bytes.Buffer
	_, err = downloader.NewDownloader().Download(e.pool.Client(ctx, media.DC), media.InputFileLoc).Stream(ctx, &output)
	if err != nil {
		return nil, "", err
	}
	return output.Bytes(), mime, nil
}

func (e *Engine) inputPeer(ctx context.Context, chatType string, chatID int64) (tg.InputPeerClass, string, error) {
	// Bot API 将频道 ID 表示成 -100xxxxxxxxxx；MTProto 使用正的原始 ID。
	if chatID < -1000000000000 {
		chatID = -(chatID + 1000000000000)
	}
	if chatType == "频道" || chatType == "超级群" {
		if channel, err := e.manager.ResolveChannelID(ctx, chatID); err == nil {
			return channel.InputPeer(), channel.Raw().Username, nil
		}
	}
	dialogs, err := e.pool.Default(ctx).MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{Limit: 100, OffsetPeer: &tg.InputPeerEmpty{}})
	if err != nil {
		return nil, "", err
	}
	modified, ok := dialogs.AsModified()
	if !ok {
		return nil, "", fmt.Errorf("chat not found")
	}
	// 保存 access hash，让后续 /c/<channel>/<message> 链接也能被 TDL 解析。
	if err := e.manager.Apply(ctx, modified.GetUsers(), modified.GetChats()); err != nil {
		e.log.Debug("cache dialog peers failed", zap.Error(err))
	}
	for _, item := range modified.GetUsers() {
		if user, ok := item.(*tg.User); ok && chatType == "私聊" && user.ID == chatID {
			return &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash}, user.Username, nil
		}
	}
	for _, item := range modified.GetChats() {
		switch chat := item.(type) {
		case *tg.Chat:
			if chatType == "群组" && chat.ID == chatID {
				return &tg.InputPeerChat{ChatID: chat.ID}, "", nil
			}
		case *tg.Channel:
			if (chatType == "频道" || chatType == "超级群") && chat.ID == chatID {
				return &tg.InputPeerChannel{ChannelID: chat.ID, AccessHash: chat.AccessHash}, chat.Username, nil
			}
		}
	}
	return nil, "", fmt.Errorf("chat not found")
}

func messageMediaInfo(message *tg.Message) (string, string, bool) {
	media, ok := message.GetMedia()
	if !ok {
		return "文件", "", false
	}
	switch value := media.(type) {
	case *tg.MessageMediaPhoto:
		return "图片", "image/jpeg", true
	case *tg.MessageMediaDocument:
		document, ok := value.Document.(*tg.Document)
		if !ok {
			return "文件", "", false
		}
		for _, attr := range document.Attributes {
			if _, ok := attr.(*tg.DocumentAttributeVideo); ok {
				_, hasThumb := tmedia.GetDocumentThumb(document)
				return "视频", document.MimeType, hasThumb
			}
		}
		_, hasThumb := tmedia.GetDocumentThumb(document)
		return "文件", document.MimeType, hasThumb
	default:
		return "文件", "", false
	}
}

func thumbnailMedia(message *tg.Message) (*tmedia.Media, string, bool) {
	media, ok := message.GetMedia()
	if !ok {
		return nil, "", false
	}
	switch value := media.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := value.Photo.(*tg.Photo)
		if !ok || len(photo.Sizes) == 0 {
			return nil, "", false
		}
		for _, size := range photo.Sizes {
			switch size := size.(type) {
			case *tg.PhotoSize:
				return &tmedia.Media{InputFileLoc: &tg.InputPhotoFileLocation{ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference, ThumbSize: size.Type}, DC: photo.DCID}, "image/jpeg", true
			case *tg.PhotoSizeProgressive:
				return &tmedia.Media{InputFileLoc: &tg.InputPhotoFileLocation{ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference, ThumbSize: size.Type}, DC: photo.DCID}, "image/jpeg", true
			}
		}
	case *tg.MessageMediaDocument:
		if document, ok := value.Document.(*tg.Document); ok {
			if thumb, ok := tmedia.GetDocumentThumb(document); ok {
				return thumb, "image/jpeg", true
			}
		}
	}
	return nil, "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
