// Package main is the entry point for the version control agent gateway.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cocohub/git-gateway/internal/auth"
	"github.com/cocohub/git-gateway/internal/config"
	"github.com/cocohub/git-gateway/internal/middleware"
	"github.com/cocohub/git-gateway/internal/policy"
	"github.com/cocohub/git-gateway/internal/proxy"
	"github.com/joho/godotenv"
)

func main() {
	configPath := flag.String("config", "gateway.yaml", "Path to configuration file")
	envPath := flag.String("env", ".env", "Path to .env file (optional)")
	flag.Parse()

	// Load .env file if it exists (don't fail if missing)
	_ = godotenv.Load(*envPath)

	// Setup initial logger (will be replaced after config load)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Create config manager with hot-reload support
	cfgMgr, err := config.NewManager(*configPath, logger)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	cfg := cfgMgr.Config()

	// Setup logger with config settings
	var logHandler slog.Handler
	logLevel := parseLogLevel(cfg.Log.Level)

	if cfg.Log.Format == "json" {
		logHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	logger = slog.New(logHandler)
	slog.SetDefault(logger)

	// Create gateway with initial config
	authenticator := auth.NewAPIKeyAuthenticator(cfg.Agents)
	policyEngine := policy.NewEngine(cfg.Agents)
	gateway := proxy.NewGateway(authenticator, policyEngine, cfg.Upstreams, logger)

	// Register reload callback to update gateway config
	cfgMgr.OnReload(func(newCfg *config.Config) {
		newAuth := auth.NewAPIKeyAuthenticator(newCfg.Agents)
		newPolicy := policy.NewEngine(newCfg.Agents)
		gateway.UpdateConfig(newAuth, newPolicy, newCfg.Upstreams)
	})

	// Start watching config file for changes
	if err := cfgMgr.Watch(); err != nil {
		logger.Error("failed to start config watcher", "error", err)
		// Continue without hot-reload
	}

	// Setup middleware chain
	var handler http.Handler = gateway
	handler = middleware.Logging(logger)(handler)
	handler = middleware.Recovery(logger)(handler)

	// Create server
	server := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("starting gateway", "addr", cfg.Server.Listen)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Handle signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigs {
		switch sig {
		case syscall.SIGHUP:
			// Manual reload trigger
			logger.Info("received SIGHUP, reloading config")
			if err := cfgMgr.Reload(); err != nil {
				logger.Error("config reload failed", "error", err)
			}
		case syscall.SIGINT, syscall.SIGTERM:
			// Shutdown
			logger.Info("shutting down gracefully...")
			cfgMgr.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := server.Shutdown(ctx); err != nil {
				logger.Error("shutdown error", "error", err)
			}
			cancel()

			logger.Info("server stopped")
			return
		}
	}
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
