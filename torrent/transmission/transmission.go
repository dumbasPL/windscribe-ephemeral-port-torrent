// Package transmission implements portforward.TorrentClient against the
// Transmission JSON-RPC 2.0 API (Transmission >= 4.1).
package transmission

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const sessionIdHeader = "X-Transmission-Session-Id"

const rpcPath = "/transmission/rpc"

// Client manages Transmission's listening port over the RPC API.
type Client struct {
	baseURL   string
	username  string
	password  string
	http      *http.Client
	sessionID string
	mu        sync.Mutex
	nextID    int64
}

// New connects to the Transmission RPC API at url, verifying reachability,
// authentication, and the CSRF session up front.
func New(ctx context.Context, url, username, password string) (*Client, error) {
	c := &Client{
		baseURL:  rpcURL(url),
		username: username,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}

	if err := c.sessionGet(ctx); err != nil {
		return nil, fmt.Errorf("connect to transmission: %v", err)
	}
	return c, nil
}

// rpcURL resolves the RPC endpoint from a base or full URL.
func rpcURL(url string) string {
	url = strings.TrimSuffix(url, "/")
	if strings.HasSuffix(url, rpcPath) {
		return url
	}
	return url + rpcPath
}

// Name identifies the client in log messages.
func (c *Client) Name() string { return "transmission" }

// GetPort returns Transmission's current configured listen port, or 0 when a
// random port is in use.
func (c *Client) GetPort() (int, error) {
	var resp struct {
		Result struct {
			PeerPort         int  `json:"peer_port"`
			PeerPortRandomOn bool `json:"peer_port_random_on_start"`
		} `json:"result"`
	}
	if err := c.call("session_get", map[string]any{
		"fields": []string{"peer_port", "peer_port_random_on_start"},
	}, &resp); err != nil {
		return 0, err
	}
	if resp.Result.PeerPortRandomOn {
		return 0, nil
	}
	return resp.Result.PeerPort, nil
}

// SetPort configures Transmission to listen on port and disables random ports.
func (c *Client) SetPort(port int) error {
	return c.call("session_set", map[string]any{
		"peer_port":                 port,
		"peer_port_random_on_start": false,
	}, &struct{}{})
}

// sessionGet performs a session_get, used both to validate connectivity and
// to fetch the peer port.
func (c *Client) sessionGet(ctx context.Context) error {
	var out struct{}
	return c.callContext(ctx, "session_get", map[string]any{"fields": []string{"peer_port"}}, &out)
}

// call issues a JSON-RPC 2.0 request and decodes the full body into out.
func (c *Client) call(method string, params any, out any) error {
	return c.callContext(context.Background(), method, params, out)
}

func (c *Client) callContext(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      id,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := c.post(ctx, body)
	if err != nil {
		return err
	}

	var envelope struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &envelope); err != nil {
		return fmt.Errorf("decoding response: %v", err)
	}
	if envelope.Error.Code != 0 {
		return errors.New(envelope.Error.Message)
	}
	return json.Unmarshal(resp, out)
}

// post sends a request, handling the CSRF session-id challenge and Basic auth.
func (c *Client) post(ctx context.Context, body []byte) ([]byte, error) {
	for range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, strings.NewReader(string(body)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		c.mu.Lock()
		sessionID := c.sessionID
		c.mu.Unlock()
		if sessionID != "" {
			req.Header.Set(sessionIdHeader, sessionID)
		}
		if c.username != "" || c.password != "" {
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.username+":"+c.password)))
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		if resp.StatusCode == http.StatusConflict {
			c.mu.Lock()
			c.sessionID = resp.Header.Get(sessionIdHeader)
			c.mu.Unlock()
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("authentication failed (401), check TRANSMISSION_USERNAME/TRANSMISSION_PASSWORD")
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("transmission returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
		}
		return data, nil
	}

	return nil, errors.New("transmission did not return a session id")
}
