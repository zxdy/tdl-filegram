package req

// CreateDownloadReq 创建下载任务请求
type CreateDownloadReq struct {
	URL      string `json:"url" binding:"required"`
	Filename string `json:"filename"` // 可选，用户指定的文件名
}

// PreviewDownloadReq 预览下载请求（解析链接获取文件名和大小）
type PreviewDownloadReq struct {
	URL string `json:"url" binding:"required"`
}

// BatchCreateDownloadReq 从聊天消息中批量创建下载任务。
type BatchCreateDownloadReq struct {
	URLs []string `json:"urls" binding:"required,min=1,max=100"`
}

// Submit2FAReq 提交两步验证密码
type Submit2FAReq struct {
	Password string `json:"password" binding:"required"`
}

// PaginationReq 分页请求
type PaginationReq struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// BatchDeleteReq 批量删除任务请求
type BatchDeleteReq struct {
	IDs        []string `json:"ids" binding:"required,min=1"`
	DeleteFile bool     `json:"delete_file"`
}
