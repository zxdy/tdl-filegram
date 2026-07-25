package telegram

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-faster/errors"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/iyear/tdl/core/tmedia"
	"github.com/iyear/tdl/core/util/tutil"
)

const partSize = 1024 * 1024 // 1MB，与 tdl MaxPartSize 一致

const maxRetries = 10 // downloadChunk 最大重试次数

// ErrFileReferenceExpired 表示 file_reference 已过期，需要重新解析媒体
var ErrFileReferenceExpired = errors.New("file reference expired")

// ProgressReporter 由 logic 层实现，用于接收下载进度回调
type ProgressReporter interface {
	OnStart(name string, size int64, mime string)
	OnProgress(downloaded, total int64)
	OnDone(filePath string, err error)
}

// resumeState 断点续传状态（bitmap），记录哪些 1MB 分片已完成
type resumeState struct {
	mu     sync.Mutex
	bitmap []byte
	parts  int
}

func newResumeState(parts int) *resumeState {
	return &resumeState{
		bitmap: make([]byte, (parts+7)/8),
		parts:  parts,
	}
}

func (r *resumeState) isDone(idx int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isDoneLocked(idx)
}

// isDoneLocked 在已持有锁的情况下读取 bitmap，调用方需自行加锁
func (r *resumeState) isDoneLocked(idx int) bool {
	return r.bitmap[idx/8]&(1<<(uint(idx)%8)) != 0
}

func (r *resumeState) markDone(idx int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bitmap[idx/8] |= 1 << (uint(idx) % 8)
}

func (r *resumeState) count() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := 0
	for i := 0; i < r.parts; i++ {
		if r.isDoneLocked(i) {
			c++
		}
	}
	return int64(c)
}

// countBytes 按分片实际大小累加计算已下载字节数
func (r *resumeState) countBytes(totalSize int64) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total int64
	for i := 0; i < r.parts; i++ {
		if !r.isDoneLocked(i) {
			continue
		}
		if i == r.parts-1 {
			total += totalSize - int64(r.parts-1)*partSize
		} else {
			total += partSize
		}
	}
	return total
}

