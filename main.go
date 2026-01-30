package main

import (
	"context"
	"log"
	"net/http"

	"github.com/b0bbywan/go-odio-api/api"
	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

func main() {
	// PulseAudio backend
	pa, err := pulseaudio.New()
	if err != nil {
		log.Fatal(err)
	}

	// systemd backend
	sd, err := systemd.New(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	api.Register(mux, pa, sd)

	log.Println("Odio Audio API running on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
