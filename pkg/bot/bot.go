// Package bot implements the small Bot API surface needed to accept download links.
package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"tdl-filegram/internal/dto/req"
	"tdl-filegram/internal/logic"
)

var linkPattern = regexp.MustCompile(`https?://t\.me/[^\s]+`)

type Config struct {
	Token          string
	AllowedUserIDs []int64
	Proxy          string
	AppID          int
	AppHash        string
}

type Client struct {
	baseURL   string
	token     string
	appID     int
	appHash   string
	proxy     string
	allowed   map[int64]struct{}
	http      *http.Client
	downloads *logic.DownloadLogic
	log       *zap.Logger

	mu       sync.Mutex
	lastEdit map[string]time.Time
	seen     map[int64]int
	albums   map[string]*album
}

func New(cfg Config, downloads *logic.DownloadLogic, log *zap.Logger) *Client {
	allowed := make(map[int64]struct{}, len(cfg.AllowedUserIDs))
	for _, id := range cfg.AllowedUserIDs {
		allowed[id] = struct{}{}
	}
	transport := &http.Transport{}
	if cfg.Proxy != "" {
		if proxyURL, err := url.Parse(cfg.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		} else {
			log.Warn("invalid bot proxy, using direct connection", zap.Error(err))
		}
	}
	return &Client{
		baseURL:   "https://api.telegram.org/bot" + cfg.Token,
		token:     cfg.Token,
		appID:     cfg.AppID,
		appHash:   cfg.AppHash,
		proxy:     cfg.Proxy,
		allowed:   allowed,
		http:      &http.Client{Timeout: 35 * time.Second, Transport: transport},
		downloads: downloads,
		log:       log,
		lastEdit:  make(map[string]time.Time),
		seen:      make(map[int64]int),
		albums:    make(map[string]*album),
	}
}

// Enabled requires a bot token. An empty allow list permits every user.
func Enabled(cfg Config) bool { return cfg.Token != "" }

func (b *Client) Run(ctx context.Context) {
	var me botUser
	if err := b.call(ctx, "getMe", map[string]any{}, &me); err != nil {
		b.log.Warn("bot connection failed", zap.Error(err))
		return
	}
	b.log.Info("bot connected via Bot API", zap.Int64("bot_id", me.ID), zap.String("username", me.Username))
	// Long polling and webhook are mutually exclusive. Keep pending updates so
	// messages sent while the service was offline are still processed.
	if err := b.call(ctx, "deleteWebhook", map[string]any{"drop_pending_updates": false}, nil); err != nil {
		b.log.Warn("bot deleteWebhook failed", zap.Error(err))
		return
	}
	b.log.Info("bot webhook disabled; starting long polling")
	b.notifyAllowedUsers(ctx)
	var offset int64
	for ctx.Err() == nil {
		var updates []update
		if err := b.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 25, "allowed_updates": []string{"message"}}, &updates); err != nil {
			if ctx.Err() == nil {
				b.log.Warn("bot getUpdates failed", zap.Error(err))
			}
			continue
		}
		for _, item := range updates {
			offset = item.UpdateID + 1
			if item.Message == nil {
				continue
			}
			go b.handleMessage(ctx, item.Message)
		}
	}
}

func (b *Client) handleMessage(ctx context.Context, message *incomingMessage) {
	userID, chatID := message.From.ID, message.Chat.ID
	text := message.Text
	if text == "" {
		text = message.Caption
	}
	if len(b.allowed) > 0 {
		if _, ok := b.allowed[userID]; !ok {
			b.log.Warn("bot message rejected: user not allowed", zap.Int64("user_id", userID), zap.Int64("chat_id", chatID), zap.String("text", text))
			return
		}
	}
	b.log.Info("bot message received", zap.Int64("user_id", userID), zap.Int64("chat_id", chatID), zap.String("text", text))
	if !message.hasMedia() {
		b.handleLinks(ctx, userID, chatID, text)
		return
	}

	source, ok := message.sourceURL()
	if !ok {
		b.log.Warn("bot forwarded media has no readable source", zap.Int64("user_id", userID), zap.Int64("chat_id", chatID))
		_, _ = b.send(ctx, chatID, "无法定位这条转发媒体的原频道。请发送原消息链接，或确认该频道对已登录的 Telegram 账号可见。")
		return
	}
	if message.MediaGroupID == "" {
		b.createForwarded(ctx, chatID, []string{source})
		return
	}
	b.queueAlbum(ctx, chatID, message.MediaGroupID, source)
}

