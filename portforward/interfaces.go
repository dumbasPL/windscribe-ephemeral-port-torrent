// Package portforward defines the contracts between a port provider and the
// torrent client that consumes the forwarded port.
package portforward

import "time"

// Port is a forwarded network port and the moment it stops being valid.
type Port struct {
	Port    int
	Expires time.Time
}

// PortProvider manages a port that is forwarded by an external service, such
// as a VPN client or a port-forwarding API.
type PortProvider interface {
	// UpdatePort ensures a forwarded port exists, creating or renewing it,
	// and returns the resulting port together with its expiry.
	UpdatePort() (Port, error)
	// GetPort returns the last known forwarded port, or nil if none is
	// cached (for example after a failed renewal).
	GetPort() (*Port, error)
}

// TorrentClient is an application whose inbound listening port can be read
// and changed.
type TorrentClient interface {
	// GetPort returns the torrent client's current configured listen port.
	GetPort() (int, error)
	// SetPort changes the torrent client's listen port to the given value.
	SetPort(port int) error
}
