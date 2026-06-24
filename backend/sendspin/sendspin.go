// Package sendspin embeds a Sendspin multi-room audio player and exposes it as
// an optional, experimental backend. It is pure Go: audio is decoded with the
// FLAC/PCM decoders and played through PulseAudio/PipeWire via pulseOutput, so
// the patched sendspin-go fork is built with cgo disabled (no miniaudio/opus).
package sendspin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Sendspin/sendspin-go/pkg/discovery"
	ssp "github.com/Sendspin/sendspin-go/pkg/sendspin"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// ErrNotConnected is returned by control methods when no player is running.
var ErrNotConnected = errors.New("sendspin player not connected")

type SendspinBackend struct {
	ctx    context.Context
	cfg    *config.SendspinConfig
	output *pulseOutput
	events chan events.Event

	mu         sync.RWMutex
	player     *ssp.Player
	serverAddr string
	state      ssp.PlayerState
	metadata   ssp.Metadata
}

// Status is the JSON-facing snapshot of the player exposed at GET /sendspin and
// carried on sendspin events.
type Status struct {
	Connected  bool   `json:"connected"`
	State      string `json:"state"`
	Volume     int    `json:"volume"`
	Muted      bool   `json:"muted"`
	Codec      string `json:"codec,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
	Channels   int    `json:"channels,omitempty"`
	BitDepth   int    `json:"bit_depth,omitempty"`
	ServerAddr string `json:"server_addr,omitempty"`
	Track      *Track `json:"track,omitempty"`
}

type Track struct {
	Title      string `json:"title,omitempty"`
	Artist     string `json:"artist,omitempty"`
	Album      string `json:"album,omitempty"`
	ArtworkURL string `json:"artwork_url,omitempty"`
	Duration   int    `json:"duration,omitempty"`
}

func New(ctx context.Context, cfg *config.SendspinConfig) (*SendspinBackend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}
	return &SendspinBackend{
		ctx:    ctx,
		cfg:    cfg,
		output: newPulseOutput(config.AppName),
		events: make(chan events.Event, 16),
	}, nil
}

func (s *SendspinBackend) Start() error {
	logger.Debug("[sendspin] starting backend")

	addr := s.cfg.ServerAddr
	if addr == "" {
		discovered, err := s.discoverServer()
		if err != nil {
			return fmt.Errorf("[sendspin] no server address configured and discovery failed: %w", err)
		}
		addr = discovered
		logger.Info("[sendspin] discovered server at %s", addr)
	}

	clientID, err := ssp.ResolveClientID(s.cfg.ClientID, "", nil)
	if err != nil {
		return fmt.Errorf("[sendspin] resolve client id: %w", err)
	}

	reconnect := ssp.ReconnectConfig{Enabled: true}
	if s.cfg.ServerAddr == "" {
		// Address came from mDNS; re-run discovery on reconnect in case the
		// server moved.
		reconnect.Rediscover = s.rediscover
	}

	player, err := ssp.NewPlayer(ssp.PlayerConfig{
		ServerAddr:     addr,
		PlayerName:     s.cfg.PlayerName,
		Volume:         s.cfg.Volume,
		ClientID:       clientID,
		PreferredCodec: "flac", // opus needs cgo libopus; flac/pcm are pure Go
		Output:         s.output,
		DeviceInfo: ssp.DeviceInfo{
			ProductName:     config.AppName,
			Manufacturer:    "odio",
			SoftwareVersion: config.AppVersion,
		},
		OnStateChange: s.onStateChange,
		OnMetadata:    s.onMetadata,
		OnError:       s.onError,
		Reconnect:     reconnect,
	})
	if err != nil {
		return fmt.Errorf("[sendspin] create player: %w", err)
	}

	s.mu.Lock()
	s.player = player
	s.serverAddr = addr
	s.mu.Unlock()

	if err := player.Connect(); err != nil {
		return fmt.Errorf("[sendspin] connect to %s: %w", addr, err)
	}
	if err := player.Play(); err != nil {
		return fmt.Errorf("[sendspin] play: %w", err)
	}

	logger.Info("[sendspin] player %q connected to %s", s.cfg.PlayerName, addr)
	return nil
}

func (s *SendspinBackend) Close() {
	s.mu.Lock()
	p := s.player
	s.player = nil
	s.mu.Unlock()

	if p != nil {
		if err := p.Close(); err != nil {
			logger.Info("[sendspin] player close: %v", err)
		}
	}
	if err := s.output.Close(); err != nil {
		logger.Info("[sendspin] output close: %v", err)
	}
	close(s.events)
}

// discoverServer browses for a Sendspin server via mDNS and returns the first
// one found, or an error after the configured timeout.
func (s *SendspinBackend) discoverServer() (string, error) {
	mgr := discovery.NewManager(discovery.Config{ServiceName: s.cfg.PlayerName})
	if err := mgr.Browse(); err != nil {
		return "", err
	}
	defer mgr.Stop()

	timeout := s.cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	select {
	case <-s.ctx.Done():
		return "", s.ctx.Err()
	case info := <-mgr.Servers():
		if info == nil {
			return "", errors.New("discovery closed without a server")
		}
		return net.JoinHostPort(info.Host, strconv.Itoa(info.Port)), nil
	case <-time.After(timeout):
		return "", fmt.Errorf("no server found within %s", timeout)
	}
}

func (s *SendspinBackend) rediscover(context.Context) (string, error) {
	return s.discoverServer()
}

func (s *SendspinBackend) Play() error  { return s.control((*ssp.Player).Play) }
func (s *SendspinBackend) Pause() error { return s.control((*ssp.Player).Pause) }
func (s *SendspinBackend) Stop() error  { return s.control((*ssp.Player).Stop) }
func (s *SendspinBackend) SetMuted(m bool) error {
	return s.control(func(p *ssp.Player) error { return p.Mute(m) })
}

func (s *SendspinBackend) SetVolume(volume int) error {
	return s.control(func(p *ssp.Player) error { return p.SetVolume(volume) })
}

func (s *SendspinBackend) control(fn func(*ssp.Player) error) error {
	s.mu.RLock()
	p := s.player
	s.mu.RUnlock()
	if p == nil {
		return ErrNotConnected
	}
	return fn(p)
}

func (s *SendspinBackend) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st := Status{
		Connected:  s.state.Connected,
		State:      s.state.State,
		Volume:     s.state.Volume,
		Muted:      s.state.Muted,
		Codec:      s.state.Codec,
		SampleRate: s.state.SampleRate,
		Channels:   s.state.Channels,
		BitDepth:   s.state.BitDepth,
		ServerAddr: s.serverAddr,
	}
	if s.metadata.Title != "" || s.metadata.Artist != "" || s.metadata.Album != "" {
		st.Track = &Track{
			Title:      s.metadata.Title,
			Artist:     s.metadata.Artist,
			Album:      s.metadata.Album,
			ArtworkURL: s.metadata.ArtworkURL,
			Duration:   s.metadata.Duration,
		}
	}
	return st
}

func (s *SendspinBackend) onStateChange(state ssp.PlayerState) {
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	s.notify(events.TypeSendspinUpdated)
}

func (s *SendspinBackend) onMetadata(m ssp.Metadata) {
	s.mu.Lock()
	s.metadata = m
	s.mu.Unlock()
	s.notify(events.TypeSendspinMetadata)
}

func (s *SendspinBackend) onError(err error) {
	logger.Warn("[sendspin] player error: %v", err)
}

func (s *SendspinBackend) notify(t string) {
	e := events.Event{Type: t, Data: s.Status()}
	select {
	case s.events <- e:
	default:
		logger.Warn("[sendspin] event channel full, dropping %s event", t)
	}
}

// Events returns the read-only event channel for this backend.
func (s *SendspinBackend) Events() <-chan events.Event { return s.events }
