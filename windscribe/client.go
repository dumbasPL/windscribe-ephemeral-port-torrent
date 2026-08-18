// Package windscribe is a PortProvider that creates ephemeral port forwardings
// through the Windscribe account web API.
package windscribe

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"windscribe-ephemeral-port-torrent/portforward"
)

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"

// expiryExtension is added to the server-reported expiry because the API
// returns the time the port was created rather than when it ends.
const expiryExtension = 7 * 86400

var (
	csrfTimeRe   = regexp.MustCompile(`csrf_time = (\d+);`)
	csrfTokenRe  = regexp.MustCompile(`csrf_token = '(\w+)';`)
	epfExpiresRe = regexp.MustCompile(`epfExpires = (\d+);`)
	pfExtRe      = regexp.MustCompile(`<span class="pf-ext">(\d+)</span>`)
	pfIntRe      = regexp.MustCompile(`<span class="pf-int">(\d+)</span>`)
)

// Client talks to the Windscribe account website using a long-lived authHash.
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

// csrfInfo carries the CSRF token and timestamp required by mutating requests.
type csrfInfo struct {
	time  int64
	token string
}

// portForwardingInfo describes the current state of Windscribe's port forwarding.
type portForwardingInfo struct {
	epfExpires int64
	ports      []int
}

// UpdatePort ensures a forwarded port exists, creating one if needed.
func (c *Client) UpdatePort() (portforward.Port, error) {
	csrf, err := c.getMyAccountCsrfToken()
	if err != nil {
		return portforward.Port{}, err
	}

	info, err := c.getPortForwardingInfo()
	if err != nil {
		return portforward.Port{}, err
	}

	if len(info.ports) == 2 && info.ports[0] != info.ports[1] {
		log.Printf("Detected mismatched ports, removing existing ports")
		if err := c.removeEphemeralPort(csrf); err != nil {
			return portforward.Port{}, err
		}
		info = portForwardingInfo{}
		c.portCache = nil
	}

	if info.epfExpires == 0 {
		log.Printf("No windscribe port configured, requesting new matching ephemeral port")
		info, err = c.requestMatchingEphemeralPort(csrf)
		if err != nil {
			return portforward.Port{}, err
		}
	} else {
		log.Printf("Using existing windscribe ephemeral port: %d", info.ports[0])
	}

	port := portforward.Port{
		Port:    info.ports[0],
		Expires: time.Unix(info.epfExpires+expiryExtension, 0),
	}
	c.portCache = &port
	return port, nil
}

// GetPort returns the last known forwarded port, or nil if none is cached.
func (c *Client) GetPort() (*portforward.Port, error) {
	return c.portCache, nil
}

func (c *Client) getMyAccountCsrfToken() (csrfInfo, error) {
	body, err := c.getText("/myaccount")
	if err != nil {
		return csrfInfo{}, fmt.Errorf("failed to get csrf token from my account page: %v", err)
	}

	t := csrfTimeRe.FindStringSubmatch(body)
	tok := csrfTokenRe.FindStringSubmatch(body)
	if t == nil || tok == nil {
		return csrfInfo{}, errors.New("failed to extract csrf token and time from my account page")
	}

	ts, err := strconv.ParseInt(t[1], 10, 64)
	if err != nil {
		return csrfInfo{}, fmt.Errorf("failed to parse csrf time: %v", err)
	}
	return csrfInfo{time: ts, token: tok[1]}, nil
}

func (c *Client) getPortForwardingInfo() (portForwardingInfo, error) {
	body, err := c.getText("/staticips/load")
	if err != nil {
		return portForwardingInfo{}, fmt.Errorf("failed to get port forwarding info: %v", err)
	}

	exp := epfExpiresRe.FindStringSubmatch(body)
	if exp == nil {
		return portForwardingInfo{}, errors.New("failed to extract epfExpires from static IPs page")
	}

	expires, err := strconv.ParseInt(exp[1], 10, 64)
	if err != nil {
		return portForwardingInfo{}, fmt.Errorf("failed to parse epfExpires: %v", err)
	}

	ext := pfExtRe.FindStringSubmatch(body)
	internal := pfIntRe.FindStringSubmatch(body)
	var ports []int
	if ext != nil && internal != nil {
		e, _ := strconv.Atoi(ext[1])
		in, _ := strconv.Atoi(internal[1])
		ports = []int{e, in}
	}

	return portForwardingInfo{epfExpires: expires, ports: ports}, nil
}

func (c *Client) removeEphemeralPort(csrf csrfInfo) error {
	var res struct {
		Success int    `json:"success"`
		Epf     bool   `json:"epf"`
		Message string `json:"message"`
	}
	if err := c.postForm("/staticips/deleteEphPort", csrfForm(csrf), &res); err != nil {
		return err
	}

	if res.Success == 0 {
		return fmt.Errorf("success = 0; %s", fallbackMsg(res.Message))
	}

	if !res.Epf {
		log.Printf("Tried to remove a non-existent ephemeral port, ignoring")
	} else {
		log.Printf("Deleted ephemeral port")
	}
	return nil
}

func (c *Client) requestMatchingEphemeralPort(csrf csrfInfo) (portForwardingInfo, error) {
	var res struct {
		Success int    `json:"success"`
		Message string `json:"message"`
		Epf     *struct {
			Ext      int   `json:"ext"`
			Internal int   `json:"int"`
			StartTS  int64 `json:"start_ts"`
		} `json:"epf"`
	}
	if err := c.postForm("/staticips/postEphPort", csrfForm(csrf), &res); err != nil {
		return portForwardingInfo{}, err
	}

	if res.Success == 0 {
		return portForwardingInfo{}, fmt.Errorf("success = 0; %s", fallbackMsg(res.Message))
	}
	if res.Epf == nil {
		return portForwardingInfo{}, errors.New("no ephemeral port present after request")
	}

	log.Printf("Created new matching ephemeral port: %d", res.Epf.Ext)
	return portForwardingInfo{
		epfExpires: res.Epf.StartTS,
		ports:      []int{res.Epf.Ext, res.Epf.Internal},
	}, nil
}

// csrfForm builds the form fields required by mutating endpoints.
func csrfForm(csrf csrfInfo) map[string]string {
	return map[string]string{
		"ctime":  strconv.FormatInt(csrf.time, 10),
		"ctoken": csrf.token,
	}
}

func fallbackMsg(msg string) string {
	if msg == "" {
		return "No message"
	}
	return msg
}

// request performs an HTTP request against the Windscribe site with the
// session cookie and user agent set, parsing the response as JSON when
// possible or as plain text otherwise.
func (c *Client) request(method, path string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequest(method, "https://windscribe.com"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", "ws_session_auth_hash="+c.authHash+";")
	req.Header.Set("User-Agent", userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("windscribe returned status %d for %s", resp.StatusCode, path)
	}

	return io.ReadAll(resp.Body)
}

// getText fetches a page and returns its body.
func (c *Client) getText(path string) (string, error) {
	data, err := c.request(http.MethodGet, path, nil, "")
	if err != nil {
		return "", fmt.Errorf("GET %s: %v", path, err)
	}
	return string(data), nil
}

// postForm posts form-encoded values and decodes the (JSON) response into out.
func (c *Client) postForm(path string, values map[string]string, out any) error {
	form := url.Values{}
	for k, v := range values {
		form.Set(k, v)
	}

	data, err := c.request(http.MethodPost, path, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return fmt.Errorf("POST %s: %v", path, err)
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding response from %s: %v", path, err)
	}
	return nil
}
