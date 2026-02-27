package pulseaudio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
	"github.com/the-jonsey/pulseaudio"
)

const cacheKey = "clients"

func New(ctx context.Context, cfg *config.PulseAudioConfig) (*PulseAudioBackend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	address := fmt.Sprintf("%s/pulse/native", cfg.XDGRuntimeDir)

	backend := &PulseAudioBackend{
		address: address,
		ctx:     ctx,
		cache:   cache.New[[]AudioClient](0), // TTL=0 = no expiration
		events:  make(chan events.Event, 32),
	}

	return backend, nil
}

// Start loads the initial cache and starts the listener
func (pa *PulseAudioBackend) Start() error {
	logger.Debug("[pulseaudio] starting backend")
	pa.mu.Lock()
	defer pa.mu.Unlock()
	var err error
	if pa.client, err = pulseaudio.NewClient(pa.address); err != nil {
		return err
	}

	if pa.server, err = pa.client.ServerInfo(); err != nil {
		return err
	}
	pa.kind = detectServerKind(pa.server)
	logger.Debug("[pulseaudio] detected server: %s (type=%s)", pa.server.PackageName, pa.kind)

	// Load the cache at startup
	if _, err := pa.ListClients(); err != nil {
		return err
	}

	// Start the listener for pulseaudio changes
	pa.listener = NewListener(pa)
	if err := pa.listener.Start(); err != nil {
		return err
	}

	go pa.heartbeat()

	logger.Info("[pulseaudio] backend started successfully")
	return nil
}

func (pa *PulseAudioBackend) Reconnect() error {
	pa.Close()

	return pa.Start()
}

func (pa *PulseAudioBackend) heartbeat() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-pa.ctx.Done():
			return
		case <-ticker.C:
			if pa.client == nil || !pa.client.Connected() {
				pa.reconnectWithBackoff()
				return
			}
		}
	}
}

func (pa *PulseAudioBackend) reconnectWithBackoff() {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-pa.ctx.Done():
			return
		default:
		}

		if err := pa.Reconnect(); err != nil {
			logger.Warn("[pulseaudio] reconnect failed, retry in %s", backoff)
			time.Sleep(backoff)

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		logger.Info("[pulseaudio] reconnected")
		return
	}
}

func (pa *PulseAudioBackend) ServerInfo() (*ServerInfo, error) {
	var volume float32
	var err error

	if volume, err = pa.client.Volume(); err != nil {
		logger.Warn("[pulseaudio] failed to get client volume: %v", err)
	}
	if pa.server != nil {
		return &ServerInfo{
			Kind:        pa.kind,
			Name:        pa.server.PackageName,
			Version:     pa.server.PackageVersion,
			User:        pa.server.User,
			Hostname:    pa.server.Hostname,
			DefaultSink: pa.server.DefaultSink,
			Volume:      volume,
		}, nil
	}

	return nil, fmt.Errorf("server info unavailable")
}

func (pa *PulseAudioBackend) ListClients() ([]AudioClient, error) {
	// Check the cache
	if cached, ok := pa.cache.Get(cacheKey); ok {
		logger.Debug("[pulseaudio] returning %d clients from cache", len(cached))
		return cached, nil
	}

	logger.Debug("[pulseaudio] cache miss, loading clients")
	return pa.refreshCache()
}

// refreshCache reloads from pulseaudio and updates the cache
func (pa *PulseAudioBackend) refreshCache() ([]AudioClient, error) {
	sinks, err := pa.client.SinkInputs()
	if err != nil {
		return nil, err
	}

	logger.Debug("[pulseaudio] loaded %d sink inputs", len(sinks))

	// retrieve the old cache
	oldClients, _ := pa.cache.Get(cacheKey)

	// generate the new cache with updates/additions
	updatedClients := pa.mergeClients(oldClients, sinks)

	// Cache it
	pa.cache.Set(cacheKey, updatedClients)

	return updatedClients, nil
}

func (pa *PulseAudioBackend) mergeClients(oldClients []AudioClient, sinks []pulseaudio.SinkInput) []AudioClient {
	// temporary map for lookup by Name
	oldMap := make(map[string]AudioClient, len(oldClients))
	for _, c := range oldClients {
		oldMap[c.Name] = c
	}

	// create the new slice and update / add clients
	newClients := make([]AudioClient, 0, len(sinks))
	for _, s := range sinks {
		client := pa.parseSinkInput(s)
		client = pa.updateOrAddClient(oldMap, client)
		newClients = append(newClients, client)
	}

	// remove missing clients
	return pa.removeMissingClients(oldMap, newClients)
}

