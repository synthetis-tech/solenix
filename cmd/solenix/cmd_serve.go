package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bbvtaev/solenix"
	"github.com/bbvtaev/solenix/collector"
	"github.com/bbvtaev/solenix/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the solenix-core server",
	RunE:  runServe,
}

var configPath string

func init() {
	serveCmd.Flags().StringVar(&configPath, "config", "", "path to solenix.yaml config file")
}

func runServe(_ *cobra.Command, _ []string) error {
	var cfg solenix.Config
	var err error

	if configPath != "" {
		cfg, err = solenix.LoadConfig(configPath)
		if err != nil {
			return err
		}
		slog.Info("loaded config", "path", configPath)
	} else {
		cfg = solenix.DefaultConfig()
		slog.Info("using default config")
	}

	db, err := solenix.Open(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	slog.Info("solenix-core started",
		"data_dir", cfg.DataDir+"/"+cfg.Database,
		"grpc_addr", cfg.GRPCAddr,
		"retention", cfg.RetentionDuration,
	)

	srv := server.New(db)
	go func() {
		slog.Info("gRPC server listening", "addr", cfg.GRPCAddr)
		if err := srv.Listen(fmt.Sprintf(":%d", cfg.GRPCAddr)); err != nil {
			slog.Error("gRPC server error", "err", err)
			os.Exit(1)
		}
	}()

	if cfg.HTTPAddr != 0 {
		httpSrv := server.NewHTTP(db, cfg)
		go func() {
			slog.Info("UI available", "url", "http://localhost"+fmt.Sprintf(":%d", cfg.HTTPAddr))
			if err := httpSrv.ListenHTTP(fmt.Sprintf(":%d", cfg.HTTPAddr)); err != nil {
				slog.Error("HTTP server error", "err", err)
			}
		}()
	}

	if cfg.Collector.Enabled {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c := collector.New(db, cfg.Collector)
		go c.Run(ctx)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("shutting down", "signal", sig)
	return nil
}
