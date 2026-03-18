package pulseaudio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
	"github.com/the-jonsey/pulseaudio"
)

const (
	cacheKey       = "clients"
	outputCacheKey = "outputs"
)

func New(ctx context.Context, cfg *config.PulseAudioConfig) (*PulseAudioBackend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	address := fmt.Sprintf("%s/pulse/native", cfg.XDGRuntimeDir)

	backend := &PulseAudioBackend{
		address:     address,
		serveCookie: cfg.ServeCookie,
		ctx:         ctx,
		cache:       cache.New[[]AudioClient](0),
		outputCache: cache.New[[]AudioOutput](0),
		events:      make(chan events.Event, 32),
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
	if _, err := pa.ListOutputs(); err != nil {
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
	pa.closeConnections()
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
	outputs, ok := pa.outputCache.Get(outputCacheKey)
	if !ok {
		return nil, &NotReadyError{Message: "output cache not ready"}
	}

	for _, o := range outputs {
		if o.Default {
			return &ServerInfo{
				Kind:        pa.kind,
				DefaultSink: o.Name,
				Volume:      o.Volume,
				Muted:       o.Muted,
			}, nil
		}
	}

	return nil, &NotFoundError{Resource: "sink", Name: "default"}
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
		return nil, &NotFoundError{Resource: "client", Name: name}
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

// closeConnections stops the listener and closes the client without closing the events channel.
// Used internally for reconnects.
func (pa *PulseAudioBackend) closeConnections() {
	if pa.listener != nil {
		pa.listener.Stop()
		pa.listener = nil
	}
	if pa.client != nil {
		pa.client.Close()
		pa.client = nil
	}
}

// Close cleanly closes connections and shuts down the event channel.
// Called only at program shutdown.
func (pa *PulseAudioBackend) Close() {
	pa.closeConnections()
	close(pa.events)
}

func (pa *PulseAudioBackend) notify(e events.Event) {
	select {
	case pa.events <- e:
		logger.Debug("[pulseaudio] emitted %s event", e.Type)
	default:
		logger.Warn("[pulseaudio] event channel full, dropping %s event", e.Type)
	}
}

// Events returns the read-only event channel for this backend.
func (pa *PulseAudioBackend) Events() <-chan events.Event { return pa.events }

// Kind returns the detected audio server kind (pulseaudio or pipewire).
func (pa *PulseAudioBackend) Kind() AudioServerKind { return pa.kind }

// Cookie returns the PulseAudio cookie file contents, or DisabledError if not enabled.
func (pa *PulseAudioBackend) Cookie() ([]byte, error) {
	if !pa.serveCookie {
		return nil, &DisabledError{Feature: "cookie"}
	}
	return pa.cookie()
}

func (pa *PulseAudioBackend) cookie() ([]byte, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config dir: %w", err)
	}
	return os.ReadFile(filepath.Join(configDir, "pulse", "cookie"))
}

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
	return nil, &NotFoundError{Resource: "module", Name: fmt.Sprintf("%s %d", name, index)}
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
	return nil, &NotFoundError{Resource: "source", Name: name}
}

func (pa *PulseAudioBackend) ListOutputs() ([]AudioOutput, error) {
	if cached, ok := pa.outputCache.Get(outputCacheKey); ok {
		logger.Debug("[pulseaudio] returning %d outputs from cache", len(cached))
		return cached, nil
	}

	logger.Debug("[pulseaudio] output cache miss, loading outputs")
	return pa.refreshOutputCache()
}

func (pa *PulseAudioBackend) refreshOutputCache() ([]AudioOutput, error) {
	srv, err := pa.client.ServerInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get server info: %w", err)
	}

	sinks, err := pa.client.Sinks()
	if err != nil {
		return nil, err
	}

	logger.Debug("[pulseaudio] loaded %d sinks", len(sinks))

	outputs := make([]AudioOutput, 0, len(sinks))
	for _, s := range sinks {
		outputs = append(outputs, pa.parseSink(s, srv.DefaultSink))
	}

	pa.outputCache.Set(outputCacheKey, outputs)
	return outputs, nil
}

