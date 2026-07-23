package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/b0bbywan/go-odio-api/logger"
)

// APIClient makes HTTP requests to the local JSON API
type APIClient struct {
	baseURL string
	client  *http.Client
}

// NewAPIClient creates a new internal API client.
// It always connects to 127.0.0.1, which is guaranteed to be in the server's listen list.
func NewAPIClient(port int) *APIClient {
	return &APIClient{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *APIClient) GetServerInfo() (*ServerInfo, error) {
	var v ServerInfo
	if err := c.get("/server", &v); err != nil {
		return nil, err
	}
	if v.Backends.Power {
		var power PowerCapabilities
		if err := c.get("/power", &power); err == nil {
			v.Power = &power
		}
	}
	return &v, nil
}

func (c *APIClient) GetPlayers() ([]PlayerView, error) {
	resp, err := c.client.Get(c.baseURL + "/players")
	if err != nil {
		return nil, fmt.Errorf("/players: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close response body for /players")
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/players: unexpected status %d", resp.StatusCode)
	}
	var raw []Player
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("/players: decode failed: %w", err)
	}
	views := convertPlayers(raw)
	c.attachTracklists(raw, views)
	return views, nil
}

// attachTracklists fetches the tracklist of each supporting player and fills
// the matching view. Failures only cost the tracklist, not the section.
func (c *APIClient) attachTracklists(raw []Player, views []PlayerView) {
	supported := make(map[string]*Player, len(raw))
	for i := range raw {
		if raw[i].TracklistSupported {
			supported[raw[i].Name] = &raw[i]
		}
	}
	for i := range views {
		p, ok := supported[views[i].Name]
		if !ok {
			continue
		}
		tl, err := c.GetTracklist(p.Name)
		if err != nil {
			logger.Warn("[ui] failed to fetch tracklist for %s: %v", p.Name, err)
			continue
		}
		views[i].CanEditTracks = tl.CanEditTracks
		views[i].Tracks = convertTracks(tl.Tracks, p.Metadata["mpris:trackid"])
	}
}

