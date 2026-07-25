package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"tdl-filegram/enum"
	"tdl-filegram/internal/dto/req"
	"tdl-filegram/internal/logic"
	"tdl-filegram/utils/response"
)

type DownloadController struct {
	downloadLogic *logic.DownloadLogic
}

func NewDownloadController(d *logic.DownloadLogic) *DownloadController {
	return &DownloadController{downloadLogic: d}
}

// Create 创建下载任务
func (ctl *DownloadController) Create(c *gin.Context) {
	var r req.CreateDownloadReq
	if err := c.ShouldBindJSON(&r); err != nil {
		response.Fail(c, enum.ErrInvalidParam.Code, enum.ErrInvalidParam.Msg, http.StatusBadRequest)
		return
	}
	res, err := ctl.downloadLogic.Create(c.Request.Context(), r)
	if err != nil {
		handleErr(c, err)
		return
	}
	response.Success(c, res)
}

// Preview 解析消息链接，返回媒体文件名和大小（用于下载前预览）
func (ctl *DownloadController) Preview(c *gin.Context) {
	var r req.PreviewDownloadReq
	if err := c.ShouldBindJSON(&r); err != nil {
		response.Fail(c, enum.ErrInvalidParam.Code, enum.ErrInvalidParam.Msg, http.StatusBadRequest)
		return
	}
	res, err := ctl.downloadLogic.Preview(c.Request.Context(), r)
	if err != nil {
		handleErr(c, err)
		return
	}
	response.Success(c, res)
}

// BatchCreate 批量创建公开聊天消息的下载任务。
func (ctl *DownloadController) BatchCreate(c *gin.Context) {
	var r req.BatchCreateDownloadReq
	if err := c.ShouldBindJSON(&r); err != nil {
		response.Fail(c, enum.ErrInvalidParam.Code, enum.ErrInvalidParam.Msg, http.StatusBadRequest)
		return
	}
	res, err := ctl.downloadLogic.CreateBatch(c.Request.Context(), r.URLs)
	if err != nil {
		handleErr(c, err)
		return
	}
	response.Success(c, res)
}
