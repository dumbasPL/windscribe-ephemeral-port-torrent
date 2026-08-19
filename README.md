# [windscribe-ephemeral-port-torrent](https://github.com/dumbasPL/windscribe-ephemeral-port-torrent)

Automatically create ephemeral ports in Windscribe and update your torrent client to use the new port

> [!CAUTION]
> v3 has not been well tested yet, use at your own risk. Feedback welcome.

# Features

- Automatically (re)create ephemeral port forwards in Windscribe
- Automatically update your torrent client to use the new port
- Support for multiple torrent clients:
  - [Deluge](https://deluge-torrent.org/)
  - [qBittorrent](https://www.qbittorrent.org/)
  - [Transmission](https://transmissionbt.com/)
  - Feel free to request or contribute more
- Automatically update port forwarding in [Gluetun](https://github.com/passteque/gluetun)

# Setup

## Getting authHash

### What? Why?

A while ago [Windscribe added Cloudflare CAPTCHA, and their own CAPTCHA](https://github.com/dumbasPL/windscribe-ephemeral-port-torrent/issues/17)
to the login endpoint, so we can no longer log in using the normal credentials reliably.
Normal web sessions expire, but the session in the Windscribe app doesn't.
Turns out that at some point, they decided to merge the APIs, and now the same token that's used in the app also works on the web.
This means that you only need to extract it once, and as long as you don't log out of the desktop app, it will keep working.

### How?

Download the official app and log in

There is a (vibe coded) Python script available [here](./getAuthHash.py) that will extract it for you (Tested on Linux/Windows)

For manual extraction, the value you're looking for is named `authHash` and is located inside of the `wsnetSettings` key in:
 - Linux: `$XDG_CONFIG_HOME/Windscribe/Windscribe2.conf`
 - Windows: `HKCU\Software\Windscribe\Windscribe2`
 - Mac: I have no idea, and neither does the slop generator it seems, feel free to contribute.

**DON'T LOG OUT! It will invalidate the session.**
If you want to log out without invalidating it, remove the configuration files listed above instead.

## Configuration

Configuration is done using environment variables

At least one torrent client or gluetun is required to be configured.

| Variable | Description | Required | Default |
| :-: | :-: | :-: | :-: |
| WINDSCRIBE_AUTH_HASH | authHash extracted from the desktop app | YES |  |
| DELUGE_URL | The base URL for the deluge web UI | NO |  |
| DELUGE_PASSWORD | The password for the deluge web UI | NO |  |
| DELUGE_HOST_ID | The internal host id to connect to in the deluge web UI. It will be printed in stdout after the first successful connection to deluge | Only if you have more than one connection configured in connection manager | If you have multiple configured in deluge web ui the app will print them out and crash. If you have only one, that one will be used, and you don't need to specify it explicitly |
| QBITTORRENT_URL | The base URL for the qBittorrent web UI | NO |  |
| QBITTORRENT_API_KEY | The API key for the qBittorrent (qBittorrent >= v5.2.0) | NO |  |
| TRANSMISSION_URL | The base URL for the transmission web UI (transmission >= 4.1) | NO |  |
| TRANSMISSION_USERNAME | The username for the transmission web UI | NO |  |
| TRANSMISSION_PASSWORD | The password for the transmission web UI | NO |  |
| GLUETUN_URL | The base URL for the gluetun Control server | NO |  |
| GLUETUN_API_KEY | The API key for the gluetun Control server (needs /v1/portforward permission) | NO |  |
| CRON_SCHEDULE | An extra cron schedule used to periodically validate and update the port if needed. Disabled if left empty | NO |  |
| WINDSCRIBE_RETRY_DELAY | how long to wait (in milliseconds) before retrying after a windscribe error. | NO | 3600000 (1 hour) |
| WINDSCRIBE_EXTRA_DELAY | how long to wait (in milliseconds) after the ephemeral port expires before trying to create a new one. | NO | 60000 (1 minute) |
| TORRENT_RETRY_DELAY | how long to wait (in milliseconds) before retrying after a torrent client error | NO | 300000 (5 minutes) |

## Running

### Using docker (and docker compose in this example)

```yaml
services:
  windscribe-ephemeral-port-torrent:
    image: dumbaspl/windscribe-ephemeral-port-torrent:latest
    restart: unless-stopped
    environment:
      WINDSCRIBE_AUTH_HASH: <authHash>
      # DELUGE_URL: http://deluge:8112
      # DELUGE_PASSWORD: <password>
      # QBITTORRENT_URL: http://qbittorrent:8080
      # QBITTORRENT_API_KEY: <apiKey>
      # TRANSMISSION_URL: http://transmission:9091
      # TRANSMISSION_USERNAME: <username>
      # TRANSMISSION_PASSWORD: <password>
      # GLUETUN_URL: http://gluetun:8000
      # GLUETUN_API_KEY: <apiKey>
```

### Native

make a `.env` file with the necessary configuration

```bash
go build -o windscribe-ephemeral-port-torrent
./windscribe-ephemeral-port-torrent
```

Make yourself a systemd service or something

# AI disclaimer

[Previous versions](https://github.com/dumbasPL/windscribe-ephemeral-port-torrent/tree/v2)
of this project were fully handwritten by me, this is an AI-assisted rewrite of the original 
codebase in GO with support for more torrent clients.

**All code has been reviewed and tested by me.**

# License

[MIT](./LICENSE)

# Contributing

With pet projects, I prefer to write things myself.
No guarantees that I won't abandon your PR for 2 years and then re-write it.
Debugging, testing, and feedback always welcome.