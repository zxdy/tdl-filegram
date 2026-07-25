package router

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"tdl-filegram/internal/controller"
	"tdl-filegram/internal/middleware"
	"tdl-filegram/web"
)

// Register 注册所有路由与中间件
func Register(r *gin.Engine, log *zap.Logger,
	loginCtl *controller.LoginController,
	downloadCtl *controller.DownloadController,
	jobCtl *controller.JobController,
	chatCtl *controller.ChatController,
) {
	r.Use(middleware.Recovery(log), middleware.Trace(), middleware.Cors())

	// 健康检查（不经过鉴权）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	{
		// 登录
		api.GET("/login/status", loginCtl.Status)
		api.POST("/login/qr/start", loginCtl.StartQR)
		api.POST("/login/qr/2fa", loginCtl.Submit2FA)

		// 聊天列表
		api.GET("/chats", chatCtl.List)
		api.GET("/chats/:type/:id/messages", chatCtl.Messages)
		api.GET("/chats/:type/:id/messages/:messageID/thumbnail", chatCtl.Thumbnail)

		// 下载
		api.POST("/download", downloadCtl.Create)
		api.POST("/download/batch", downloadCtl.BatchCreate)
		api.POST("/download/preview", downloadCtl.Preview)

		// 任务
		api.GET("/jobs", jobCtl.List)
		api.GET("/jobs/:id", jobCtl.Get)
		api.GET("/jobs/:id/file", jobCtl.DownloadFile)
		api.POST("/jobs/:id/pause", jobCtl.Pause)
		api.POST("/jobs/:id/retry", jobCtl.Retry)
		api.POST("/jobs/:id/cancel", jobCtl.Cancel)
		api.DELETE("/jobs/:id", jobCtl.Delete)
		api.DELETE("/jobs", jobCtl.BatchDelete)
	}

	// 前端静态资源（构建产物嵌入）
	distFS, _ := fs.Sub(web.FS, "dist")
	r.GET("/", func(c *gin.Context) {
		b, _ := fs.ReadFile(distFS, "index.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", b)
	})
	r.GET("/assets/*filepath", func(c *gin.Context) {
		http.FileServer(http.FS(distFS)).ServeHTTP(c.Writer, c.Request)
	})
	r.NoRoute(func(c *gin.Context) {
		// API 路由 404 返回 JSON
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"code": 10040401, "msg": "not found"})
			return
		}
		// 其他路由回退到首页（SPA history 模式）
		b, _ := fs.ReadFile(distFS, "index.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", b)
	})
}
