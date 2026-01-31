package pulseaudio

import (
	"context"

	"github.com/the-jonsey/pulseaudio"

	"github.com/b0bbywan/go-odio-api/cache"

)

type AudioServerKind string

const (
	ServerPulse    AudioServerKind = "pulseaudio"
	ServerPipeWire AudioServerKind = "pipewire"
)

type PulseAudioBackend struct {
	client *pulseaudio.Client
	server *pulseaudio.Server
	kind   AudioServerKind
	ctx    context.Context

	// cache permanent (pas d'expiration)
	cache *cache.Cache[[]AudioClient]

	// listener pour les changements pulseaudio
	listener *Listener
}

type ServerInfo struct {
	Kind 		AudioServerKind `json:"kind"`
	Name 		string          `json:"name"`
	Version 	string			`json:"version"`
	User 		string 			`json:"user"`
	Hostname 	string 			`json:"hostname"`
	DefaultSink string			`json:"default_sink"`
	Volume		float32			`json:"volume"`
}

type AudioClient struct {
	ID       uint32            `json:"id"`
	Name     string            `json:"name"` // media.name
	App      string            `json:"app"`  // application.name
	Muted    bool              `json:"muted"`
	Volume   float32           `json:"volume"`
	Corked   bool              `json:"corked"`
	Backend  AudioServerKind   `json:"backend"` // pulse | pipewire
	Binary   string            `json:"binary,omitempty"`
	User     string            `json:"user,omitempty"`
	Host     string            `json:"host,omitempty"`
	Props    map[string]string `json:"props,omitempty"`
}