func (c *APIClient) GetTracklist(name string) (*TracklistResponse, error) {
	var v TracklistResponse
	if err := c.get("/players/"+name+"/tracklist", &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func convertTracks(tracks []Track, currentID string) []TrackView {
	views := make([]TrackView, 0, len(tracks))
	for _, t := range tracks {
		label := t.Metadata["xesam:title"]
		if label == "" {
			label = path.Base(t.TrackID)
		}
		views = append(views, TrackView{
			Ref:     url.PathEscape(t.TrackID),
			Label:   label,
			Artist:  t.Metadata["xesam:artist"],
			Current: t.TrackID == currentID,
		})
	}
	return views
}

func convertPlayers(raw []Player) []PlayerView {
	views := make([]PlayerView, 0, len(raw))
	for _, p := range raw {
		if p.Status != "Playing" && p.Status != "Paused" {
			continue
		}
		displayName := playerDisplayName(p)
		artUrl := ""
		if rawArt := p.Metadata["mpris:artUrl"]; rawArt != "" {
			// Cache-bust on trackid AND artUrl: trackid alone misses Chrome's
			// case where it keeps a stable trackid across track changes but
			// rotates the art file path; artUrl alone misses players that
			// reuse the same art file across distinct tracks.
			q := url.Values{
				"t": {p.Metadata["mpris:trackid"]},
				"a": {rawArt},
			}
			artUrl = "/players/" + p.Name + "/cover?" + q.Encode()
		}
		hasLoop := p.LoopStatus != nil
		loopVal := ""
		if hasLoop {
			loopVal = *p.LoopStatus
		}
		views = append(views, PlayerView{
			Name:              p.Name,
			DisplayName:       displayName,
			Artist:            p.Metadata["xesam:artist"],
			Title:             p.Metadata["xesam:title"],
			Album:             p.Metadata["xesam:album"],
			ArtUrl:            artUrl,
			State:             p.Status,
			Volume:            p.Volume,
			CanPlay:           p.Capabilities.CanPlay,
			CanPause:          p.Capabilities.CanPause,
			CanNext:           p.Capabilities.CanGoNext,
			CanPrev:           p.Capabilities.CanGoPrevious,
			CanStop:           p.Capabilities.CanControl,
			CanShuffle:        hasLoop,
			Shuffle:           p.Shuffle,
			CanLoop:           hasLoop,
			LoopStatus:        loopVal,
			Position:          p.Position,
			Duration:          parseMicros(p.Metadata["mpris:length"]),
			Rate:              p.Rate,
			CanSeek:           p.Capabilities.CanSeek,
			PositionUpdatedAt: p.PositionUpdatedAt.Format(time.RFC3339Nano),
		})
	}
	return views
}

// parseMicros parses a microsecond string from MPRIS metadata (e.g. mpris:length).
func parseMicros(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

// playerDisplayName picks the cleanest available label for a player. Identity
// (set by the player itself, e.g. "Chrome", "Spotify") is already user-readable
// per the MPRIS spec, so we use it as-is. Only when Identity is missing do we
// fall back to stripping the org.mpris.MediaPlayer2. prefix and any
// .instanceXXX suffix, then capitalizing the first letter.
func playerDisplayName(p Player) string {
	if p.Identity != "" {
		return p.Identity
	}
	name := strings.TrimPrefix(p.Name, "org.mpris.MediaPlayer2.")
	if i := strings.Index(name, "."); i >= 0 {
		name = name[:i]
	}
	if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
		name = string(name[0]-32) + name[1:]
	}
	return name
}

func (c *APIClient) GetAudio() (*AudioData, error) {
	var raw struct {
		Kind    string        `json:"kind"`
		Clients []AudioClient `json:"clients"`
		Outputs []AudioOutput `json:"outputs"`
	}
	if err := c.get("/audio", &raw); err != nil {
		return nil, err
	}
	clients := make([]AudioClient, 0, len(raw.Clients))
	for _, cl := range raw.Clients {
		if !cl.Corked {
			clients = append(clients, cl)
		}
	}
	data := &AudioData{
		Kind:    raw.Kind,
		Clients: clients,
		Outputs: raw.Outputs,
	}
	for i := range raw.Outputs {
		if raw.Outputs[i].Default {
			data.DefaultSink = &raw.Outputs[i]
			break
		}
	}
	return data, nil
}

func (c *APIClient) GetBluetoothStatus() (*BluetoothView, error) {
	var raw BluetoothStatus
	if err := c.get("/bluetooth", &raw); err != nil {
		return nil, err
	}
	return convertBluetooth(&raw), nil
}

func convertBluetooth(raw *BluetoothStatus) *BluetoothView {
	if raw == nil {
		return nil
	}
	connected := 0
	for _, d := range raw.KnownDevices {
		if d.Connected {
			connected++
		}
	}
	var untilMs int64
	if raw.PairingActive && raw.PairingUntil != nil {
		untilMs = raw.PairingUntil.UnixMilli()
	}
	// The backend lists devices in BlueZ map order (non-deterministic), so sort
	// for a stable display that doesn't jump on every SSE re-render: connected
	// first (the active device), then freshly discovered (not-yet-bonded) ones
	// — what you act on during a scan — then the rest of the known devices, each
	// group by name.
	devices := raw.KnownDevices
	sort.SliceStable(devices, func(i, j int) bool {
		a, b := devices[i], devices[j]
		if a.Connected != b.Connected {
			return a.Connected
		}
		if a.Bonded != b.Bonded {
			return !a.Bonded
		}
		return a.Label() < b.Label()
	})
	return &BluetoothView{
		Powered:        raw.Powered,
		PairingActive:  raw.PairingActive,
		PairingUntilMs: untilMs,
		Scanning:       raw.Scanning,
		ConnectedCount: connected,
		Devices:        devices,
	}
}

func (c *APIClient) GetServices() ([]ServiceView, error) {
	var raw []Service
	if err := c.get("/services", &raw); err != nil {
		return nil, err
	}
	return convertServices(raw), nil
}

// GetUpgrade returns the detector status from /upgrade. The endpoint yields
// JSON null when no detection has run; that decodes to a zero status (Known()
// is then false), which the badge renders as a neutral "check" prompt.
func (c *APIClient) GetUpgrade() (*UpgradeStatus, error) {
	var v UpgradeStatus
	if err := c.get("/upgrade", &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func convertServices(raw []Service) []ServiceView {
	views := make([]ServiceView, 0, len(raw))
	for _, s := range raw {
		views = append(views, ServiceView{
			Name:        s.Name,
			Description: s.Description,
			Active:      s.ActiveState == "active",
			State:       s.SubState,
			IsUser:      s.Scope == "user",
			URL:         s.URL,
		})
	}
	return views
}

// get performs a GET request and decodes the JSON response into dest.
func (c *APIClient) get(path string, dest any) error {
	resp, err := c.client.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close response body for %s", path)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: unexpected status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("%s: decode failed: %w", path, err)
	}
	return nil
}