// save 原子写入 bitmap 到 path（先写临时文件再 rename，避免半写入状态）
func (r *resumeState) save(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, r.bitmap, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadResume(path string, parts int) *resumeState {
	r := newResumeState(parts)
	if data, err := os.ReadFile(path); err == nil && len(data) == len(r.bitmap) {
		r.bitmap = data
	}
	return r
}

// resolveMedia 解析 t.me 链接对应的可下载媒体，用于 file_reference 过期后的刷新
func (e *Engine) resolveMedia(ctx context.Context, url string) (*tmedia.Media, error) {
	peer, msgID, err := e.parseMessageLink(ctx, url)
	if err != nil {
		return nil, errors.Wrap(err, "parse message link")
	}
	msg, err := tutil.GetSingleMessage(ctx, e.pool.Default(ctx), peer, msgID)
	if err != nil {
		return nil, errors.Wrap(err, "get message")
	}
	media, ok := tmedia.GetMedia(msg)
	if !ok {
		return nil, errors.New("message has no downloadable media")
	}
	return media, nil
}

// MediaInfo 媒体文件信息（用于下载前预览）
type MediaInfo struct {
	Name string
	Size int64
	MIME string
}

// ResolveMedia 解析消息链接对应的媒体信息（用于下载前预览文件名和大小）
func (e *Engine) ResolveMedia(ctx context.Context, url string) (*MediaInfo, error) {
	peer, msgID, err := e.parseMessageLink(ctx, url)
	if err != nil {
		return nil, errors.Wrap(err, "parse message link")
	}
	msg, err := tutil.GetSingleMessage(ctx, e.pool.Default(ctx), peer, msgID)
	if err != nil {
		return nil, errors.Wrap(err, "get message")
	}
	media, ok := tmedia.GetMedia(msg)
	if !ok {
		return nil, errors.New("message has no downloadable media")
	}
	return &MediaInfo{
		Name: SanitizeName(media.Name),
		Size: media.Size,
		MIME: extractMIME(msg),
	}, nil
}

// DownloadMedia 解析 t.me 链接，下载其中的媒体文件到 dir，通过 reporter 上报进度。
// filename 非空时使用用户指定的文件名，否则使用媒体原始文件名。
// 支持断点续传：已下载的分片不会重复下载，通过 .resume 元数据文件跟踪进度。
func (e *Engine) DownloadMedia(ctx context.Context, url, dir, filename string, rep ProgressReporter) error {
	// 初始解析媒体（保留 msg 用于 mime 与兜底文件名）
	peer, msgID, err := e.parseMessageLink(ctx, url)
	if err != nil {
		return errors.Wrap(err, "parse message link")
	}
	msg, err := tutil.GetSingleMessage(ctx, e.pool.Default(ctx), peer, msgID)
	if err != nil {
		return errors.Wrap(err, "get message")
	}
	media, ok := tmedia.GetMedia(msg)
	if !ok {
		return errors.New("message has no downloadable media")
	}

	mime := extractMIME(msg)
	name := SanitizeName(media.Name)
	if filename != "" {
		name = SanitizeName(filename)
	}
	if name == "" {
		name = fmt.Sprintf("%d_%d", tutil.GetInputPeerID(peer), msgID)
	}
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.Wrap(err, "mkdir")
	}
	// 不截断文件，保留已下载内容用于断点续传
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return errors.Wrap(err, "create file")
	}
	defer f.Close()

	// 加载断点续传状态
	metaPath := path + ".resume"
	totalParts := int((media.Size + partSize - 1) / partSize)
	resume := loadResume(metaPath, totalParts)

	rep.OnStart(name, media.Size, mime)

	// 报告已下载部分的初始进度（按分片实际大小累加）
	initialDownloaded := resume.countBytes(media.Size)
	if initialDownloaded > 0 {
		rep.OnProgress(initialDownloaded, media.Size)
	}

	// 如果所有分片已完成，直接结束
	if initialDownloaded >= media.Size {
		_ = os.Remove(metaPath)
		rep.OnDone(path, nil)
		return nil
	}

	// 并行下载缺失的分片
	threads := tutil.BestThreads(media.Size, e.cfg.Threads)
	var downloaded atomic.Int64
	downloaded.Store(initialDownloaded)

	// 进入并行下载循环前一次性获取 client，file_reference 刷新时按需更新
	client := e.pool.Client(ctx, media.DC)

	for {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(threads)

		for i := 0; i < totalParts; i++ {
			// 上下文取消后立即停止调度新分片
			if gctx.Err() != nil {
				break
			}
			if resume.isDone(i) {
				continue
			}

			partIdx := i
			offset := int64(partIdx) * partSize

			g.Go(func() error {
				// 始终使用 partSize 作为 limit：Telegram API 要求 limit 必须是 4096 的倍数，
				// 服务端会自动只返回剩余字节，无需对末尾分片做特殊处理（否则会触发 LIMIT_INVALID）
				data, err := downloadChunk(gctx, client, media.InputFileLoc, offset, partSize)
				if err != nil {
					return err
				}

				if _, err := f.WriteAt(data, offset); err != nil {
					return errors.Wrap(err, "write file")
				}
				// 确保数据落盘后再标记分片完成，避免断电等场景下元数据先于数据持久化
				if err := f.Sync(); err != nil {
					return errors.Wrap(err, "sync file")
				}

				resume.markDone(partIdx)
				if err := resume.save(metaPath); err != nil {
					e.log.Warn("save resume state failed", zap.Error(err))
				}

				rep.OnProgress(downloaded.Add(int64(len(data))), media.Size)
				return nil
			})
		}

		err := g.Wait()
		if err == nil {
			break
		}
		// file_reference 过期：刷新 media 后重试未完成的分片
		if errors.Is(err, ErrFileReferenceExpired) {
			newMedia, refErr := e.resolveMedia(ctx, url)
			if refErr != nil {
				rep.OnDone(path, refErr)
				return refErr
			}
			media.InputFileLoc = newMedia.InputFileLoc
			// DC 可能变化，需要重新获取 client
			if media.DC != newMedia.DC {
				media.DC = newMedia.DC
				client = e.pool.Client(ctx, media.DC)
			}
			continue
		}
		rep.OnDone(path, err)
		return err
	}

	// 最终落盘
	if err := f.Sync(); err != nil {
		rep.OnDone(path, err)
		return errors.Wrap(err, "final sync")
	}
	// 校验文件大小
	stat, err := f.Stat()
	if err != nil {
		rep.OnDone(path, err)
		return errors.Wrap(err, "stat file")
	}
	if stat.Size() != media.Size {
		err := errors.Errorf("file size mismatch: got %d, want %d", stat.Size(), media.Size)
		rep.OnDone(path, err)
		return err
	}

	// 下载完成，清理元数据文件
	_ = os.Remove(metaPath)
	rep.OnDone(path, nil)
	return nil
}

