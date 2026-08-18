// Command windscribe-ephemeral-port-torrent keeps a forwarded Windscribe port
// configured as a torrent client's listen port, renewing it on expiration.
package main

import (
	"context"
	"log"
	"time"

	"windscribe-ephemeral-port-torrent/config"
	"windscribe-ephemeral-port-torrent/portforward"
	"windscribe-ephemeral-port-torrent/torrent/deluge"
	"windscribe-ephemeral-port-torrent/windscribe"
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	provider := windscribe.New(cfg.WindscribeAuthHash)

	torrent, err := deluge.New(context.Background(), cfg.DelugeURL, cfg.DelugePassword, cfg.DelugeHostID)
	if err != nil {
		log.Fatal(err)
	}

	trigger := make(chan string, 1)
	go func() {
		trigger <- "initial"
		for name := range trigger {
			nextRun, nextRetry := run(cfg, provider, torrent, name)
			var at time.Time
			var next string
			switch {
			case !nextRetry.IsZero():
				at, next = nextRetry, "retry"
			case !nextRun.IsZero():
				at, next = nextRun, "normal"
			default:
				log.Fatal("invalid state, no next retry/run date present")
			}

			delay := max(time.Until(at), 0)
			log.Printf("Next %s scheduled for %s (in %s)", next, at.Format(time.RFC3339), delay.Round(time.Second))
			time.AfterFunc(delay, func() { trigger <- next })
		}
	}()

	select {}
}

// run performs a single update pass and reports when the following run or
// retry should happen. A retry always takes priority over a regular run.
func run(cfg *config.Config, provider portforward.PortProvider, torrent portforward.TorrentClient, trigger string) (time.Time, time.Time) {
	log.Printf("starting update, trigger type: %s", trigger)

	var nextRun, nextRetry time.Time
	var portInfo *portforward.Port

	port, err := provider.UpdatePort()
	if err != nil {
		log.Printf("Windscribe update failed: %v", err)
		nextRetry = time.Now().Add(cfg.WindscribeRetryDelay)
		if cached, e := provider.GetPort(); e == nil {
			portInfo = cached
		}
	} else {
		portInfo = &port
		nextRun = port.Expires.Add(cfg.WindscribeExtraDelay)
	}

	currentPort, err := torrent.GetPort()
	if err != nil {
		log.Printf("Deluge update failed: %v", err)
		return nextRun, time.Now().Add(cfg.DelugeRetryDelay)
	}

	if portInfo != nil {
		if currentPort == portInfo.Port {
			log.Printf("Current deluge port (%d) already matches windscribe port", currentPort)
			return nextRun, nextRetry
		}

		log.Printf("Current deluge port (%d) does not match windscribe port (%d)", currentPort, portInfo.Port)
		if err := torrent.SetPort(portInfo.Port); err != nil {
			log.Printf("Deluge update failed: %v", err)
			return nextRun, time.Now().Add(cfg.DelugeRetryDelay)
		}

		if currentPort, err = torrent.GetPort(); err != nil || currentPort != portInfo.Port {
			log.Printf("Unable to set deluge port! Current deluge port: %d", currentPort)
			return nextRun, time.Now().Add(cfg.DelugeRetryDelay)
		}
		log.Printf("Deluge port updated")
	} else {
		log.Printf("Windscribe port is unknown, current deluge port is %d", currentPort)
	}

	return nextRun, nextRetry
}
