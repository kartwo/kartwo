// 程序入口 / Application Entrypoint
// 功能：装配配置→数据层→迁移→HTTP 服务，并处理优雅关停（M0 骨架）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kartwo/kartwo/internal/admin"
	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/config"
	"github.com/kartwo/kartwo/internal/media"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/server"
	"github.com/kartwo/kartwo/internal/storefront"
	"github.com/kartwo/kartwo/internal/store"
	"github.com/kartwo/kartwo/migrations"
)

// Version 为构建版本，发布时经 ldflags 注入；默认开发占位。
var Version = "0.0.0-dev"

// generateDemoCover 生成一张自有版权的演示封面图（渐变 PNG），用于演示数据。
func generateDemoCover() []byte {
	const w, h = 1000, 1000
	c8 := func(v int) uint8 { // 限定到 0..255，转换显式安全
		switch {
		case v < 0:
			return 0
		case v > 255:
			return 255
		default:
			return uint8(v)
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: c8(56 + x*120/w), G: c8(120 + y*100/h), B: c8(200 - x*80/w), A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

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

// newMediaService 按配置构造媒体服务（serve 与 seed-demo 共用）。
func newMediaService(cfg *config.Config, st *store.Store) *media.Service {
	mediaRoot := filepath.Join(cfg.DataDir, "media")
	// 默认存储策略：不限总量，单文件 ≤10MiB，磁盘可用 <200MiB 时停新上传。
	return media.New(st.DB, media.NewLocalBackend(mediaRoot), media.NewDefaultPolicy(mediaRoot, 10<<20, 200<<20), 20)
}

// runSeedDemo 装入演示商品（含一张演示封面图）并打印变体矩阵，完成后退出。
func runSeedDemo(logger *slog.Logger) error {
	cfg, st, err := setup(logger)
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
		// 配一张自生成的演示封面图（无版权问题），让店面演示更完整。
		if _, err := newMediaService(cfg, st).Upload(ctx, pid, generateDemoCover()); err != nil {
			logger.Warn("演示封面上传失败（忽略）", "err", err)
		} else {
			logger.Info("演示封面已上传")
		}
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

	mediaSvc := newMediaService(cfg, st)
	adminHTTP := admin.NewHTTP(admin.New(st.DB), catalog.New(st.DB), mediaSvc, cfg.Env == "prod")
	storeHTTP := storefront.NewHTTP(storefront.New(st.DB), cart.New(st.DB), cfg.ShopName, cfg.Currency, cfg.BaseURL, cfg.Env == "prod")
	srv := server.New(cfg, st, Version, adminHTTP, storeHTTP)
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
