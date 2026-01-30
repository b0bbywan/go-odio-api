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

const cacheKey = "clients"

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

	return backend, nil
}

// Start charge le cache initial et démarre le listener
func (pa *PulseAudioBackend) Start() error {
	// Charger le cache au démarrage
	if _, err := pa.ListClients(); err != nil {
		return err
	}

	// Démarrer le listener pour les changements pulseaudio
	pa.listener = NewListener(pa)
	if err := pa.listener.Start(); err != nil {
		return err
	}

	return nil
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

	// récupérer l'ancien cache
	oldClients, _ := pa.cache.Get(cacheKey)

	// générer le nouveau cache avec mises à jour/ajouts
	updatedClients := pa.mergeClients(oldClients, sinks)

	// Mettre en cache
	pa.cache.Set(cacheKey, updatedClients)

	return updatedClients, nil
}

func (pa *PulseAudioBackend) mergeClients(oldClients []AudioClient, sinks []pulseaudio.SinkInput) []AudioClient {
	// map temporaire pour lookup par Name
	oldMap := make(map[string]AudioClient, len(oldClients))
	for _, c := range oldClients {
		oldMap[c.Name] = c
	}

	// créer le nouveau slice et mettre à jour / ajouter les clients
	newClients := make([]AudioClient, 0, len(sinks))
	for _, s := range sinks {
		client := pa.parseSinkInput(s)
		client = pa.updateOrAddClient(oldMap, client)
		newClients = append(newClients, client)
	}

	// supprimer les clients disparus
	return pa.removeMissingClients(oldMap, newClients)
}

func (pa *PulseAudioBackend) updateOrAddClient(oldMap map[string]AudioClient, client AudioClient) AudioClient {
	if old, exists := oldMap[client.Name]; exists {
		if clientChanged(old, client) {
			oldMap[client.Name] = client
		}
		return oldMap[client.Name]
	}

	// nouveau client
	oldMap[client.Name] = client
	return client
}

func clientChanged(a, b AudioClient) bool {
	return a.Volume != b.Volume ||
		a.Muted != b.Muted ||
		a.Corked != b.Corked
}

func (pa *PulseAudioBackend) removeMissingClients(oldMap map[string]AudioClient, newClients []AudioClient) []AudioClient {
	final := make([]AudioClient, 0, len(newClients))
	for _, c := range newClients {
		if _, exists := oldMap[c.Name]; exists {
			final = append(final, c)
		}
	}
	return final
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
		log.Printf("failed to resolve bluetooth sink %s", s.Name)
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
	return nil, fmt.Errorf("module %s %d not found", name, index)
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
