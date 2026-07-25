package logic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"tdl-filegram/enum"
	"tdl-filegram/internal/dto/req"
	"tdl-filegram/internal/dto/res"
	"tdl-filegram/internal/model"
	"tdl-filegram/pkg/telegram"
)

// DownloadLogic 下载业务编排：创建任务、后台执行、进度跟踪
type DownloadLogic struct {
	jobModel    *model.JobModel
	engine      *telegram.Engine
	downloadDir string
	log         *zap.Logger

	mu          sync.RWMutex
	progress    map[string]*liveProgress
	cancels     map[string]context.CancelFunc
	cancelFlags map[string]bool          // true=取消（不可恢复），false/缺省=暂停
	dones       map[string]chan struct{} // process goroutine 退出信号
	watchers    map[string][]func(DownloadEvent)
}

// DownloadEvent 是下载状态变化的轻量通知，用于 Bot 等外部入口反馈进度。
type DownloadEvent struct {
	JobID      string
	Status     string
	FileName   string
	Downloaded int64
	Total      int64
	Error      string
}

// liveProgress 下载过程中的实时进度（内存）
type liveProgress struct {
	fileName   string
	mime       string
	total      int64
	downloaded int64
	startedAt  time.Time
}

func NewDownloadLogic(jobModel *model.JobModel, engine *telegram.Engine, downloadDir string, log *zap.Logger) *DownloadLogic {
	return &DownloadLogic{
		jobModel:    jobModel,
		engine:      engine,
		downloadDir: downloadDir,
		log:         log,
		progress:    make(map[string]*liveProgress),
		cancels:     make(map[string]context.CancelFunc),
		cancelFlags: make(map[string]bool),
		dones:       make(map[string]chan struct{}),
		watchers:    make(map[string][]func(DownloadEvent)),
	}
}

// Watch 订阅单个任务的进度；返回函数可取消订阅。
func (l *DownloadLogic) Watch(jobID string, fn func(DownloadEvent)) func() {
	l.mu.Lock()
	l.watchers[jobID] = append(l.watchers[jobID], fn)
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		listeners := l.watchers[jobID]
		for i, listener := range listeners {
			if fmt.Sprintf("%p", listener) == fmt.Sprintf("%p", fn) {
				l.watchers[jobID] = append(listeners[:i], listeners[i+1:]...)
				break
			}
		}
	}
}

// Create 创建下载任务并异步执行
func (l *DownloadLogic) Create(ctx context.Context, r req.CreateDownloadReq) (*res.CreateDownloadRes, error) {
	if !l.engine.IsReady() {
		return nil, enum.ErrTelegramNotReady
	}
	// 确定文件名：用户指定则使用用户输入，否则解析媒体获取默认名
	filename := ""
	if r.Filename != "" {
		filename = telegram.SanitizeName(r.Filename)
	}
	if filename == "" {
		l.log.Debug("resolving download media", zap.String("url", r.URL))
		info, err := l.engine.ResolveMedia(ctx, r.URL)
		if err != nil {
			return nil, err
		}
		filename = info.Name
	}
	// 检查同名文件是否已存在
	targetPath := filepath.Join(l.downloadDir, filename)
	if _, err := os.Stat(targetPath); err == nil {
		return nil, enum.ErrFileExists
	}
	job := &model.Job{
		ID:       uuid.NewString(),
		URL:      r.URL,
		Status:   enum.JobStatusPending,
		FileName: filename,
	}
	if err := l.jobModel.Create(ctx, job); err != nil {
		return nil, err
	}
	go l.process(job)
	return &res.CreateDownloadRes{JobID: job.ID}, nil
}

