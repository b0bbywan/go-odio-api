package pulseaudio

import (
	"github.com/the-jonsey/pulseaudio"
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
}

type ServerInfo struct {
	Kind 		AudioServerKind `json:"kind"`
	Name 		string          `json:"name"`
	Version 	string			`json:"version"`
	User 		string 			`json:"user"`
	Hostname 	string 			`json:"hostname"`
	DefaultSink string			`json:"default_sink"`
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
