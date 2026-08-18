// Package config loads settings from the environment and an optional .env file.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the runtime settings for the application.
type Config struct {
	TorrentRetryDelay    time.Duration
	WindscribeAuthHash   string
	WindscribeRetryDelay time.Duration
	WindscribeExtraDelay time.Duration

	DelugeURL      string
	DelugePassword string
	DelugeHostID   string

	QBittorrentURL    string
	QBittorrentAPIKey string
}

// HasDeluge reports whether Deluge is configured.
func (c *Config) HasDeluge() bool { return c.DelugeURL != "" }

// HasQBittorrent reports whether qBittorrent is configured.
func (c *Config) HasQBittorrent() bool { return c.QBittorrentURL != "" }

// Load reads any present .env file (existing environment variables win) and
// builds a Config, failing on missing required variables or invalid numbers.
func Load(path string) (*Config, error) {
	if err := loadDotEnv(path); err != nil {
		return nil, err
	}

	cfg := Config{
		WindscribeAuthHash: os.Getenv("WINDSCRIBE_AUTH_HASH"),
		DelugeURL:          os.Getenv("DELUGE_URL"),
		DelugePassword:     os.Getenv("DELUGE_PASSWORD"),
		DelugeHostID:       os.Getenv("DELUGE_HOST_ID"),
		QBittorrentURL:     os.Getenv("QBITTORRENT_URL"),
		QBittorrentAPIKey:  os.Getenv("QBITTORRENT_API_KEY"),
	}

	if cfg.WindscribeAuthHash == "" {
		return nil, errors.New("missing environment variable WINDSCRIBE_AUTH_HASH")
	}
	if !cfg.HasDeluge() && !cfg.HasQBittorrent() {
		return nil, errors.New("at least one of DELUGE_URL or QBITTORRENT_URL must be set")
	}
	if cfg.HasDeluge() && cfg.DelugePassword == "" {
		return nil, errors.New("DELUGE_PASSWORD is required when DELUGE_URL is set")
	}
	if cfg.HasQBittorrent() && cfg.QBittorrentAPIKey == "" {
		return nil, errors.New("QBITTORRENT_API_KEY is required when QBITTORRENT_URL is set")
	}

	var err error
	if cfg.TorrentRetryDelay, err = envDurationMs("TORRENT_RETRY_DELAY", 5*60*1000); err != nil {
		return nil, err
	}
	if cfg.WindscribeRetryDelay, err = envDurationMs("WINDSCRIBE_RETRY_DELAY", 60*60*1000); err != nil {
		return nil, err
	}
	if cfg.WindscribeExtraDelay, err = envDurationMs("WINDSCRIBE_EXTRA_DELAY", 60*1000); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func envDurationMs(name string, def int64) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return time.Duration(def) * time.Millisecond, nil
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s must be an integer number of milliseconds: %v", name, err)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// loadDotEnv applies KEY=VALUE lines from path as environment variables
// without overriding values that are already set. Missing files are ignored.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open %s: %v", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}
