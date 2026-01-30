package pulseaudio

import (
	"fmt"
	"os"
	"strings"

	"github.com/the-jonsey/pulseaudio"
)

func New() (*PulseAudioBackend, error) {
	xdgRuntimeDir, ok := os.LookupEnv("XDG_RUNTIME_DIR")
	if !ok {
		xdgRuntimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	addressArr := fmt.Sprintf("%s/pulse/native", xdgRuntimeDir)

	c, err := pulseaudio.NewClient(addressArr)
	if err != nil {
		return nil, err
	}
	server, err := c.ServerInfo()
	if err != nil {
		return nil, err
	}
	kind := detectServerKind(server)

	return &PulseAudioBackend{
		client: c,
		server: server,
		kind: kind,
	}, nil
}

func (pa *PulseAudioBackend) ServerInfo() (*ServerInfo, error) {
	if pa.server != nil {
		return &ServerInfo{
			Kind: 			pa.kind,
			Name: 			pa.server.PackageName,
			Version:		pa.server.PackageVersion,
			User:			pa.server.User,
			Hostname:		pa.server.Hostname,
			DefaultSink:	pa.server.DefaultSink,
		}, nil
	}

	return nil, fmt.Errorf("server info unavailable")
}

func (pa *PulseAudioBackend) ListClients() ([]AudioClient, error) {
	sinks, err := pa.client.SinkInputs()
	if err != nil {
		return nil, err
	}

	clients := make([]AudioClient, 0, len(sinks))

	for _, s := range sinks {
		clients = append(clients, pa.parseSinkInput(s))
}

	return clients, nil
}

func (pa *PulseAudioBackend) ToggleMuteMaster() error {
	if _, err := pa.client.ToggleMute(); err != nil {
		return fmt.Errorf("Failed to get default sink: %w", err)
	}
	return nil
}

func (pa *PulseAudioBackend) SetVolumeMaster(volume float32) error {
	return pa.client.SetVolume(volume)
}

func (pa *PulseAudioBackend) ToggleMute(name string) error {
	sink, err := pa.client.GetSinkInputByName(name)
	if err != nil {
		return fmt.Errorf("Failed to get Sink Input: %w", err)
	}

	return sink.ToggleMute()
}

func (pa *PulseAudioBackend) SetVolume(name string, vol float32) error {
	sink, err := pa.client.GetSinkInputByName(name)
	if err != nil {
		return fmt.Errorf("Failed to get Sink Input: %w", err)
	}
	return sink.SetVolume(vol)
}

func (pa *PulseAudioBackend) parseSinkInput(s pulseaudio.SinkInput) AudioClient {
	switch pa.kind {
	case ServerPipeWire:
		return parsePipeWireSinkInput(s)
	default:
		return parsePulseSinkInput(s)
	}
}

func parsePulseSinkInput(s pulseaudio.SinkInput) AudioClient {
	props := cloneProps(s.PropList)

	return AudioClient{
		ID:      s.Index,
		Name:    props["media.name"],
		App:     props["application.name"],
		Muted:   s.IsMute(),
		Volume:  s.GetVolume(),
		Corked:  s.Corked,
		Backend: ServerPulse,
		Binary:  props["application.process.binary"],
		User:    props["application.process.user"],
		Host:    props["application.process.host"],
		Props:   props,
	}
}

func detectServerKind(s *pulseaudio.Server) AudioServerKind {
	if strings.Contains(strings.ToLower(s.PackageName), "pipewire") {
		return ServerPipeWire
	}
	return ServerPulse
}

func cloneProps(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
