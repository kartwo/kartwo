// 程序入口 / Application Entrypoint
// 功能：装配配置→数据层→迁移→HTTP 服务，并处理优雅关停（M0 骨架）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kartwo/kartwo/internal/config"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/server"
	"github.com/kartwo/kartwo/internal/store"
	"github.com/kartwo/kartwo/migrations"
)

// Version 为构建版本，发布时经 ldflags 注入；默认开发占位。
var Version = "0.0.0-dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("启动失败", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Info("配置已加载", "env", cfg.Env, "addr", cfg.Addr, "data_dir", cfg.DataDir, "engine", cfg.DBEngine)

	st, err := store.Open(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	// 启动即跑迁移：幂等可重入，未应用的才执行。
	applied, err := migrate.Run(context.Background(), st.DB, migrations.FS)
	if err != nil {
		return err
	}
	logger.Info("数据库迁移完成", "newly_applied", applied)

	srv := server.New(cfg, st, Version)
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 优雅关停：监听 SIGINT/SIGTERM。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("HTTP 服务启动", "addr", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("收到关停信号，开始优雅关停")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
