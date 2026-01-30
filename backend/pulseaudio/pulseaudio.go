package pulseaudio

import (
	"fmt"
	"log"
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
	var volume float32
	var err error

	if volume, err = pa.client.Volume(); err != nil {
		log.Printf("failed to get client volume: %v", err)
	}
	if pa.server != nil {
		return &ServerInfo{
			Kind: 			pa.kind,
			Name: 			pa.server.PackageName,
			Version:		pa.server.PackageVersion,
			User:			pa.server.User,
			Hostname:		pa.server.Hostname,
			DefaultSink:	pa.server.DefaultSink,
			Volume:			volume,
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

func (pa *PulseAudioBackend) Close () {
	pa.client.Close()
}

func (pa *PulseAudioBackend) parseSinkInput(s pulseaudio.SinkInput) AudioClient {
	switch pa.kind {
	case ServerPipeWire:
		return pa.parsePipeWireSinkInput(s)
	default:
		return pa.parsePulseSinkInput(s)
	}
}

func (pa *PulseAudioBackend) parsePulseSinkInput(s pulseaudio.SinkInput) AudioClient {
	props := cloneProps(s.PropList)

	if props["media.icon_name"] == "audio-card-bluetooth" && strings.HasPrefix(props["media.name"], "Loopback from") {
		if client, ok := pa.parsePulseBluetoothSink(s, props); ok {
			return client
		}
		log.Printf("failed to resolve blueooth sink %s", s.Name)
	}

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

func (pa *PulseAudioBackend) parsePulseBluetoothSink(s pulseaudio.SinkInput, props map[string]string) (AudioClient, bool) {
	// récupérer le module-loopback
	mod, err := pa.findModule(s.OwnerModule, "module-loopback")
	if err != nil {
		return AudioClient{}, false
	}

	// extraire la source bluez
	sourceName := extractModuleSource(mod.Argument)
	if sourceName == "" {
		return AudioClient{}, false
	}

	// lookup de la source
	src, err := pa.findSourceByName(sourceName)
	if err != nil {
		return AudioClient{}, false
	}

	//
	btProps := cloneProps(props)
	for k, v := range src.PropList {
		btProps[k] = v
	}

	// enrichissement props
	name := src.PropList["device.description"]
	if name == "" {
		name = strings.TrimPrefix(props["media.name"], "Loopback from ")
	}

	return AudioClient{
		ID:      s.Index,
		Name:    name,
		App:     "bluetooth",
		Muted:   s.IsMute(),
		Volume:  s.GetVolume(),
		Corked:  s.Corked,
		Backend: ServerPulse,
		Binary:  "bluez",
		User:    "",
		Host:    name,
		Props:   btProps,
	}, true

}

func (pa *PulseAudioBackend) findModule(index uint32, name string) (*pulseaudio.Module, error) {
	mods, err := pa.client.ModuleList()
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		if m.Index == index && m.Name == name {
			return &m, nil
		}
	}
	return nil, fmt.Errorf("module %s %d  not found", name, index)
}


func (pa *PulseAudioBackend) findSourceByName(name string) (*pulseaudio.Source, error) {
	sources, err := pa.client.Sources()
	if err != nil {
		return nil, err
	}
	for _, s := range sources {
		if s.Name == name {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("source %s not found", name)
}

func extractModuleSource(arg string) string {
	// source="bluez_source.C8_2A_DD_A7_D5_0D.a2dp_source"
	for _, part := range strings.Fields(arg) {
		if strings.HasPrefix(part, "source=") {
			return strings.Trim(part[len("source="):], `"`)
		}
	}
	return ""
}
