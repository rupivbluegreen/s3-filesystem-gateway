package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vipurkumar/s3-filesystem-gateway/internal/config"
)

func main() {
	configPath := flag.String("config", "configs/default.yaml", "path to configuration file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("starting s3-filesystem-gateway",
		"s3_endpoint", cfg.S3.Endpoint,
		"s3_bucket", cfg.S3.Bucket,
		"nfs_port", cfg.NFS.Port,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// TODO: Phase 1 implementation
	// 1. Initialize S3 client and verify bucket access
	// 2. Initialize bbolt metadata store
	// 3. Create S3 filesystem (libnfs-go fs.FS)
	// 4. Start NFSv4 server

	<-ctx.Done()
	slog.Info("s3-filesystem-gateway stopped")
}
