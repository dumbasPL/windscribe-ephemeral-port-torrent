// Package deluge implements portforward.TorrentClient against the Deluge Web
// UI JSON-RPC API using golift.io/deluge for transport and authentication.
package deluge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"golift.io/deluge"
)

// Client manages Deluge's listening port over the Web UI.
type Client struct {
	d             *deluge.Deluge
	defaultHostID string
	currentHost   string
}

// New dials the Deluge Web UI at url using password and prepares for use.
// The Web UI is contacted eagerly so authentication failures surface early.
func New(ctx context.Context, url, password, defaultHostID string) (*Client, error) {
	d, err := deluge.New(ctx, &deluge.Config{
		URL:      url,
		Password: password,
		Client:   &http.Client{Timeout: 30 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("connect to deluge: %v", err)
	}
	return &Client{d: d, defaultHostID: defaultHostID}, nil
}

// GetPort returns Deluge's current configured listen port.
func (c *Client) GetPort() (int, error) {
	if err := c.ensureConnection(); err != nil {
		return 0, err
	}

	var cfg struct {
		RandomPort  deluge.Bool `json:"random_port"`
		ListenPorts []int       `json:"listen_ports"`
	}
	if err := c.call("core.get_config", []string{}, &cfg); err != nil {
		return 0, err
	}

	if cfg.RandomPort {
		return 0, nil
	}
	if len(cfg.ListenPorts) == 0 {
		return 0, errors.New("deluge report 0 listen ports")
	}
	return cfg.ListenPorts[0], nil
}

// SetPort configures Deluge to listen on port and disables random ports.
func (c *Client) SetPort(port int) error {
	if err := c.ensureConnection(); err != nil {
		return err
	}

	// result is a dict or True depending on Deluge version; success is verified
	// by the caller re-reading the port afterward.
	return c.call("core.set_config", []any{
		map[string]any{
			"listen_ports": []int{port, port},
			"random_port":  false,
		},
	}, &struct{}{})
}

// ensureConnection selects a host (reading the configured default if set) and
// connects to it, verifying it is reachable.
func (c *Client) ensureConnection() error {
	if c.currentHost != "" && c.connected() {
		return nil
	}

	hosts, err := c.getHosts()
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return errors.New("no deluge hosts available")
	}

	hostID := c.currentHost
	if hostID == "" {
		hostID = c.defaultHostID
	}

	if hostID == "" {
		if len(hosts) == 1 {
			hostID = hosts[0].id
			log.Printf("Selecting the only available deluge host: %s", hostID)
		} else {
			ids := make([]string, 0, len(hosts))
			for _, h := range hosts {
				log.Printf("\t%s: %s:%d - %s", h.id, h.addr, h.port, h.status)
				ids = append(ids, h.id)
			}
			sort.Strings(ids)
			return fmt.Errorf("found more than one deluge host (%v), select one via DELUGE_HOST_ID", ids)
		}
	} else if !containsHost(hosts, hostID) {
		return fmt.Errorf("deluge host with id %s does not exist", hostID)
	}

	// The user is already authenticated via the web UI, so no password is needed.
	var connected any
	if err := c.call("web.connect", []string{hostID}, &connected); err != nil {
		return err
	}
	c.currentHost = hostID

	status, err := c.getHostStatus(hostID)
	if err != nil {
		return err
	}
	if status.status != "Connected" && status.status != "Online" {
		return fmt.Errorf("not connected to deluge (host status: %s)", status.status)
	}
	return nil
}

func (c *Client) connected() bool {
	var ok bool
	if err := c.call("web.connected", []string{}, &ok); err != nil {
		return false
	}
	return ok
}

type host struct {
	id     string
	addr   string
	port   int
	status string
}

func (c *Client) getHosts() ([]host, error) {
	var raw [][]any
	if err := c.call("web.get_hosts", []string{}, &raw); err != nil {
		return nil, err
	}

	hosts := make([]host, 0, len(raw))
	for _, h := range raw {
		if len(h) < 4 {
			continue
		}
		id, _ := h[0].(string)
		addr, _ := h[1].(string)
		port, _ := h[2].(float64)
		status, _ := h[3].(string)
		hosts = append(hosts, host{id: id, addr: addr, port: int(port), status: status})
	}
	return hosts, nil
}

func containsHost(hosts []host, id string) bool {
	for _, h := range hosts {
		if h.id == id {
			return true
		}
	}
	return false
}

func (c *Client) getHostStatus(hostID string) (host, error) {
	var raw []any
	if err := c.call("web.get_host_status", []string{hostID}, &raw); err != nil {
		return host{}, err
	}
	if len(raw) < 3 {
		return host{}, errors.New("unexpected deluge get_host_status response")
	}
	id, _ := raw[0].(string)
	status, _ := raw[1].(string)
	return host{id: id, status: status}, nil
}

// call invokes an authenticated deluge RPC method and decodes the result.
func (c *Client) call(method string, params any, out any) error {
	resp, err := c.d.Get(context.Background(), method, params)
	if err != nil {
		return err
	}
	return json.Unmarshal(resp.Result, out)
}
