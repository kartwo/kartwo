// 程序入口 / Application Entrypoint
// 功能：装配配置→数据层→迁移→HTTP 服务，并处理优雅关停（M0 骨架）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kartwo/kartwo/internal/catalog"
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

	// 子命令分发：默认 serve；seed-demo 装演示数据后退出。
	sub := "serve"
	if len(os.Args) > 1 {
		sub = os.Args[1]
	}

	var err error
	switch sub {
	case "serve":
		err = runServe(logger)
	case "seed-demo":
		err = runSeedDemo(logger)
	default:
		err = fmt.Errorf("未知子命令 %q（可用：serve | seed-demo）", sub)
	}
	if err != nil {
		logger.Error("执行失败", "subcommand", sub, "err", err)
		os.Exit(1)
	}
}

// setup 完成"配置→数据层→迁移"的公共装配，serve 与 seed-demo 共用。
func setup(logger *slog.Logger) (*config.Config, *store.Store, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	logger.Info("配置已加载", "env", cfg.Env, "addr", cfg.Addr, "data_dir", cfg.DataDir, "engine", cfg.DBEngine)

	st, err := store.Open(cfg)
	if err != nil {
		return nil, nil, err
	}

	// 启动即跑迁移：幂等可重入，未应用的才执行。
	applied, err := migrate.Run(context.Background(), st.DB, migrations.FS)
	if err != nil {
		_ = st.Close()
		return nil, nil, err
	}
	logger.Info("数据库迁移完成", "newly_applied", applied)
	return cfg, st, nil
}

// runSeedDemo 装入演示商品并打印变体矩阵（M1.1 验收用），完成后退出。
func runSeedDemo(logger *slog.Logger) error {
	_, st, err := setup(logger)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()
	svc := catalog.New(st.DB)
	pid, created, err := svc.SeedDemo(ctx)
	if err != nil {
		return err
	}
	if created {
		logger.Info("演示数据已装入", "product_id", pid)
	} else {
		logger.Info("演示数据已存在，跳过装入", "product_id", pid)
	}

	matrix, err := svc.GetVariantMatrix(ctx, pid)
	if err != nil {
		return err
	}
	fmt.Printf("\n演示商品变体矩阵（共 %d 个变体）：\n", len(matrix))
	for _, v := range matrix {
		opts := ""
		for i, p := range v.Options {
			if i > 0 {
				opts += " × "
			}
			opts += fmt.Sprintf("%s=%s", p.Name, p.Value)
		}
		fmt.Printf("  [%s] %-14s ¥%.2f  库存=%d  %s\n", v.PublicID[:8], v.SKU, float64(v.PriceCents)/100, v.Quantity, opts)
	}
	fmt.Println()
	return nil
}

func runServe(logger *slog.Logger) error {
	cfg, st, err := setup(logger)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

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
