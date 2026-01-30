package pulseaudio

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/the-jonsey/pulseaudio"
)

func New(ctx context.Context) (*PulseAudioBackend, error) {
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

	backend := &PulseAudioBackend{
		client: c,
		server: server,
		kind:   kind,
		ctx:    ctx,
		cache:  cache.New[[]AudioClient](0), // TTL=0 = pas d'expiration
	}

	// Charger le cache au démarrage
	if _, err := backend.ListClients(); err != nil {
		return nil, err
	}

	// Démarrer le listener pour les changements pulseaudio
	backend.listener = NewListener(backend)
	if err := backend.listener.Start(); err != nil {
		return nil, err
	}

	return backend, nil
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

const cacheKey = "clients"

func (pa *PulseAudioBackend) ListClients() ([]AudioClient, error) {
	// Vérifier le cache
	if cached, ok := pa.cache.Get(cacheKey); ok {
		log.Printf("Returning %d clients from cache", len(cached))
		return cached, nil
	}

	log.Println("Cache miss, loading clients from pulseaudio")
	return pa.refreshCache()
}

// refreshCache recharge depuis pulseaudio et met à jour le cache
func (pa *PulseAudioBackend) refreshCache() ([]AudioClient, error) {
	sinks, err := pa.client.SinkInputs()
	if err != nil {
		return nil, err
	}

	log.Printf("Loaded %d sink inputs from pulseaudio", len(sinks))

	clients := make([]AudioClient, 0, len(sinks))

	for _, s := range sinks {
		clients = append(clients, pa.parseSinkInput(s))
	}

	log.Printf("Parsed %d clients, caching them", len(clients))

	// Mettre en cache
	pa.cache.Set(cacheKey, clients)

	return clients, nil
}

// GetClient récupère un client spécifique du cache
func (pa *PulseAudioBackend) GetClient(name string) (*AudioClient, bool) {
	clients, ok := pa.cache.Get(cacheKey)
	if !ok {
		return nil, false
	}

	for _, client := range clients {
		if client.Name == name {
			return &client, true
		}
	}
	return nil, false
}

// UpdateClient met à jour un client spécifique dans le cache
func (pa *PulseAudioBackend) UpdateClient(updated AudioClient) error {
	clients, ok := pa.cache.Get(cacheKey)
	if !ok {
		// Si pas de cache, on recharge tout
		_, err := pa.ListClients()
		return err
	}

	found := false
	for i, client := range clients {
		if client.Name == updated.Name {
			clients[i] = updated
			found = true
			break
		}
	}

	if !found {
		// Client pas dans le cache, on l'ajoute
		clients = append(clients, updated)
	}

	pa.cache.Set(cacheKey, clients)
	return nil
}

// RefreshClient recharge un client spécifique depuis pulseaudio et met à jour le cache
func (pa *PulseAudioBackend) RefreshClient(name string) (*AudioClient, error) {
	sink, err := pa.client.GetSinkInputByName(name)
	if err != nil {
		// Client n'existe plus, on recharge tout
		if _, err := pa.ListClients(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("client not found: %s", name)
	}

	client := pa.parseSinkInput(sink)

	// Mettre à jour dans le cache
	if err := pa.UpdateClient(client); err != nil {
		return nil, err
	}

	return &client, nil
}

// InvalidateCache invalide tout le cache
func (pa *PulseAudioBackend) InvalidateCache() {
	pa.cache.Delete(cacheKey)
}

// Close ferme proprement les connexions et arrête le listener
func (pa *PulseAudioBackend) Close() {
	if pa.listener != nil {
		pa.listener.Stop()
	}
	if pa.client != nil {
		pa.client.Close()
	}
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

	if err := sink.ToggleMute(); err != nil {
		return err
	}

	// Rafraîchir le client dans le cache
	if _, err := pa.RefreshClient(name); err != nil {
		log.Printf("Warning: failed to refresh client %q in cache: %v", name, err)
	}
	return nil
}

func (pa *PulseAudioBackend) SetVolume(name string, vol float32) error {
	sink, err := pa.client.GetSinkInputByName(name)
	if err != nil {
		return fmt.Errorf("Failed to get Sink Input: %w", err)
	}

	if err := sink.SetVolume(vol); err != nil {
		return err
	}

	// Rafraîchir le client dans le cache
	if _, err := pa.RefreshClient(name); err != nil {
		log.Printf("Warning: failed to refresh client %q in cache: %v", name, err)
	}
	return nil
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
