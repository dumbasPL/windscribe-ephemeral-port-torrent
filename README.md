# [deluge-windscribe-ephemeral-port](https://github.com/dumbasPL/deluge-windscribe-ephemeral-port)

Automatically create ephemeral ports in windscribe and update deluge config to use the new port

# Getting authHash

## What? Why?

A while ago [windscribe added cloudflare captcha, and their own captcha](https://github.com/dumbasPL/deluge-windscribe-ephemeral-port/issues/17)
to the login endpoint, so we can no longer log in using the normal credentials reliably.
Normal web sessions expire, but the session in the windscribe app doesn't.
Turns out that at some point, they decided to merge the APIs, and now the same token that's used in the app also works on the web.
This means that you only need to extract it once, and as long as you don't log out of the desktop app, it will keep working.

## How?

Download the official app and log in

There is a (vibe coded) python script available [here](./getAuthHash.py) that will extract it for you (Tested on Linux/Windows)

For manual extraction, the value you're looking for is named `authHash` and is located inside of the `wsnetSettings` key in:
 - Linux: `$XDG_CONFIG_HOME/Windscribe/Windscribe2.conf`
 - Windows: `HKCU\Software\Windscribe\Windscribe2`
 - Mac: I have no idea, and neither does the slop generator it seems, feel free to contribute.

DON'T LOG OUT! It will invalidate the session.
If you want to log out without invalidating it, remove the configuration files listed above instead.

# Configuration

Configuration is done using environment variables

| Variable | Description | Required | Default |
| :-: | :-: | :-: | :-: |
| WINDSCRIBE_AUTH_HASH | authHash extracted from the desktop app | YES |  |
| DELUGE_URL | The base URL for the deluge web UI | YES |  |
| DELUGE_PASSWORD | The password for the deluge web UI | YES |  |
| CRON_SCHEDULE | An extra cron schedule used to periodically validate and update the port if needed. Disabled if left empty | NO |  |
| DELUGE_HOST_ID | The internal host id to connect to in the deluge web UI. It will be printed in stdout after the first successful connection to deluge | Only if you have more then one connection configured in connection manager | If you have multiple configured in deluge web ui the app will print them out and crash. If you have only one that one will be used and you don't need to specify it explicitly |
| WINDSCRIBE_RETRY_DELAY | how long to wait (in milliseconds) before retrying after a windscribe error. For example a failed login. | NO | 3600000 (1 hour) |
| WINDSCRIBE_EXTRA_DELAY | how long to wait (in milliseconds) after the ephemeral port expires before trying to create a new one. | NO | 60000 (1 minute) |
| DELUGE_RETRY_DELAY | how long to wait (in milliseconds) before retrying after a deluge error. For example a failed login. | NO | 300000 (5 minutes) |

# Running

## Using docker (and docker compose in this example)

```yaml
services:
  deluge-windscribe-ephemeral-port:
    image: dumbaspl/deluge-windscribe-ephemeral-port:3
    restart: unless-stopped
    environment:
      - WINDSCRIBE_USERNAME=<your windscribe username>
      - WINDSCRIBE_PASSWORD=<your windscribe password>
      - DELUGE_URL=<url of your Deluge Web UI>
      - DELUGE_PASSWORD=<password for the Deluge Web UI>

      # optional
      # - DELUGE_HOST_ID=
      # - DELUGE_RETRY_DELAY=300000
      # - WINDSCRIBE_RETRY_DELAY=3600000
      # - WINDSCRIBE_EXTRA_DELAY=60000
      # - CRON_SCHEDULE=
```

## Using nodejs

**This project requires Node.js version 22 or newer**

1. clone this repository
2. Install dependencies by running `npm ci`
3. Create a `.env` file in the root of the project with the necessary configuration
```shell
WINDSCRIBE_USERNAME=<your windscribe username>
WINDSCRIBE_PASSWORD=<your windscribe password>
DELUGE_URL=<url of your Deluge Web UI>
DELUGE_PASSWORD=<password for the Deluge Web UI>

# optional
# DELUGE_HOST_ID=
# DELUGE_RETRY_DELAY=300000
# WINDSCRIBE_RETRY_DELAY=3600000
# WINDSCRIBE_EXTRA_DELAY=60000
# CRON_SCHEDULE=
```
4. Build and start using `npm start`

Tip: you can use tools like pm2 to manage nodejs applications
