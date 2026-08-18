// Package gluetun implements portforward.TorrentClient against the Gluetun
// control server, authenticated with an API key.
package gluetun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client manages the port forwarded by a Gluetun VPN container.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New connects to the Gluetun control server at baseURL using apiKey,
// verifying reachability and authentication up front.
func New(ctx context.Context, baseURL, apiKey string) (*Client, error) {
	c := &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}

	if _, err := c.getPort(ctx); err != nil {
		return nil, fmt.Errorf("connect to gluetun: %v", err)
	}
	return c, nil
}

// Name identifies the client in log messages.
func (c *Client) Name() string { return "gluetun" }

// GetPort returns the port currently forwarded by Gluetun.
func (c *Client) GetPort() (int, error) {
	return c.getPort(context.Background())
}

// SetPort overrides Gluetun's forwarded ports with just port.
func (c *Client) SetPort(port int) error {
	body, err := json.Marshal(map[string]any{"ports": []int{port}})
	if err != nil {
		return err
	}
	_, err = c.request(context.Background(), http.MethodPut, "/v1/portforward", body)
	return err
}

func (c *Client) getPort(ctx context.Context) (int, error) {
	var res struct {
		Port int `json:"port"`
	}
	if err := c.get(ctx, "/v1/portforward", &res); err != nil {
		return 0, err
	}
	return res.Port, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	body, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (c *Client) request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("authentication failed (401), check GLUETUN_API_KEY")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("gluetun returned status %d for %s: %s", resp.StatusCode, path, strings.TrimSpace(string(data)))
	}

	return io.ReadAll(resp.Body)
}
