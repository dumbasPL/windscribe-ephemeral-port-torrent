// Package windscribe is a PortProvider that creates ephemeral port forwardings
// through the Windscribe API.
package windscribe

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"windscribe-ephemeral-port-torrent/portforward"
)

const (
	apiURL    = "https://api.windscribe.com/EphPortForwarding"
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"
)

// expiryExtension is added to the server-reported start time because the API
// returns the time the port was created rather than when it ends.
const expiryExtension = 7 * 86400

// Client talks to the Windscribe API using a long-lived authHash.
type Client struct {
	httpClient *http.Client
	authHash   string
	portCache  *portforward.Port
}

// New creates a Client authenticated with the given authHash.
func New(authHash string) *Client {
	return &Client{
		httpClient: &http.Client{},
		authHash:   authHash,
	}
}

// epf describes an ephemeral port forward. The API returns numbers when
// creating a port but strings when reading it back, so both are accepted.
type epf struct {
	Ext      flexInt `json:"ext"`
	Internal flexInt `json:"int"`
	StartTS  flexInt `json:"start_ts"`
}

// flexInt is an integer that may be encoded as a JSON number or string.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	b = bytes.Trim(b, `"`)
	v, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt(v)
	return nil
}

// UpdatePort ensures a forwarded port exists, creating one if needed.
func (c *Client) UpdatePort() (portforward.Port, error) {
	current, err := c.getEphemeralPort()
	if err != nil {
		return portforward.Port{}, err
	}

	if current != nil && current.Ext != current.Internal {
		log.Printf("Detected mismatched ports, removing existing ports")
		if err := c.removeEphemeralPort(int(current.Ext)); err != nil {
			return portforward.Port{}, err
		}
		current = nil
		c.portCache = nil
	}

	if current == nil {
		log.Printf("No windscribe port configured, requesting new matching ephemeral port")
		current, err = c.requestMatchingEphemeralPort()
		if err != nil {
			return portforward.Port{}, err
		}
	} else {
		log.Printf("Using existing windscribe ephemeral port: %d", current.Ext)
	}

	port := portforward.Port{
		Port:    int(current.Ext),
		Expires: time.Unix(int64(current.StartTS)+expiryExtension, 0),
	}
	c.portCache = &port
	return port, nil
}

// GetPort returns the last known forwarded port, or nil if none is cached.
func (c *Client) GetPort() (*portforward.Port, error) {
	return c.portCache, nil
}

// getEphemeralPort returns the current port forward, or nil if there is none.
func (c *Client) getEphemeralPort() (*epf, error) {
	var raw json.RawMessage
	if err := c.request(http.MethodGet, nil, &raw); err != nil {
		return nil, fmt.Errorf("failed to get port forwarding info: %v", err)
	}
	// "epf" is false when no port is configured.
	if string(raw) == "false" {
		return nil, nil
	}
	var e epf
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("failed to decode port forwarding info: %v", err)
	}
	return &e, nil
}

func (c *Client) removeEphemeralPort(port int) error {
	form := url.Values{"id": {strconv.Itoa(port)}}
	var removed bool
	if err := c.request(http.MethodDelete, form, &removed); err != nil {
		return fmt.Errorf("failed to remove ephemeral port: %v", err)
	}

	if !removed {
		log.Printf("Tried to remove a non-existent ephemeral port, ignoring")
	} else {
		log.Printf("Deleted ephemeral port")
	}
	return nil
}

func (c *Client) requestMatchingEphemeralPort() (*epf, error) {
	var e *epf
	if err := c.request(http.MethodPost, nil, &e); err != nil {
		return nil, fmt.Errorf("failed to request ephemeral port: %v", err)
	}
	if e == nil {
		return nil, errors.New("no ephemeral port present after request")
	}

	log.Printf("Created new matching ephemeral port: %d", e.Ext)
	return e, nil
}

// request performs an HTTP request against the EphPortForwarding endpoint,
// sending form as a url-encoded body when non-nil, and decodes the "epf"
// member of the response into out.
func (c *Client) request(method string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, apiURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.authHash)
	req.Header.Set("User-Agent", userAgent)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var res struct {
		Data *struct {
			Success int             `json:"success"`
			Epf     json.RawMessage `json:"epf"`
		} `json:"data"`
		ErrorCode    int    `json:"errorCode"`
		ErrorMessage string `json:"errorMessage"`
	}
	if err := json.Unmarshal(respBody, &res); err != nil {
		return fmt.Errorf("%s %s: status %d, decoding response: %v", method, apiURL, resp.StatusCode, err)
	}
	if res.ErrorCode != 0 || res.Data == nil {
		return fmt.Errorf("%s %s: status %d, error %d: %s", method, apiURL, resp.StatusCode, res.ErrorCode, res.ErrorMessage)
	}
	if res.Data.Success == 0 {
		return fmt.Errorf("%s %s: success = 0", method, apiURL)
	}

	if err := json.Unmarshal(res.Data.Epf, out); err != nil {
		return fmt.Errorf("%s %s: decoding epf: %v", method, apiURL, err)
	}
	return nil
}