// CreateBatch 从公开聊天消息链接批量创建下载任务。
func (l *DownloadLogic) CreateBatch(ctx context.Context, urls []string) ([]*res.CreateDownloadRes, error) {
	result := make([]*res.CreateDownloadRes, 0, len(urls))
	for _, url := range urls {
		item, err := l.Create(ctx, req.CreateDownloadReq{URL: url})
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

// Preview 解析消息链接，返回媒体文件名和大小（用于下载前预览）
func (l *DownloadLogic) Preview(ctx context.Context, r req.PreviewDownloadReq) (*res.PreviewDownloadRes, error) {
	if !l.engine.IsReady() {
		return nil, enum.ErrTelegramNotReady
	}
	info, err := l.engine.ResolveMedia(ctx, r.URL)
	if err != nil {
		return nil, err
	}
	return &res.PreviewDownloadRes{
		Filename: info.Name,
		Size:     info.Size,
		MIME:     info.MIME,
	}, nil
}

func (l *DownloadLogic) process(job *model.Job) {
	// 注册 done channel，供 Delete 等待 process 退出
	done := make(chan struct{})
	l.mu.Lock()
	l.dones[job.ID] = done
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.dones, job.ID)
		l.mu.Unlock()
		close(done)
	}()

	runCtx := l.engine.RunCtx()
	if runCtx == nil {
		l.failJob(job, "telegram engine not ready")
		return
	}

	// 检查任务是否已被删除（pending 期间被删除的情况）
	if _, err := l.jobModel.GetByID(context.Background(), job.ID); err != nil {
		return
	}

	job.Status = enum.JobStatusDownloading
	_ = l.jobModel.Save(context.Background(), job)
	l.log.Info("download started", zap.String("job_id", job.ID), zap.String("url", job.URL), zap.String("file", job.FileName))
	l.emit(DownloadEvent{JobID: job.ID, Status: job.Status, FileName: job.FileName})

	dlCtx, cancel := context.WithCancel(runCtx)
	l.mu.Lock()
	l.cancels[job.ID] = cancel
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.cancels, job.ID)
		l.mu.Unlock()
	}()

	reporter := &jobReporter{logic: l, jobID: job.ID}
	err := l.engine.DownloadMedia(dlCtx, job.URL, l.downloadDir, job.FileName, reporter)
	if err != nil {
		if errors.Is(dlCtx.Err(), context.Canceled) {
			// 检查是取消还是暂停
			l.mu.Lock()
			isCancel := l.cancelFlags[job.ID]
			delete(l.cancelFlags, job.ID)
			l.mu.Unlock()

			if isCancel {
				// 取消：标记为已取消，清理进度
				job.Status = enum.JobStatusCancelled
				_ = l.jobModel.Save(context.Background(), job)
				l.removeProgress(job.ID)
				l.log.Info("download cancelled", zap.String("job_id", job.ID))
				return
			}
			// 暂停：保留部分文件与已下载字节
			job.Status = enum.JobStatusPaused
			if lp := l.getProgress(job.ID); lp != nil {
				job.FileName = lp.fileName
				job.FileSize = lp.total
				job.DownloadedBytes = lp.downloaded
				job.MIME = lp.mime
				job.FilePath = fmt.Sprintf("%s/%s", l.downloadDir, lp.fileName)
			}
			_ = l.jobModel.Save(context.Background(), job)
			l.log.Info("download paused", zap.String("job_id", job.ID))
			return
		}
		l.failJob(job, err.Error())
		return
	}
	// 完成后回写文件信息
	job.Status = enum.JobStatusSuccess
	if lp := l.getProgress(job.ID); lp != nil {
		job.FileName = lp.fileName
		job.FileSize = lp.total
		job.DownloadedBytes = lp.total
		job.MIME = lp.mime
		job.FilePath = fmt.Sprintf("%s/%s", l.downloadDir, lp.fileName)
	}
	_ = l.jobModel.Save(context.Background(), job)
	l.removeProgress(job.ID)
	l.emit(DownloadEvent{JobID: job.ID, Status: job.Status, FileName: job.FileName, Downloaded: job.DownloadedBytes, Total: job.FileSize})
	l.clearWatchers(job.ID)
	l.log.Info("download success", zap.String("job_id", job.ID), zap.String("file", job.FileName))
}

// Pause 暂停指定任务的下载
func (l *DownloadLogic) Pause(jobID string) error {
	l.mu.Lock()
	cancel, ok := l.cancels[jobID]
	if ok {
		l.cancelFlags[jobID] = false
	}
	l.mu.Unlock()
	if !ok {
		return errors.New("task is not downloading")
	}
	// 立即更新状态为已暂停，并从内存进度同步已下载字节数，避免前端轮询看到进度归零
	if job, err := l.jobModel.GetByID(context.Background(), jobID); err == nil {
		job.Status = enum.JobStatusPaused
		if lp := l.getProgress(jobID); lp != nil {
			job.FileName = lp.fileName
			job.FileSize = lp.total
			job.DownloadedBytes = lp.downloaded
			job.MIME = lp.mime
			job.FilePath = fmt.Sprintf("%s/%s", l.downloadDir, lp.fileName)
		}
		_ = l.jobModel.Save(context.Background(), job)
	}
	cancel()
	return nil
}