func (pa *PulseAudioBackend) GetOutput(name string) (*AudioOutput, bool) {
	outputs, ok := pa.outputCache.Get(outputCacheKey)
	if !ok {
		return nil, false
	}
	for _, o := range outputs {
		if o.Name == name {
			return &o, true
		}
	}
	return nil, false
}

func (pa *PulseAudioBackend) UpdateOutput(updated AudioOutput) error {
	outputs, ok := pa.outputCache.Get(outputCacheKey)
	if !ok {
		_, err := pa.ListOutputs()
		return err
	}

	found := false
	for i, o := range outputs {
		if o.Name == updated.Name {
			outputs[i] = updated
			found = true
			break
		}
	}
	if !found {
		outputs = append(outputs, updated)
	}

	pa.outputCache.Set(outputCacheKey, outputs)
	return nil
}

func (pa *PulseAudioBackend) OutputCacheUpdatedAt() time.Time {
	return pa.outputCache.UpdatedAt()
}

func (pa *PulseAudioBackend) SetDefaultOutput(name string) error {
	logger.Debug("[pulseaudio] setting default output to %q", name)
	return pa.client.SetDefaultSink(name)
}

func (pa *PulseAudioBackend) ToggleMuteOutput(name string) error {
	logger.Debug("[pulseaudio] toggling mute for output %q", name)
	sink, err := pa.findSinkByName(name)
	if err != nil {
		return err
	}
	return sink.ToggleMute()
}

func (pa *PulseAudioBackend) SetVolumeOutput(name string, vol float32) error {
	logger.Debug("[pulseaudio] setting volume for output %q to %.2f", name, vol)
	sink, err := pa.findSinkByName(name)
	if err != nil {
		return err
	}
	return sink.SetVolume(vol)
}

func (pa *PulseAudioBackend) findSinkByName(name string) (*pulseaudio.Sink, error) {
	sinks, err := pa.client.Sinks()
	if err != nil {
		return nil, err
	}
	for _, s := range sinks {
		if s.Name == name {
			return &s, nil
		}
	}
	return nil, &NotFoundError{Resource: "sink", Name: name}
}

func (pa *PulseAudioBackend) parseSink(s pulseaudio.Sink, defaultName string) AudioOutput {
	switch pa.kind {
	case ServerPipeWire:
		return pa.parsePipeWireSink(s, defaultName)
	default:
		return pa.parsePulseSink(s, defaultName)
	}
}

func (pa *PulseAudioBackend) parsePulseSink(s pulseaudio.Sink, defaultName string) AudioOutput {
	props := cloneProps(s.PropList)
	const paNetworkFlag uint32 = 0x20000
	return AudioOutput{
		Index:       s.Index,
		Name:        s.Name,
		Description: s.Description,
		Nick:        props["device.description"],
		Muted:       s.IsMute(),
		Volume:      s.GetVolume(),
		State:       sinkStateString(s.SinkState),
		Default:     s.Name == defaultName,
		Driver:      s.Driver,
		ActivePort:  s.ActivePortName,
		IsNetwork:   s.Flags&paNetworkFlag != 0,
		Props:       props,
	}
}

func sinkStateString(state uint32) string {
	switch state {
	case 0:
		return "running"
	case 1:
		return "idle"
	case 2:
		return "suspended"
	default:
		return "unknown"
	}
}

func diffOutputs(old, new []AudioOutput) (changed []AudioOutput, removed []AudioOutput) {
	newByName := make(map[string]struct{}, len(new))
	for _, o := range new {
		newByName[o.Name] = struct{}{}
	}

	oldByName := make(map[string]AudioOutput, len(old))
	for _, o := range old {
		oldByName[o.Name] = o
		if _, exists := newByName[o.Name]; !exists {
			removed = append(removed, o)
		}
	}

	for _, o := range new {
		prev, exists := oldByName[o.Name]
		if !exists || outputChanged(prev, o) {
			changed = append(changed, o)
		}
	}
	return
}

func outputChanged(a, b AudioOutput) bool {
	return a.Volume != b.Volume ||
		a.Muted != b.Muted ||
		a.State != b.State ||
		a.Default != b.Default
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
