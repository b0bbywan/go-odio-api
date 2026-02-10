package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// APIClient makes HTTP requests to the local JSON API
type APIClient struct {
	baseURL string
	client  *http.Client
}

// NewAPIClient creates a new internal API client
func NewAPIClient(port int) *APIClient {
	return &APIClient{
		baseURL: fmt.Sprintf("http://localhost:%d", port),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetServerInfo fetches server information from /server
func (c *APIClient) GetServerInfo() (*ServerInfo, error) {
	var info ServerInfo
	err := c.get("/server", &info)
	return &info, err
}

// GetPlayers fetches MPRIS players from /players
func (c *APIClient) GetPlayers() ([]Player, error) {
	var players []Player
	err := c.get("/players", &players)
	if err != nil {
		return nil, err
	}
	return players, nil
}

// GetAudioInfo fetches PulseAudio server info from /audio/server
func (c *APIClient) GetAudioInfo() (*AudioInfo, error) {
	var info AudioInfo
	err := c.get("/audio/server", &info)
	return &info, err
}

// GetAudioClients fetches PulseAudio clients from /audio/clients
func (c *APIClient) GetAudioClients() ([]AudioClient, error) {
	var clients []AudioClient
	err := c.get("/audio/clients", &clients)
	if err != nil {
		return nil, err
	}
	return clients, nil
}

// GetServices fetches systemd services from /services
func (c *APIClient) GetServices() ([]Service, error) {
	var services []Service
	err := c.get("/services", &services)
	if err != nil {
		return nil, err
	}
	return services, nil
}

// get is a helper method for GET requests
func (c *APIClient) get(path string, dest interface{}) error {
	resp, err := c.client.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}
