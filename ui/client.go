package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	return convertPlayers(raw, resp.Header.Get("X-Cache-Updated-At")), nil
}

func convertPlayers(raw []Player, cacheUpdatedAt string) []PlayerView {
	views := make([]PlayerView, 0, len(raw))
	for _, p := range raw {
		displayName := strings.TrimPrefix(p.Name, "org.mpris.MediaPlayer2.")
		artUrl := p.Metadata["mpris:artUrl"]
		if !strings.HasPrefix(artUrl, "http://") && !strings.HasPrefix(artUrl, "https://") {
			artUrl = ""
		}
		views = append(views, PlayerView{
			Name:           p.Name,
			DisplayName:    displayName,
			Artist:         p.Metadata["xesam:artist"],
			Title:          p.Metadata["xesam:title"],
			Album:          p.Metadata["xesam:album"],
			ArtUrl:         artUrl,
			State:          p.Status,
			Volume:         p.Volume,
			CanPlay:        p.Capabilities.CanPlay,
			CanPause:       p.Capabilities.CanPause,
			CanNext:        p.Capabilities.CanGoNext,
			CanPrev:        p.Capabilities.CanGoPrevious,
			Position:       p.Position,
			Duration:       parseMicros(p.Metadata["mpris:length"]),
			Rate:           p.Rate,
			CanSeek:        p.Capabilities.CanSeek,
			CacheUpdatedAt: cacheUpdatedAt,
		})
	}
	return views
}

// parseMicros parses a microsecond string from MPRIS metadata (e.g. mpris:length).
func parseMicros(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func (c *APIClient) GetAudioInfo() (*AudioInfo, error) {
	var v AudioInfo
	if err := c.get("/audio/server", &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *APIClient) GetAudioClients() ([]AudioClient, error) {
	var v []AudioClient
	if err := c.get("/audio/clients", &v); err != nil {
		return nil, err
	}
	return v, nil
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
	secsLeft := 0
	if raw.PairingActive && raw.PairingUntil != nil {
		if d := time.Until(*raw.PairingUntil); d > 0 {
			secsLeft = int(d.Seconds())
		}
	}
	return &BluetoothView{
		Powered:            raw.Powered,
		PairingActive:      raw.PairingActive,
		PairingSecondsLeft: secsLeft,
		ConnectedCount:     connected,
	}
}

func (c *APIClient) GetServices() ([]ServiceView, error) {
	var raw []Service
	if err := c.get("/services", &raw); err != nil {
		return nil, err
	}
	return convertServices(raw), nil
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