// notifyAllowedUsers is a startup connectivity check and also confirms that
// the configured Bot API token can reach every allowed private chat.
func (b *Client) notifyAllowedUsers(ctx context.Context) {
	for userID := range b.allowed {
		if _, err := b.send(ctx, userID, "下载 Bot 已启动，可以直接发送 t.me 链接。 "); err != nil {
			b.log.Warn("bot startup message failed", zap.Int64("user_id", userID), zap.Error(err))
			continue
		}
		b.log.Info("bot startup message sent", zap.Int64("user_id", userID))
	}
}

func (b *Client) handleText(ctx context.Context, userID, chatID int64, text string) {
	if len(b.allowed) > 0 {
		if _, ok := b.allowed[userID]; !ok {
			b.log.Warn("bot message rejected: user not allowed", zap.Int64("user_id", userID), zap.Int64("chat_id", chatID), zap.String("text", text))
			return
		}
	}
	b.log.Info("bot message received", zap.Int64("user_id", userID), zap.Int64("chat_id", chatID), zap.String("text", text))
	b.handleLinks(ctx, userID, chatID, text)
}

func (b *Client) handleLinks(ctx context.Context, userID, chatID int64, text string) {
	links := linkPattern.FindAllString(text, -1)
	if len(links) == 0 {
		b.log.Debug("bot message ignored: no telegram link", zap.Int64("user_id", userID), zap.Int64("chat_id", chatID))
		b.send(ctx, chatID, "请发送一个 Telegram 消息链接，例如 https://t.me/channel/123")
		return
	}
	b.log.Info("bot received download links", zap.Int64("user_id", userID), zap.Int64("chat_id", chatID), zap.Int("count", len(links)))
	for _, link := range links {
		link = strings.TrimRight(link, ".,!?，。！）)")
		b.log.Info("bot parsing link", zap.String("url", link), zap.Int64("user_id", userID))
		result, err := b.downloads.Create(ctx, req.CreateDownloadReq{URL: link})
		if err != nil {
			b.log.Warn("bot create download failed", zap.String("url", link), zap.Error(err))
			b.send(ctx, chatID, "创建下载任务失败："+err.Error())
			continue
		}
		b.log.Info("bot download task created", zap.String("job_id", result.JobID), zap.String("url", link))
		statusID, err := b.send(ctx, chatID, "已创建下载任务，正在解析媒体…")
		if err != nil {
			b.log.Warn("bot send status failed", zap.Error(err))
			continue
		}
		b.watch(result.JobID, chatID, statusID)
	}
}

type album struct {
	chatID  int64
	sources map[string]struct{}
}

