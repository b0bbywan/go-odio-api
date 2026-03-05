package pulseaudio

import (
	"context"
	"sync"

	"github.com/the-jonsey/pulseaudio"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/events"
)

type AudioServerKind string

const (
	ServerPulse    AudioServerKind = "pulseaudio"
	ServerPipeWire AudioServerKind = "pipewire"
)

type PulseAudioBackend struct {
	ctx context.Context
	mu  sync.Mutex

	address string
	client  *pulseaudio.Client
	server  *pulseaudio.Server
	kind    AudioServerKind

	cache       *cache.Cache[[]AudioClient]
	outputCache *cache.Cache[[]AudioOutput]
	listener    *Listener
	events      chan events.Event
}

type ServerInfo struct {
	Kind        AudioServerKind `json:"kind"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	User        string          `json:"user"`
	Hostname    string          `json:"hostname"`
	DefaultSink string          `json:"default_sink"`
	Volume      float32         `json:"volume"`
}

type AudioOutput struct {
	Index       uint32            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Nick        string            `json:"nick,omitempty"`
	Muted       bool              `json:"muted"`
	Volume      float32           `json:"volume"`
	State       string            `json:"state"`
	Default     bool              `json:"default"`
	Driver      string            `json:"driver,omitempty"`
	ActivePort  string            `json:"active_port,omitempty"`
	IsNetwork   bool              `json:"is_network,omitempty"`
	Props       map[string]string `json:"props"`
}

type AudioClient struct {
	ID      uint32            `json:"id"`
	Name    string            `json:"name"` // media.name
	App     string            `json:"app"`  // application.name
	Muted   bool              `json:"muted"`
	Volume  float32           `json:"volume"`
	Corked  bool              `json:"corked"`
	Backend AudioServerKind   `json:"backend"` // pulse | pipewire
	Binary  string            `json:"binary,omitempty"`
	User    string            `json:"user,omitempty"`
	Host    string            `json:"host,omitempty"`
	Props   map[string]string `json:"props,omitempty"`
}
