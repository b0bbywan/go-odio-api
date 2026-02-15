package ui

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/b0bbywan/go-odio-api/logger"
)

// APIClient makes HTTP requests to the local JSON API
type APIClient struct {
	baseURL string
	client  *http.Client
}

// NewAPIClient creates a new internal API client using the server's listen address.
// If the host is 0.0.0.0 (all interfaces), 127.0.0.1 is used instead.
func NewAPIClient(listenAddr string) *APIClient {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil || host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return &APIClient{
		baseURL: "http://" + net.JoinHostPort(host, port),
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
	return &v, nil
}

func (c *APIClient) GetPlayers() ([]Player, error) {
	var v []Player
	if err := c.get("/players", &v); err != nil {
		return nil, err
	}
	return v, nil
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

func (c *APIClient) GetBluetoothStatus() (*BluetoothStatus, error) {
	var v BluetoothStatus
	if err := c.get("/bluetooth", &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *APIClient) GetServices() ([]Service, error) {
	var v []Service
	if err := c.get("/services", &v); err != nil {
		return nil, err
	}
	return v, nil
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