// queueAlbum waits briefly for Telegram to deliver every item in a forwarded
// album, then creates one download task per original message.
func (b *Client) queueAlbum(ctx context.Context, chatID int64, groupID, source string) {
	key := fmt.Sprintf("%d:%s", chatID, groupID)
	b.mu.Lock()
	entry, exists := b.albums[key]
	if !exists {
		entry = &album{chatID: chatID, sources: make(map[string]struct{})}
		b.albums[key] = entry
		go func() {
			timer := time.NewTimer(1500 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			b.mu.Lock()
			current := b.albums[key]
			delete(b.albums, key)
			b.mu.Unlock()
			if current == nil {
				return
			}
			urls := make([]string, 0, len(current.sources))
			for url := range current.sources {
				urls = append(urls, url)
			}
			b.createForwarded(ctx, current.chatID, urls)
		}()
	}
	entry.sources[source] = struct{}{}
	b.mu.Unlock()
}

func (b *Client) createForwarded(ctx context.Context, chatID int64, sources []string) {
	created := make([]string, 0, len(sources))
	for _, source := range sources {
		result, err := b.downloads.Create(ctx, req.CreateDownloadReq{URL: source})
		if err != nil {
			b.log.Warn("bot create forwarded-media download failed", zap.String("url", source), zap.Error(err))
			continue
		}
		created = append(created, result.JobID)
	}
	if len(created) == 0 {
		_, _ = b.send(ctx, chatID, "未能创建媒体下载任务；请确认已登录的 Telegram 账号可以访问原频道。")
		return
	}
	b.log.Info("bot forwarded media tasks created", zap.Int64("chat_id", chatID), zap.Int("count", len(created)))
	if len(created) == 1 {
		statusID, err := b.send(ctx, chatID, "已创建转发媒体下载任务，正在下载…")
		if err == nil {
			b.watch(created[0], chatID, statusID)
		}
		return
	}
	statusID, err := b.send(ctx, chatID, fmt.Sprintf("已识别合集，已创建 %d 个下载任务，正在下载…", len(created)))
	if err != nil {
		b.log.Warn("bot send album status failed", zap.Error(err))
		return
	}
	b.watchAlbum(created, chatID, statusID)
}

// watchAlbum combines progress events from album items into one Bot message so
// a forwarded album does not produce one progress message per photo or video.
func (b *Client) watchAlbum(jobIDs []string, chatID int64, messageID int) {
	state := &albumProgress{total: len(jobIDs), events: make(map[string]logic.DownloadEvent, len(jobIDs))}
	for _, jobID := range jobIDs {
		id := jobID
		b.downloads.Watch(id, func(event logic.DownloadEvent) {
			state.mu.Lock()
			state.events[id] = event
			text, final := state.format()
			state.mu.Unlock()
			key := fmt.Sprintf("%d:%d", chatID, messageID)
			if !final && !b.shouldEdit(key) {
				return
			}
			editCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := b.edit(editCtx, chatID, messageID, text); err != nil {
				b.log.Debug("bot edit album status failed", zap.Error(err))
			}
		})
	}
}

type albumProgress struct {
	mu     sync.Mutex
	total  int
	events map[string]logic.DownloadEvent
}

func (p *albumProgress) format() (string, bool) {
	completed, failed := 0, 0
	var downloaded, totalBytes int64
	for _, event := range p.events {
		switch event.Status {
		case "success":
			completed++
		case "failed", "cancelled":
			completed++
			failed++
		}
		downloaded += event.Downloaded
		totalBytes += event.Total
	}
	if completed == p.total {
		if failed > 0 {
			return fmt.Sprintf("合集下载完成：%d/%d 成功，%d 个失败或取消。", p.total-failed, p.total, failed), true
		}
		return fmt.Sprintf("合集下载完成：共 %d 个媒体。", p.total), true
	}
	if totalBytes > 0 {
		return fmt.Sprintf("合集下载中：已完成 %d/%d\n%d%%（%s / %s）", completed, p.total, downloaded*100/totalBytes, size(downloaded), size(totalBytes)), false
	}
	return fmt.Sprintf("合集下载中：已完成 %d/%d", completed, p.total), false
}

func (b *Client) watch(jobID string, chatID int64, messageID int) {
	b.downloads.Watch(jobID, func(event logic.DownloadEvent) {
		text, final := formatProgress(event)
		key := fmt.Sprintf("%d:%d", chatID, messageID)
		if !final && !b.shouldEdit(key) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := b.edit(ctx, chatID, messageID, text); err != nil {
			b.log.Debug("bot edit status failed", zap.Error(err))
		}
	})
}

func (b *Client) shouldEdit(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Since(b.lastEdit[key]) < 3*time.Second {
		return false
	}
	b.lastEdit[key] = time.Now()
	return true
}

func formatProgress(event logic.DownloadEvent) (string, bool) {
	switch event.Status {
	case "success":
		return "下载完成：" + event.FileName, true
	case "failed":
		return "下载失败：" + event.Error, true
	case "cancelled":
		return "下载已取消", true
	case "downloading":
		if event.Total == 0 {
			return "正在下载：" + event.FileName, false
		}
		return fmt.Sprintf("正在下载：%s\n%d%%（%s / %s）", event.FileName, event.Downloaded*100/event.Total, size(event.Downloaded), size(event.Total)), false
	default:
		return "任务状态更新中…", false
	}
}

func size(value int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	amount := float64(value)
	index := 0
	for amount >= 1024 && index < len(units)-1 {
		amount /= 1024
		index++
	}
	return fmt.Sprintf("%.1f %s", amount, units[index])
}

func (b *Client) send(ctx context.Context, chatID int64, text string) (int, error) {
	var message sentMessage
	err := b.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text}, &message)
	return message.MessageID, err
}

