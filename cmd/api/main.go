package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"tdl-filegram/bootstrap"
	"tdl-filegram/internal/controller"
	"tdl-filegram/internal/logic"
	"tdl-filegram/internal/model"
	"tdl-filegram/internal/router"
	"tdl-filegram/pkg/bot"
	"tdl-filegram/pkg/telegram"
)

func main() {
	// 1. 加载配置
	cfg, err := bootstrap.LoadConfig("config/config.yaml")
	if err != nil {
		panic(err)
	}

	// 2. 初始化日志
	log, err := bootstrap.NewLogger(cfg.Log)
	if err != nil {
		panic(err)
	}

	// 3. 初始化数据库（SQLite）
	if _, err := bootstrap.NewDB(cfg.Database.Path, log); err != nil {
		log.Fatal("init db failed", zap.Error(err))
	}

	// 4. 初始化 telegram 引擎
	engine, err := telegram.NewEngine(telegram.Config{
		AppID:            cfg.Telegram.AppID,
		AppHash:          cfg.Telegram.AppHash,
		DataDir:          cfg.Telegram.DataDir,
		Namespace:        cfg.Telegram.Namespace,
		PoolSize:         cfg.Telegram.PoolSize,
		ReconnectTimeout: cfg.Telegram.ParseReconnectTimeout(),
		Proxy:            cfg.Telegram.Proxy,
		DownloadDir:      cfg.Download.Dir,
		Threads:          cfg.Download.Threads,
		Limit:            cfg.Download.Limit,
	}, log)
	if err != nil {
		log.Fatal("init telegram engine failed", zap.Error(err))
	}

	// 5. 组装业务层（logic → model / pkg）
	jobModel := model.NewJobModel()
	downloadLogic := logic.NewDownloadLogic(jobModel, engine, cfg.Download.Dir, log)
	jobLogic := logic.NewJobLogic(jobModel, downloadLogic)
	loginLogic := logic.NewLoginLogic(engine)
	chatLogic := logic.NewChatLogic(engine)
	var telegramBot *bot.Client
	botCfg := bot.Config{Token: cfg.Bot.Token, AllowedUserIDs: cfg.Bot.AllowedUserIDs, Proxy: cfg.Telegram.Proxy, AppID: cfg.Telegram.AppID, AppHash: cfg.Telegram.AppHash}
	if bot.Enabled(botCfg) {
		telegramBot = bot.New(botCfg, downloadLogic, log)
	}

	// 6. 组装控制器与路由
	loginCtl := controller.NewLoginController(loginLogic)
	downloadCtl := controller.NewDownloadController(downloadLogic)
	jobCtl := controller.NewJobController(jobLogic)
	chatCtl := controller.NewChatController(chatLogic)

	gin.SetMode(cfg.App.Env)
	r := gin.New()
	router.Register(r, log, loginCtl, downloadCtl, jobCtl, chatCtl)

	// 7. 信号监听
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Info("shutting down...")
		cancel()
	}()

	// 8. 后台启动 telegram 引擎（client.Run 连接 Telegram，就绪后设置 runCtx/pool/manager）
	// 与 HTTP 服务解耦：即使 Telegram 未连通，HTTP 也能响应，前端可提示「未就绪，检查代理」
	// Bot API 与 MTProto 引擎是独立连接，不能在每次引擎重连时重复启动。
	if telegramBot != nil {
		go telegramBot.Run(ctx)
		log.Info("telegram bot started")
	}
	go func() {
		const maxReconnectDelay = 30 * time.Second
		reconnectDelay := 2 * time.Second
		for ctx.Err() == nil {
			err := engine.Run(ctx, func(runCtx context.Context) error {
				// 连接成功过一次后，下一次异常断开从最短等待时间开始重试。
				reconnectDelay = 2 * time.Second
				// 保持 client.Run 运行直到连接或应用上下文结束。
				<-runCtx.Done()
				return runCtx.Err()
			})
			if ctx.Err() != nil {
				return
			}
			if err == nil {
				// 意外结束但没有具体错误时同样尝试恢复连接。
				log.Warn("telegram engine stopped unexpectedly; reconnecting")
			} else {
				log.Warn("telegram engine stopped; reconnecting", zap.Error(err), zap.Duration("retry_after", reconnectDelay))
			}

			timer := time.NewTimer(reconnectDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if reconnectDelay < maxReconnectDelay {
				reconnectDelay *= 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
			}
		}
	}()

	// 9. 启动 HTTP 服务（主 goroutine）
	srv := &http.Server{Addr: cfg.App.Host + ":" + cfg.App.Port, Handler: r}
	go func() {
		<-ctx.Done()
		sc, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = srv.Shutdown(sc)
	}()
	log.Info("http server starting", zap.String("port", cfg.App.Port))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("http server failed", zap.Error(err))
	}
	_ = engine.Close()
	log.Info("bye")
}