// parseMessageLink 解析公开链接，以及 Bot 转发时构造的私有频道 /c/ 链接。
// 后者没有用户名，TDL 会从当前登录账号的对话缓存中取得频道 access hash。
func (e *Engine) parseMessageLink(ctx context.Context, rawURL string) (tg.InputPeerClass, int, error) {
	peer, msgID, err := tutil.ParseMessageLink(ctx, e.manager, rawURL)
	if err == nil {
		return peer.InputPeer(), msgID, nil
	}

	u, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		return nil, 0, err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "c" {
		return nil, 0, err
	}
	channelID, idErr := strconv.ParseInt(parts[1], 10, 64)
	if idErr != nil {
		return nil, 0, err
	}
	messageID, idErr := strconv.Atoi(parts[2])
	if idErr != nil {
		return nil, 0, err
	}
	input, _, resolveErr := e.inputPeer(ctx, "频道", channelID)
	if resolveErr != nil {
		return nil, 0, resolveErr
	}
	return input, messageID, nil
}

// downloadChunk 下载指定偏移量的文件块，自动处理 FloodWait 和超时重试
func downloadChunk(ctx context.Context, client *tg.Client, loc tg.InputFileLocationClass, offset int64, limit int) ([]byte, error) {
	req := &tg.UploadGetFileRequest{
		Location: loc,
		Offset:   offset,
		Limit:    limit,
	}
	req.SetPrecise(true)

	var retries int
	for {
		r, err := client.UploadGetFile(ctx, req)
		if flood, err := tgerr.FloodWait(ctx, err); err != nil {
			if flood || tgerr.Is(err, tg.ErrTimeout) {
				retries++
				if retries > maxRetries {
					return nil, errors.Wrap(err, "max retries exceeded")
				}
				continue
			}
			if tgerr.Is(err, "FILE_REFERENCE_EXPIRED") {
				return nil, ErrFileReferenceExpired
			}
			return nil, errors.Wrap(err, "get file chunk")
		}

		switch result := r.(type) {
		case *tg.UploadFile:
			return result.Bytes, nil
		default:
			return nil, errors.Errorf("unexpected response type %T", r)
		}
	}
}

func extractMIME(msg *tg.Message) string {
	switch m := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		if doc, ok := m.Document.AsNotEmpty(); ok {
			return doc.MimeType
		}
	case *tg.MessageMediaPhoto:
		return "image/jpeg"
	}
	return "application/octet-stream"
}

func SanitizeName(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\x00", "")
	// 过滤控制字符 \x01-\x1f
	var b strings.Builder
	for _, r := range name {
		if r >= 0x01 && r <= 0x1f {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
