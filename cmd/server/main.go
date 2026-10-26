// Command server 启动 phanthycode2api 网关。
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"phanthycode2api/internal/config"
	"phanthycode2api/internal/credential"
	"phanthycode2api/internal/gateway"
	"phanthycode2api/internal/keepalive"
	"phanthycode2api/internal/phanthy"
	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/protocol"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径；留空则自动尝试 ./config.json，并可用 P2A_* 环境变量覆盖")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.LstdFlags)

	path := *configPath
	if path == "" {
		if _, err := os.Stat("config.json"); err == nil {
			path = "config.json"
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		logger.Fatalf("配置加载失败: %v", err)
	}

	creds, warnings := credential.LoadDir(cfg.AuthDir)
	for _, w := range warnings {
		logger.Printf("凭证警告: %v", w)
	}
	if len(creds) == 0 {
		logger.Printf("警告: %s 下没有可用凭证，服务会一直返回 503", cfg.AuthDir)
	} else {
		logger.Printf("已加载 %d 个账号凭证", len(creds))
	}

	p, err := pool.New(creds, cfg.StateFile)
	if err != nil {
		logger.Fatalf("账号池初始化失败: %v", err)
	}

	fp := phanthy.NewFingerprint(phanthy.FingerprintConfig{
		Enabled:      cfg.Fingerprint.Enabled,
		CliVersion:   cfg.Fingerprint.CliVersion,
		Client:       cfg.Fingerprint.Client,
		ClientSource: cfg.Fingerprint.ClientSource,
		Product:      cfg.Fingerprint.Product,
		UserAgent:    cfg.Fingerprint.UserAgent,
		DeviceJSON:   cfg.Fingerprint.DeviceJSON,
	})
	if fp == nil {
		logger.Printf("提示: 指纹头注入已关闭")
	}

	client := phanthy.New(phanthy.Options{
		BaseURL:     cfg.BaseURL,
		Timeout:     cfg.UpstreamTimeout,
		Fingerprint: fp,
	})

	gw := gateway.New(cfg, p, client, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler := keepalive.New(client, p, logger, cfg.Schedule.KeepaliveHours, cfg.ProbeInterval)
	scheduler.Start(ctx)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           gw.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("监听 %s（上游 %s，模型别名 %d 个）", cfg.Listen, cfg.BaseURL, len(protocol.ModelList()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			logger.Fatalf("HTTP 服务异常退出: %v", err)
		}
	case sig := <-sigCh:
		logger.Printf("收到信号 %s，开始优雅退出", sig)
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("优雅退出超时: %v", err)
	}
}