func (b *Client) edit(ctx context.Context, chatID int64, messageID int, text string) error {
	return b.call(ctx, "editMessageText", map[string]any{"chat_id": chatID, "message_id": messageID, "text": text}, nil)
}

func (b *Client) call(ctx context.Context, method string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := b.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var envelope apiResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return err
	}
	if !envelope.OK {
		return fmt.Errorf("bot api: %s", envelope.Description)
	}
	if target != nil {
		return json.Unmarshal(envelope.Result, target)
	}
	return nil
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
}
type update struct {
	UpdateID int64            `json:"update_id"`
	Message  *incomingMessage `json:"message"`
}
type incomingMessage struct {
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Text                 string          `json:"text"`
	Caption              string          `json:"caption"`
	MediaGroupID         string          `json:"media_group_id"`
	Photo                json.RawMessage `json:"photo"`
	Video                json.RawMessage `json:"video"`
	Document             json.RawMessage `json:"document"`
	ForwardOrigin        *forwardOrigin  `json:"forward_origin"`
	ForwardFromChat      *originChat     `json:"forward_from_chat"`
	ForwardFromMessageID int             `json:"forward_from_message_id"`
}

type forwardOrigin struct {
	Type      string     `json:"type"`
	Chat      originChat `json:"chat"`
	MessageID int        `json:"message_id"`
}
type originChat struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func (m *incomingMessage) hasMedia() bool {
	return len(m.Photo) > 0 || len(m.Video) > 0 || len(m.Document) > 0
}

func (m *incomingMessage) sourceURL() (string, bool) {
	chat, messageID := originChat{}, 0
	if m.ForwardOrigin != nil && m.ForwardOrigin.Type == "channel" {
		chat, messageID = m.ForwardOrigin.Chat, m.ForwardOrigin.MessageID
	} else if m.ForwardFromChat != nil && m.ForwardFromMessageID > 0 {
		chat, messageID = *m.ForwardFromChat, m.ForwardFromMessageID
	}
	if chat.ID == 0 || messageID <= 0 {
		return "", false
	}
	if chat.Username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", chat.Username, messageID), true
	}
	// Bot API uses -100<channel id>, while a t.me/c link uses the positive
	// MTProto channel id. The TDL session supplies the required access hash.
	channelID := chat.ID
	if channelID < -1000000000000 {
		channelID = -(channelID + 1000000000000)
	}
	if channelID <= 0 {
		return "", false
	}
	return fmt.Sprintf("https://t.me/c/%d/%d", channelID, messageID), true
}

type sentMessage struct {
	MessageID int `json:"message_id"`
}

type botUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}
