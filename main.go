package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/b0bbywan/go-odio-api/api"
	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/config"
)

func main() {
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Context global pour toute l'application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// PulseAudio backend
	pa, err := pulseaudio.New(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// systemd backend
	sd, err := systemd.New(ctx, cfg.Services)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	api.Register(mux, pa, sd)

	// HTTP server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Goroutine pour signal handling
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutdown signal received, stopping server...")

		// Cancel le context global - arrête tous les listeners
		cancel()

		// Cleanup des backends
		sd.Close()
		pa.Close()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Println("Odio Audio API running on :8080")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("Server error: %v", err)
	}
	log.Println("Server stopped")
}
