package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/b0bbywan/go-odio-api/api"
	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

func main() {
	if os.Getuid() == 0 {
		logger.Fatal("[%s] root user is strictly forbidden! Odio cannot and will not run as root.", config.AppName)
	}

	flag.Usage = usage
	configFile := flag.String("config", "", "path to configuration file")
	versionFlag := flag.Bool("version", false, "Print version")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s %s\n", config.AppName, config.AppVersion)
		return
	}

	cfg, err := config.New(configFile)
	if err != nil {
		logger.Fatal("[%s] Failed to load config: %v", config.AppName, err)
	}

	// Set log level from config
	logger.SetLevel(cfg.LogLevel)

	// Global context for the entire application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize backends
	b, err := backend.New(
		ctx,
		cfg.Bluetooth,
		cfg.Login1,
		cfg.MPRIS,
		cfg.Pulseaudio,
		cfg.Systemd,
		cfg.Zeroconf,
	)
	if err != nil {
		logger.Fatal("[%s] Backend initialization failed: %v", config.AppName, err)
	}

	// Start enabled backend
	if err := b.Start(); err != nil {
		logger.Fatal("[%s] Backend start failed: %v", config.AppName, err)
	}

	// New api server
	server := api.NewServer(cfg.Api, b)

	// Channel to synchronize shutdown
	shutdownDone := make(chan struct{})
	// Goroutine for signal handling
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		logger.Info("[%s] Shutdown signal received, stopping server...", config.AppName)
		clear(b, cancel, shutdownDone)
	}()

	logger.Info("[%s] started", config.AppName)
	if server != nil {
		if err := server.Run(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("[%s] http server error: %v", config.AppName, err)
			clear(b, cancel, shutdownDone)
		}
	}

	<-shutdownDone
	logger.Info("[%s] stopped", config.AppName)
}

func clear(b *backend.Backend, cancel context.CancelFunc, shutdown chan struct{}) {
	// Cancel the global context - stops all listeners
	cancel()

	// Cleanup backends
	b.Close()

	// Signal that cleanup is complete
	close(shutdown)
}

func usage() {
	fmt.Println("Usage:")
	fmt.Println("  odio-api [options]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --config <path>  configuration file to use")
	fmt.Println("  --version        Display version")
	fmt.Println("  -h, --help       this help message")
}