// Cancel 取消指定任务的下载（不可恢复）
func (l *DownloadLogic) Cancel(jobID string) error {
	l.mu.Lock()
	cancel, ok := l.cancels[jobID]
	if ok {
		l.cancelFlags[jobID] = true
	}
	l.mu.Unlock()
	if !ok {
		return errors.New("task is not downloading")
	}
	// 立即更新状态为已取消
	if job, err := l.jobModel.GetByID(context.Background(), jobID); err == nil {
		job.Status = enum.JobStatusCancelled
		_ = l.jobModel.Save(context.Background(), job)
	}
	cancel()
	return nil
}

// Wait 等待指定任务的 process goroutine 退出
func (l *DownloadLogic) Wait(jobID string) {
	l.mu.RLock()
	done, ok := l.dones[jobID]
	l.mu.RUnlock()
	if ok {
		<-done
	}
}

// Retry 重试/继续下载指定任务（断点续传）
func (l *DownloadLogic) Retry(jobID string) error {
	job, err := l.jobModel.GetByID(context.Background(), jobID)
	if err != nil {
		return err
	}
	if job.Status == enum.JobStatusDownloading {
		return errors.New("task is already downloading")
	}
	// 等待可能存在的旧 process goroutine 退出（暂停后立即点继续的场景）
	l.Wait(jobID)
	job.Status = enum.JobStatusDownloading
	job.Error = ""
	_ = l.jobModel.Save(context.Background(), job)
	go l.process(job)
	return nil
}

func (l *DownloadLogic) failJob(job *model.Job, errMsg string) {
	job.Status = enum.JobStatusFailed
	job.Error = errMsg
	_ = l.jobModel.Save(context.Background(), job)
	l.removeProgress(job.ID)
	l.emit(DownloadEvent{JobID: job.ID, Status: job.Status, FileName: job.FileName, Error: errMsg})
	l.clearWatchers(job.ID)
	l.log.Error("download failed", zap.String("job_id", job.ID), zap.String("error", errMsg))
}

// --- 进度注册表 ---

func (l *DownloadLogic) setProgress(id string, p *liveProgress) {
	l.mu.Lock()
	l.progress[id] = p
	l.mu.Unlock()
}

func (l *DownloadLogic) getProgress(id string) *liveProgress {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.progress[id]
}

func (l *DownloadLogic) updateProgress(id string, downloaded int64) {
	l.mu.Lock()
	var event DownloadEvent
	if p, ok := l.progress[id]; ok {
		p.downloaded = downloaded
		event = DownloadEvent{JobID: id, Status: enum.JobStatusDownloading, FileName: p.fileName, Downloaded: downloaded, Total: p.total}
	}
	l.mu.Unlock()
	if event.Total > 0 {
		l.emit(event)
	}
}

func (l *DownloadLogic) emit(event DownloadEvent) {
	l.mu.RLock()
	listeners := append([]func(DownloadEvent){}, l.watchers[event.JobID]...)
	l.mu.RUnlock()
	for _, listener := range listeners {
		listener(event)
	}
}

func (l *DownloadLogic) clearWatchers(jobID string) {
	l.mu.Lock()
	delete(l.watchers, jobID)
	l.mu.Unlock()
}

func (l *DownloadLogic) removeProgress(id string) {
	l.mu.Lock()
	delete(l.progress, id)
	l.mu.Unlock()
}

// FilePath 返回指定任务的本地文件路径（从内存进度中获取，用于删除时文件清理）。
// 下载中的任务 FilePath 尚未回写 DB，需从内存 liveProgress 获取。
func (l *DownloadLogic) FilePath(jobID string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if lp, ok := l.progress[jobID]; ok && lp.fileName != "" {
		return fmt.Sprintf("%s/%s", l.downloadDir, lp.fileName)
	}
	return ""
}

// FilePathByName 根据文件名构建完整路径（用于已取消/失败任务的文件清理，
// 这些任务的 FilePath 未回写 DB、liveProgress 已清除，但 FileName 在创建时已存储）
func (l *DownloadLogic) FilePathByName(filename string) string {
	if filename == "" {
		return ""
	}
	return filepath.Join(l.downloadDir, filename)
}

// jobReporter 实现 telegram.ProgressReporter
type jobReporter struct {
	logic *DownloadLogic
	jobID string
}

func (r *jobReporter) OnStart(name string, size int64, mime string) {
	r.logic.setProgress(r.jobID, &liveProgress{fileName: name, mime: mime, total: size, startedAt: time.Now()})
}
func (r *jobReporter) OnProgress(downloaded, total int64) {
	r.logic.updateProgress(r.jobID, downloaded)
}
func (r *jobReporter) OnDone(string, error) {}
