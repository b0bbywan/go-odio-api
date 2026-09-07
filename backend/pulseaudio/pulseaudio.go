package pulseaudio

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
	"github.com/jfreymuth/pulse/proto"
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
		events:      make(chan events.Event, 32),
	}

	return backend, nil
}

// Start loads the initial cache and starts the listener
func (pa *PulseAudioBackend) Start() error {
	logger.Debug("[pulseaudio] starting backend")
	pa.mu.Lock()
	defer pa.mu.Unlock()
	if err := pa.connect(); err != nil {
		return err
	}

	var err error
	if pa.server, err = pa.serverInfo(); err != nil {
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
			if pa.client == nil || !pa.connected.Load() {
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
	outputs := pa.outputCache.Load()
	if outputs == nil {
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
	if cached := pa.cache.Load(); cached != nil {
		logger.Debug("[pulseaudio] returning %d clients from cache", len(cached))
		return cached, nil
	}

	logger.Debug("[pulseaudio] cache miss, loading clients")
	return pa.refreshCache()
}

// refreshCache reloads from pulseaudio and updates the cache
func (pa *PulseAudioBackend) refreshCache() ([]AudioClient, error) {
	sinks, err := pa.sinkInputs()
	if err != nil {
		return nil, err
	}

	logger.Debug("[pulseaudio] loaded %d sink inputs", len(sinks))

	// retrieve the old cache
	oldClients := pa.cache.Load()

	// generate the new cache with updates/additions
	updatedClients := pa.mergeClients(oldClients, sinks)

	// Cache it
	pa.cache.Store(updatedClients)

	return updatedClients, nil
}

func (pa *PulseAudioBackend) mergeClients(oldClients []AudioClient, sinks []*proto.GetSinkInputInfoReply) []AudioClient {
	// temporary map for lookup by Name
	oldMap := make(map[string]AudioClient, len(oldClients))
	for _, c := range oldClients {
		oldMap[c.Name] = c
	}

	// clients absent from sinks are dropped by construction
	newClients := make([]AudioClient, 0, len(sinks))
	for _, s := range sinks {
		client := pa.parseSinkInput(s)
		client = pa.updateOrAddClient(oldMap, client)
		newClients = append(newClients, client)
	}
	return newClients
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

// GetClient retrieves a specific client from the cache
func (pa *PulseAudioBackend) GetClient(name string) (*AudioClient, bool) {
	clients := pa.cache.Load()
	if clients == nil {
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
	clients := pa.cache.Load()
	if clients == nil {
		// If no cache, reload everything
		_, err := pa.ListClients()
		return err
	}
	clients = slices.Clone(clients)

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

	pa.cache.Store(clients)
	return nil
}

// removeClient drops the client with the given sink input index from the cache.
func (pa *PulseAudioBackend) removeClient(index uint32) (AudioClient, bool) {
	clients := pa.cache.Load()
	for i, c := range clients {
		if c.ID == index {
			pa.cache.Store(slices.Delete(slices.Clone(clients), i, i+1))
			return c, true
		}
	}
	return AudioClient{}, false
}

// RefreshClient reloads a specific client from pulseaudio and updates the cache
func (pa *PulseAudioBackend) RefreshClient(name string) (*AudioClient, error) {
	sink, err := pa.findSinkInput(name)
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
	pa.cache.Reset()
}

// closeConnections stops the listener and closes the client without closing the events channel.
// Used internally for reconnects.
func (pa *PulseAudioBackend) closeConnections() {
	if pa.listener != nil {
		pa.listener.Stop()
		pa.listener = nil
	}
	if pa.client != nil {
		if err := pa.conn.Close(); err != nil {
			logger.Warn("[pulseaudio] failed to close connection: %v", err)
		}
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
	sink, err := pa.findDefaultSink()
	if err != nil {
		return fmt.Errorf("failed to get default sink: %w", err)
	}
	return pa.setSinkMute(sink, !sink.Mute)
}

func (pa *PulseAudioBackend) SetVolumeMaster(volume float32) error {
	sink, err := pa.findDefaultSink()
	if err != nil {
		return fmt.Errorf("failed to get default sink: %w", err)
	}
	return pa.setSinkVolume(sink, volume)
}

func (pa *PulseAudioBackend) findDefaultSink() (*proto.GetSinkInfoReply, error) {
	srv, err := pa.serverInfo()
	if err != nil {
		return nil, err
	}
	return pa.findSinkByName(srv.DefaultSinkName)
}

func (pa *PulseAudioBackend) ToggleMute(name string) error {
	logger.Debug("[pulseaudio] toggling mute for client %q", name)
	sink, err := pa.findSinkInput(name)
	if err != nil {
		return err
	}

	if err := pa.setSinkInputMute(sink, !sink.Muted); err != nil {
		return err
	}
	return nil
}

func (pa *PulseAudioBackend) SetVolume(name string, vol float32) error {
	logger.Debug("[pulseaudio] setting volume for client %q to %.2f", name, vol)
	sink, err := pa.findSinkInput(name)
	if err != nil {
		return err
	}

	if err := pa.setSinkInputVolume(sink, vol); err != nil {
		return err
	}

	return nil
}

// findSinkInput matches a sink input by the same derived name the parsers
// expose, so clients registering empty names stay addressable.
func (pa *PulseAudioBackend) findSinkInput(name string) (*proto.GetSinkInputInfoReply, error) {
	inputs, err := pa.sinkInputs()
	if err != nil {
		return nil, fmt.Errorf("failed to list sink inputs: %w", err)
	}
	for _, s := range inputs {
		if sinkInputMatchesName(cloneProps(s.Properties), name) {
			return s, nil
		}
	}
	return nil, &NotFoundError{Resource: "client", Name: name}
}

// sinkInputMatchesName accepts both the raw PulseAudio stream name and the
// Bluetooth display name exposed by parsePulseBluetoothSink. Bluetooth A2DP
// inputs are implemented as module-loopback streams whose raw media.name is
// "Loopback from <device>", while the API deliberately exposes just <device>.
func sinkInputMatchesName(props map[string]string, name string) bool {
	if strings.EqualFold(clientName(props), name) {
		return true
	}
	if props["media.icon_name"] == "audio-card-bluetooth" {
		return strings.EqualFold(strings.TrimPrefix(props["media.name"], "Loopback from "), name)
	}
	return false
}

func (pa *PulseAudioBackend) parseSinkInput(s *proto.GetSinkInputInfoReply) AudioClient {
	switch pa.kind {
	case ServerPipeWire:
		return pa.parsePipeWireSinkInput(s)
	default:
		return pa.parsePulseSinkInput(s)
	}
}

func (pa *PulseAudioBackend) parsePulseSinkInput(s *proto.GetSinkInputInfoReply) AudioClient {
	props := cloneProps(s.Properties)

	if props["media.icon_name"] == "audio-card-bluetooth" && strings.HasPrefix(props["media.name"], "Loopback from") {
		if client, ok := pa.parsePulseBluetoothSink(s, props); ok {
			return client
		}
		logger.Warn("[pulseaudio] failed to resolve bluetooth sink %s", s.MediaName)
	}

	return AudioClient{
		ID:      s.SinkInputIndex,
		Name:    clientName(props),
		App:     props["application.name"],
		Muted:   s.Muted,
		Volume:  volumeOf(s.ChannelVolumes),
		Corked:  s.Corked,
		Backend: ServerPulse,
		Binary:  props["application.process.binary"],
		User:    props["application.process.user"],
		Host:    props["application.process.host"],
		Props:   props,
	}
}

// clientName is a client's routing and display name: media.name, falling back
// to application.name then the process binary for streams that register empty
// names (e.g. spotifyd).
func clientName(props map[string]string) string {
	for _, key := range []string{"media.name", "application.name", "application.process.binary"} {
		if v := props[key]; v != "" {
			return v
		}
	}
	return ""
}

func detectServerKind(s *proto.GetServerInfoReply) AudioServerKind {
	if strings.Contains(strings.ToLower(s.PackageName), "pipewire") {
		return ServerPipeWire
	}
	return ServerPulse
}

// cloneProps converts a protocol property list into a plain string map.
func cloneProps(in proto.PropList) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v.String()
	}
	return out
}

func (pa *PulseAudioBackend) parsePulseBluetoothSink(s *proto.GetSinkInputInfoReply, props map[string]string) (AudioClient, bool) {
	// retrieve the module-loopback
	mod, err := pa.findModule(s.ModuleIndex, "module-loopback")
	if err != nil {
		return AudioClient{}, false
	}

	// extract the bluez source
	sourceName := extractModuleSource(mod.ModuleArgs)
	if sourceName == "" {
		return AudioClient{}, false
	}

	// source lookup
	src, err := pa.findSourceByName(sourceName)
	if err != nil {
		return AudioClient{}, false
	}

	btProps := maps.Clone(props)
	for k, v := range src.Properties {
		btProps[k] = v.String()
	}

	// enrich props
	name := btProps["device.description"]
	if name == "" {
		name = strings.TrimPrefix(props["media.name"], "Loopback from ")
	}

	return AudioClient{
		ID:      s.SinkInputIndex,
		Name:    name,
		App:     "bluetooth",
		Muted:   s.Muted,
		Volume:  volumeOf(s.ChannelVolumes),
		Corked:  s.Corked,
		Backend: ServerPulse,
		Binary:  "bluez",
		User:    "",
		Host:    name,
		Props:   btProps,
	}, true

}

func (pa *PulseAudioBackend) findModule(index uint32, name string) (*proto.GetModuleInfoReply, error) {
	mods, err := pa.modules()
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		if m.ModuleIndex == index && m.ModuleName == name {
			return m, nil
		}
	}
	return nil, &NotFoundError{Resource: "module", Name: fmt.Sprintf("%s %d", name, index)}
}

func (pa *PulseAudioBackend) findSourceByName(name string) (*proto.GetSourceInfoReply, error) {
	sources, err := pa.sources()
	if err != nil {
		return nil, err
	}
	for _, s := range sources {
		if s.SourceName == name {
			return s, nil
		}
	}
	return nil, &NotFoundError{Resource: "source", Name: name}
}

func (pa *PulseAudioBackend) ListOutputs() ([]AudioOutput, error) {
	if cached := pa.outputCache.Load(); cached != nil {
		logger.Debug("[pulseaudio] returning %d outputs from cache", len(cached))
		return cached, nil
	}

	logger.Debug("[pulseaudio] output cache miss, loading outputs")
	return pa.refreshOutputCache()
}

func (pa *PulseAudioBackend) refreshOutputCache() ([]AudioOutput, error) {
	srv, err := pa.serverInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get server info: %w", err)
	}

	sinks, err := pa.sinks()
	if err != nil {
		return nil, err
	}

	logger.Debug("[pulseaudio] loaded %d sinks", len(sinks))

	outputs := make([]AudioOutput, 0, len(sinks))
	for _, s := range sinks {
		outputs = append(outputs, pa.parseSink(s, srv.DefaultSinkName))
	}

	pa.outputCache.Store(outputs)
	return outputs, nil
}

func (pa *PulseAudioBackend) GetOutput(name string) (*AudioOutput, bool) {
	outputs := pa.outputCache.Load()
	if outputs == nil {
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
	outputs := pa.outputCache.Load()
	if outputs == nil {
		_, err := pa.ListOutputs()
		return err
	}
	outputs = slices.Clone(outputs)

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

	pa.outputCache.Store(outputs)
	return nil
}

// removeOutput drops the output with the given sink index from the cache.
func (pa *PulseAudioBackend) removeOutput(index uint32) (AudioOutput, bool) {
	outputs := pa.outputCache.Load()
	for i, o := range outputs {
		if o.Index == index {
			pa.outputCache.Store(slices.Delete(slices.Clone(outputs), i, i+1))
			return o, true
		}
	}
	return AudioOutput{}, false
}

func (pa *PulseAudioBackend) OutputCacheUpdatedAt() time.Time {
	return pa.outputCache.UpdatedAt()
}

func (pa *PulseAudioBackend) SetDefaultOutput(name string) error {
	logger.Debug("[pulseaudio] setting default output to %q", name)
	return pa.setDefaultSink(name)
}

func (pa *PulseAudioBackend) ToggleMuteOutput(name string) error {
	logger.Debug("[pulseaudio] toggling mute for output %q", name)
	sink, err := pa.findSinkByName(name)
	if err != nil {
		return err
	}
	return pa.setSinkMute(sink, !sink.Mute)
}

func (pa *PulseAudioBackend) SetVolumeOutput(name string, vol float32) error {
	logger.Debug("[pulseaudio] setting volume for output %q to %.2f", name, vol)
	sink, err := pa.findSinkByName(name)
	if err != nil {
		return err
	}
	return pa.setSinkVolume(sink, vol)
}

func (pa *PulseAudioBackend) findSinkByName(name string) (*proto.GetSinkInfoReply, error) {
	sinks, err := pa.sinks()
	if err != nil {
		return nil, err
	}
	for _, s := range sinks {
		if s.SinkName == name {
			return s, nil
		}
	}
	return nil, &NotFoundError{Resource: "sink", Name: name}
}

func (pa *PulseAudioBackend) parseSink(s *proto.GetSinkInfoReply, defaultName string) AudioOutput {
	switch pa.kind {
	case ServerPipeWire:
		return pa.parsePipeWireSink(s, defaultName)
	default:
		return pa.parsePulseSink(s, defaultName)
	}
}

func (pa *PulseAudioBackend) parsePulseSink(s *proto.GetSinkInfoReply, defaultName string) AudioOutput {
	props := cloneProps(s.Properties)
	const paNetworkFlag uint32 = 0x20000
	return AudioOutput{
		Index:       s.SinkIndex,
		Name:        s.SinkName,
		Description: s.Device,
		Nick:        props["device.description"],
		Muted:       s.Mute,
		Volume:      volumeOf(s.ChannelVolumes),
		State:       sinkStateString(s.State),
		Default:     s.SinkName == defaultName,
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