func (pa *PulseAudioBackend) updateOrAddClient(oldMap map[string]AudioClient, client AudioClient) AudioClient {
	if old, exists := oldMap[client.Name]; exists {
		if clientChanged(old, client) {
			oldMap[client.Name] = client
		}
		return oldMap[client.Name]
	}

	// new client
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

// GetClient retrieves a specific client from the cache
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

// UpdateClient updates a specific client in the cache
func (pa *PulseAudioBackend) UpdateClient(updated AudioClient) error {
	clients, ok := pa.cache.Get(cacheKey)
	if !ok {
		// If no cache, reload everything
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
		// Client not in cache, add it
		clients = append(clients, updated)
	}

	pa.cache.Set(cacheKey, clients)
	return nil
}

// RefreshClient reloads a specific client from pulseaudio and updates the cache
func (pa *PulseAudioBackend) RefreshClient(name string) (*AudioClient, error) {
	sink, err := pa.client.GetSinkInputByName(name)
	if err != nil {
		// Client no longer exists, reload everything
		if _, err := pa.ListClients(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("client not found: %s", name)
	}

	client := pa.parseSinkInput(sink)

	// Update in the cache
	if err := pa.UpdateClient(client); err != nil {
		return nil, err
	}

	return &client, nil
}

// CacheUpdatedAt returns the last time the client cache was written to.
func (pa *PulseAudioBackend) CacheUpdatedAt() time.Time {
	return pa.cache.UpdatedAt()
}

// InvalidateCache invalidates the entire cache
func (pa *PulseAudioBackend) InvalidateCache() {
	pa.cache.Delete(cacheKey)
}

// Close cleanly closes the connections and stops the listener
func (pa *PulseAudioBackend) Close() {
	if pa.listener != nil {
		pa.listener.Stop()
		pa.listener = nil

	}
	if pa.client != nil {
		pa.client.Close()
		pa.client = nil
	}
	close(pa.events)
}

func (pa *PulseAudioBackend) notify(e events.Event) {
	select {
	case pa.events <- e:
	default:
		logger.Warn("[pulseaudio] event channel full, dropping %s event", e.Type)
	}
}

// Events returns the read-only event channel for this backend.
func (pa *PulseAudioBackend) Events() <-chan events.Event { return pa.events }

func (pa *PulseAudioBackend) ToggleMuteMaster() error {
	if _, err := pa.client.ToggleMute(); err != nil {
		return fmt.Errorf("failed to get default sink: %w", err)
	}
	return nil
}

func (pa *PulseAudioBackend) SetVolumeMaster(volume float32) error {
	return pa.client.SetVolume(volume)
}

func (pa *PulseAudioBackend) ToggleMute(name string) error {
	logger.Debug("[pulseaudio] toggling mute for client %q", name)
	sink, err := pa.client.GetSinkInputByName(name)
	if err != nil {
		return fmt.Errorf("failed to get sink input: %w", err)
	}

	if err := sink.ToggleMute(); err != nil {
		return err
	}
	return nil
}

func (pa *PulseAudioBackend) SetVolume(name string, vol float32) error {
	logger.Debug("[pulseaudio] setting volume for client %q to %.2f", name, vol)
	sink, err := pa.client.GetSinkInputByName(name)
	if err != nil {
		return fmt.Errorf("failed to get sink input: %w", err)
	}

	if err := sink.SetVolume(vol); err != nil {
		return err
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
		logger.Warn("[pulseaudio] failed to resolve bluetooth sink %s", s.Name)
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
	// retrieve the module-loopback
	mod, err := pa.findModule(s.OwnerModule, "module-loopback")
	if err != nil {
		return AudioClient{}, false
	}

	// extract the bluez source
	sourceName := extractModuleSource(mod.Argument)
	if sourceName == "" {
		return AudioClient{}, false
	}

	// source lookup
	src, err := pa.findSourceByName(sourceName)
	if err != nil {
		return AudioClient{}, false
	}

	btProps := cloneProps(props)
	for k, v := range src.PropList {
		btProps[k] = v
	}

	// enrich props
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
