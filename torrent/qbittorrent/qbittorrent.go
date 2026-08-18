// Package qbittorrent implements portforward.TorrentClient against the
// qBittorrent Web API using Bearer API-key authentication (qBittorrent >= 5.2).
package qbittorrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client manages qBittorrent's listening port over the Web API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New connects to the qBittorrent Web API at baseURL using apiKey, verifying
// reachability and authentication up front.
func New(ctx context.Context, baseURL, apiKey string) (*Client, error) {
	c := &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}

	if _, err := c.version(ctx); err != nil {
		return nil, fmt.Errorf("connect to qbittorrent: %v", err)
	}
	return c, nil
}

// Name identifies the client in log messages.
func (c *Client) Name() string { return "qbittorrent" }

// GetPort returns qBittorrent's current configured listen port, or 0 when a
// random port is in use.
func (c *Client) GetPort() (int, error) {
	var preferences struct {
		ListenPort int  `json:"listen_port"`
		RandomPort bool `json:"random_port"`
	}
	if err := c.get("/api/v2/app/preferences", &preferences); err != nil {
		return 0, err
	}
	if preferences.RandomPort {
		return 0, nil
	}
	return preferences.ListenPort, nil
}

// SetPort configures qBittorrent to listen on port and disables random ports.
func (c *Client) SetPort(port int) error {
	if err := c.post("/api/v2/app/setPreferences", map[string]string{
		"json": fmt.Sprintf(`{"listen_port":%d,"random_port":false}`, port),
	}); err != nil {
		return err
	}
	return nil
}

// version fetches the qBittorrent version to validate connectivity and auth.
func (c *Client) version(ctx context.Context) (string, error) {
	body, err := c.request(ctx, http.MethodGet, "/api/v2/app/version", nil, "")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) get(path string, out any) error {
	body, err := c.request(context.Background(), http.MethodGet, path, nil, "")
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (c *Client) post(path string, form map[string]string) error {
	values := url.Values{}
	for k, v := range form {
		values.Set(k, v)
	}
	_, err := c.request(context.Background(), http.MethodPost, path, strings.NewReader(values.Encode()), "application/x-www-form-urlencoded")
	return err
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if resp.StatusCode == http.StatusForbidden {
			return nil, errors.New("authentication failed (403), check QBITTORRENT_API_KEY")
		}
		return nil, fmt.Errorf("qbittorrent returned status %d for %s: %s", resp.StatusCode, path, strings.TrimSpace(string(data)))
	}

	return io.ReadAll(resp.Body)
}
