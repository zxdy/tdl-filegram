package res

import "time"

// LoginStatusRes 登录状态响应
type LoginStatusRes struct {
	Ready         bool   `json:"ready"`
	Authenticated bool   `json:"authenticated"`
	LoginStatus   string `json:"login_status"`
	QRURL         string `json:"qr_url,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ChatRes 聊天列表项，不包含消息内容。
type ChatRes struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Username    string `json:"username,omitempty"`
	UnreadCount int    `json:"unread_count"`
}

type ChatMessageRes struct {
	ID         int       `json:"id"`
	Title      string    `json:"title"`
	Date       time.Time `json:"date"`
	MediaType  string    `json:"media_type,omitempty"`
	Size       int64     `json:"size"`
	MIME       string    `json:"mime,omitempty"`
	SourceURL  string    `json:"source_url,omitempty"`
	HasMedia   bool      `json:"has_media"`
	HasPreview bool      `json:"has_preview"`
}

// CreateDownloadRes 创建下载任务响应
type CreateDownloadRes struct {
	JobID string `json:"job_id"`
}

// PreviewDownloadRes 预览下载响应（解析链接后的文件名和大小）
type PreviewDownloadRes struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MIME     string `json:"mime"`
}

// JobRes 任务响应
type JobRes struct {
	ID              string    `json:"id"`
	URL             string    `json:"url"`
	Status          string    `json:"status"`
	FileName        string    `json:"file_name"`
	FileSize        int64     `json:"file_size"`
	DownloadedBytes int64     `json:"downloaded_bytes"`
	MIME            string    `json:"mime"`
	Speed           int64     `json:"speed"`
	EtaSeconds      int64     `json:"eta_seconds"`
	FilePath        string    `json:"file_path,omitempty"`
	Progress        int       `json:"progress"`
	Error           string    `json:"error,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// JobListRes 任务列表响应
type JobListRes struct {
	List     []*JobRes `json:"list"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
