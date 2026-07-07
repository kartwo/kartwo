// 程序入口 / Application Entrypoint
// 功能：装配配置→数据层→迁移→HTTP 服务，并处理优雅关停（M0 骨架）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kartwo/kartwo/internal/admin"
	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/config"
	"github.com/kartwo/kartwo/internal/media"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/payment"
	"github.com/kartwo/kartwo/internal/server"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/internal/store"
	"github.com/kartwo/kartwo/internal/storefront"
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
	settingsSvc := settings.New(st.DB)

	// 收款密钥内存缓存（绑定 KEK 金库：登录解锁/登出销毁）+ 支付编排服务。
	payCache := payment.NewKeyCache(settingsSvc)
	adminSvc := admin.New(st.DB)
	adminSvc.SetPaymentKeys(payCache)
	paySvc := payment.NewService(st.DB, settingsSvc, payCache)
	// 仅记录密钥「来源」，绝不打印任何密钥值。
	payStatus := payCache.Status()
	logger.Info("收款密钥来源", "stripe", payStatus.StripeSource, "stripe_mode", payStatus.StripeMode,
		"paypal", payStatus.PayPalSource, "paypal_mode", payStatus.PayPalMode)
	// Stripe env 半设：设了 secret 却没设 whsec —— 明确告警，且绝不回退加密库取 whsec。
	if payStatus.StripeSource == "env" && payStatus.StripeHasSecret && !payStatus.StripeHasWebhook {
		logger.Warn("env Stripe 密钥不完整",
			"detail", "已设 STRIPE_SECRET_KEY 但缺 STRIPE_WEBHOOK_SECRET；Webhook 将不可用(返 503)，且不会回退加密库取 whsec",
			"fix", "请补设 STRIPE_WEBHOOK_SECRET(由 stripe listen 现场生成)，或清空全部 STRIPE_* 改用后台加密库")
	}
	// LIVE 防误：env 路径无 §7 沙箱兜底，正式模式显眼告警。
	if payStatus.StripeSource == "env" && payStatus.StripeMode == "live" {
		logger.Warn("⚠️ Stripe 处于 LIVE 正式模式", "detail", "env 路径无沙箱兜底，将产生真实收款；请确认")
	}
	if payStatus.PayPalSource == "env" && payStatus.PayPalMode == "live" {
		logger.Warn("⚠️ PayPal 处于 LIVE 正式模式", "detail", "env 路径无沙箱兜底，将产生真实收款；请确认")
	}

	orderSvc := order.New(st.DB, settingsSvc)
	adminHTTP := admin.NewHTTP(adminSvc, catalog.New(st.DB), mediaSvc, settingsSvc, orderSvc, paySvc, cfg.Domain, cfg.Env == "prod")
	storeHTTP := storefront.NewHTTP(storefront.New(st.DB), cart.New(st.DB), orderSvc, settingsSvc, paySvc, cfg.ShopName, cfg.BaseURL, cfg.Env == "prod")
	payHTTP := payment.NewHTTP(paySvc)
	// 解析"当前生效域名"（env 覆盖 DB），决定是否启用 HTTPS（仅 prod）。
	baseCtx := context.Background()
	domain, domainSource := "", "none"
	tlsEnabled := false
	if cfg.Env == "prod" {
		domain, domainSource = server.EffectiveDomain(baseCtx, cfg.Domain, settingsSvc)
		tlsEnabled = domain != ""
	}
	// HSTS 门控：仅 HTTPS 真正启用时注入（HTTP-only 评估态/dev 不发）。
	srv := server.New(cfg, st, Version, adminHTTP, storeHTTP, payHTTP, tlsEnabled)

	// 优雅关停：监听 SIGINT/SIGTERM。
	ctx, stop := signal.NotifyContext(baseCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var servers []*http.Server
	errCh := make(chan error, 1)
	// serveOn 在给定地址起一个 http.Server（明文），绑定失败给人话提示。
	serveOn := func(addr string, h http.Handler, role string) error {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return listenHint(addr, err)
		}
		s := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
		servers = append(servers, s)
		go func() {
			logger.Info("HTTP 服务启动", "addr", addr, "role", role)
			if err := s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
		return nil
	}

	switch {
	case cfg.Env != "prod":
		// dev：纯 HTTP，现状不变。
		if err := serveOn(cfg.Addr, srv, "dev-http"); err != nil {
			return err
		}
	case !tlsEnabled:
		// prod HTTP-only 评估态（正式受支持状态）：未配域名，服应用于 :80，绝不发 HSTS。
		logger.Warn("HTTP-only 评估态：未配置域名，未启用 HTTPS",
			"http_addr", cfg.HTTPAddr, "fix", "在向导/后台配置域名并重启后自动签发 HTTPS")
		if err := serveOn(cfg.HTTPAddr, srv, "prod-http-eval"); err != nil {
			return err
		}
	default:
		// prod + 域名：内嵌 autocert 自动签发/续期 HTTPS。
		mgr, err := server.NewCertManager(domain, filepath.Join(cfg.DataDir, "certs"), cfg.ACMEDirectory)
		if err != nil {
			return err
		}
		logger.Info("自动 HTTPS 已启用", "domain", domain, "domain_source", domainSource,
			"https_addr", cfg.HTTPSAddr, "http_addr", cfg.HTTPAddr, "acme", acmeLabel(cfg.ACMEDirectory))
		// :80 服 ACME HTTP-01 challenge，其余 301 跳 HTTPS。
		if err := serveOn(cfg.HTTPAddr, server.ChallengeHandler(mgr, domain), "prod-http-challenge"); err != nil {
			return err
		}
		// :443 服真应用（TLS，证书由 autocert 按需签发）。
		tlsLn, err := net.Listen("tcp", cfg.HTTPSAddr)
		if err != nil {
			return listenHint(cfg.HTTPSAddr, err)
		}
		s := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
		servers = append(servers, s)
		go func() {
			logger.Info("HTTPS 服务启动", "addr", cfg.HTTPSAddr, "role", "prod-https")
			if err := s.Serve(tls.NewListener(tlsLn, server.TLSConfig(mgr))); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("收到关停信号，开始优雅关停")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var firstErr error
		for _, s := range servers {
			if err := s.Shutdown(shutdownCtx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
}

// listenHint 把端口绑定失败转成人话提示：特权端口(<1024)权限不足时给出 setcap/systemd/root 三条解法。
func listenHint(addr string, err error) error {
	if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "permission denied") {
		return fmt.Errorf("绑定 %s 被拒（特权端口 <1024 需权限）："+
			"用 `sudo setcap 'cap_net_bind_service=+ep' <二进制>` 授权，"+
			"或用 systemd 加 AmbientCapabilities=CAP_NET_BIND_SERVICE，或以 root 运行；"+
			"亦可设 KARTWO_HTTP_ADDR/KARTWO_HTTPS_ADDR 换高位端口经反代转发。原始错误：%w", addr, err)
	}
	return fmt.Errorf("绑定 %s 失败：%w", addr, err)
}

// acmeLabel 为日志给出 ACME 目录的可读标签（空=LE 生产）。
func acmeLabel(dir string) string {
	if dir == "" {
		return "letsencrypt-production"
	}
	return dir
}
