package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"tdl-filegram/internal/logic"
	"tdl-filegram/utils/response"
)

type ChatController struct {
	chatLogic *logic.ChatLogic
}

func (ctl *ChatController) Messages(c *gin.Context) {
	chatID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 10040001, "msg": "聊天 ID 无效"})
		return
	}
	maxID, _ := strconv.Atoi(c.Query("max_id"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	items, err := ctl.chatLogic.Messages(c.Request.Context(), c.Param("type"), chatID, maxID, limit)
	if err != nil {
		handleErr(c, err)
		return
	}
	response.Success(c, items)
}

func (ctl *ChatController) Thumbnail(c *gin.Context) {
	chatID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	messageID, err := strconv.Atoi(c.Param("messageID"))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	data, mime, err := ctl.chatLogic.Thumbnail(c.Request.Context(), c.Param("type"), chatID, messageID)
	if err != nil {
		c.Error(err)
		c.Status(http.StatusNotFound)
		return
	}
	c.Data(http.StatusOK, mime, data)
}

func NewChatController(l *logic.ChatLogic) *ChatController {
	return &ChatController{chatLogic: l}
}

// List 获取当前 Telegram 账号的聊天列表。
func (ctl *ChatController) List(c *gin.Context) {
	items, err := ctl.chatLogic.List(c.Request.Context())
	if err != nil {
		handleErr(c, err)
		return
	}
	response.Success(c, items)
}
