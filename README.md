# Odio API

> The universal remote for your Linux multimedia server

[![CI](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml/badge.svg)](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml)
[![Build](https://github.com/b0bbywan/go-odio-api/actions/workflows/build.yml/badge.svg)](https://github.com/b0bbywan/go-odio-api/actions/workflows/build.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/b0bbywan/go-odio-api)](https://goreportcard.com/report/github.com/b0bbywan/go-odio-api)

Odio is an ultra-lightweight Go daemon that exposes a single clean REST API over your Linux user session's D-Bus: MPRIS players (Spotify, VLC, Firefox, MPD, Kodi), PulseAudio/PipeWire, systemd user services, and power management. No root. No hacks. Just Linux primitives.

Building a Linux multimedia setup is easy. Integrating it cleanly into Home Assistant always felt hacky, scattered integrations, SSH scripts, and fragile glue.


Tested on Fedora 43 Gnome, Debian 13 KDE, Raspbian 13, Openmediavault 8
Raspberry Pi B through Pi 5.
Works without any system tweak.

## Quick Start

Start Spotify, VLC, or any MPRIS player first, then:

```bash
# 1. Config (bind: all required for Docker)
cp share/config.yaml config.yaml

# 2. Start
docker compose up -d

# 3. Test
curl http://localhost:8018/players
curl http://localhost:8018/audio/server
```

→ See [Installation](#installation) for systemd service, packages, or running from source.

## User Interface

<img width="1658" height="963" alt="Capture d’écran du 2026-02-17 00-32-56" src="https://github.com/user-attachments/assets/0a9697da-902b-41d0-8977-908ef66f1168" />

The built-in Odio UI is accessible at:

**http://localhost:8018/ui**
(or http://your-host.local:8018/ui if zeroconf/mDNS is enabled)

It's a **100% local**, **responsive** (mobile + desktop), web interface designed to control your entire Linux multimedia setup from one place: MPRIS players, per-app/global volume, systemd user services, PipeWire/PulseAudio server, and more.

There's also an **[installable PWA](https://odio-pwa.vercel.app/)** to install on your phone/desktop to easily access your remote and navigate between several instances.

[More info](UI.md)

## Home Assistant Integration

**[odio-ha](https://github.com/b0bbywan/odio-ha)** is the official Home Assistant integration for Odio.

Install via HACS → Custom Repositories → `https://github.com/b0bbywan/odio-ha`

What it exposes as HA entities:
- `media_player` — global PulseAudio/PipeWire audio receiver (volume, mute)
- `media_player` per systemd service — power on/off, volume, state tracking (MPD, Kodi, shairport-sync, etc.)
- MPRIS players — auto-discovered players with full playback control and metadata *(in progress)*

Odio becomes the hub that makes all your HA integrations point to the correct machine. MPD service lifecycle managed by Odio, rich playback via HA's existing MPD integration — the two work together.

## Use Cases

| Setup | What Odio gives you |
|---|---|
| RPi music server (MPD + shairport-sync) | MPRIS control + restart services from HA |
| HTPC / Kodi | Start/stop Kodi, MPRIS control via odio-ha |
| Firefox kiosk (Netflix, YouTube) | Start/stop fake Netflix and Youtube app, MPRIS control via odio-ha |
| Headless Spotify (spotifyd) | MPRIS playback + service lifecycle |
| Any PulseAudio/PipeWire setup | Per-client and global volume/mute control |

## Features

### Media Player Control (MPRIS)

Auto-discovers all MPRIS-compatible players in real time — Spotify, VLC, Firefox, MPD, Kodi, etc. Add a new player and it appears immediately, zero config.

- Full playback control: play, pause, stop, next, previous
- Volume, seek, and position control
- Shuffle and loop mode management
- Real-time state updates via D-Bus signals
- Smart caching with automatic cache invalidation
- Position heartbeat for accurate playback tracking

### Audio Management (PulseAudio/PipeWire)

- Server info and default output
- Global and per-client volume/mute control
- Real-time audio events via native PulseAudio monitoring
- Limited PipeWire support via `pipewire-pulse`

### Service Management (systemd)

Explicit whitelist required — nothing managed unless listed in `config.yaml`.

- List and monitor whitelisted systemd services (system + user)
- Start, stop, restart, enable, disable user services
- Real-time service state updates via D-Bus signals
- Disabled by default

⚠️ **Security model:** Odio enforces user-session mutations only at the application layer, regardless of D-Bus or polkit configuration. System units are strictly read-only. See [Security](#security) for full details.

### Power Management

Remote reboot and power-off via the REST API — no SSH needed for day-to-day operations. Disabled by default. Uses `org.freedesktop.login1` D-Bus interface.

### Real-time Event Stream (SSE)

`GET /events` streams live state changes to any HTTP client — no polling needed.

Events emitted:

| Event type | Triggered by |
|---|---|
| `player.updated` | Playback state change, volume, metadata, position tick |
| `player.added` | New MPRIS player appeared |
| `player.removed` | MPRIS player closed |
| `audio.updated` | PulseAudio sink-input change (volume, mute, cork) |
| `service.updated` | systemd unit state change |

### REST API

- `<50ms` p95 response time, `0%` CPU on idle — tested on Raspberry Pi B and B+
- Localhost binding by default, configurable per network interface
- Zeroconf/mDNS auto-discovery on the LAN (opt-in)

## Platform Support

| Architecture | Package | Tested on |
|---|---|---|
| amd64 | deb, rpm | Fedora 43 Gnome, Debian 13 KDE |
| arm64 | deb, rpm | Raspberry Pi 3/4/5 (64-bit) |
| armv7hf | deb, rpm | Raspberry Pi 2/3 (32-bit) |
| **armhf (ARMv6)** | deb, rpm | **Raspberry Pi B / B+ / Zero** |

Pre-built packages (amd64, arm64, armv7hf, armhf/ARMv6) and a multi-arch Docker image (amd64, arm64, arm/v7) are available on every build. Docker does not target arm/v6 — Pi B/Zero users should use the armhf package.

## Roadmap

- Bluetooth backend: turn your Linux box into a fully API-controllable BT speaker, exposed as a `media_player` in HA
- SSE push events
- Wayland Remote Control, Authentication, Photos Casting...

## Installation

### Packages (deb / rpm)

Pre-built packages for amd64, arm64, armv7hf, and armhf (ARMv6) are available as artifacts on each [build workflow run](https://github.com/b0bbywan/go-odio-api/actions/workflows/build.yml).

```bash
# Debian/Ubuntu/Raspberry Pi OS
sudo dpkg -i odio-api_<version>_amd64.deb

# Fedora/RHEL
sudo rpm -i odio-api-<version>.x86_64.rpm
```

### From Source

```bash
git clone https://github.com/b0bbywan/go-odio-api.git
cd go-odio-api
task build    # builds CSS + Go binary with version from git
./bin/odio-api
```

### systemd User Service

Create `~/.config/systemd/user/odio-api.service`:

```ini
[Unit]
Description=Dbus api for Odio
Documentation=https://github.com/b0bbywan/go-odio-api
Wants=sound.target
After=sound.target
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/usr/bin/odio-api
Restart=always
RestartSec=12
TimeoutSec=30

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable odio-api.service
systemctl --user start odio-api.service
```

**Headless systems:** Enable lingering so the user session (PulseAudio/PipeWire, D-Bus, `XDG_RUNTIME_DIR`) survives without an active login:

```bash
sudo loginctl enable-linger <username>
```

### Docker

A pre-built multi-arch image is available on GHCR (amd64, arm64, arm/v6, arm/v7):

```
ghcr.io/b0bbywan/go-odio-api:latest
```

#### Quick start

```bash
# 1. Prepare configuration (bind: all required for Docker)
cp share/config.yaml config.yaml
# Edit config.yaml: set bind: all

# 3. (Optional) Only needed if docker compose config shows wrong paths
cp .env.example .env

# 4. Start
docker compose up -d
```

The `docker-compose.yml` reads `UID`, `XDG_RUNTIME_DIR`, `HOME` and `DBUS_SESSION_BUS_ADDRESS`
directly from your shell environment — no configuration needed for a standard Linux setup.
See `.env.example` if your shell doesn't export these automatically (e.g. fish).

Environment variables passed to the container:

| Variable | Source | Purpose |
|---|---|---|
| `XDG_RUNTIME_DIR` | host env → fallback `/run/user/$UID` | D-Bus and PulseAudio runtime directory |
| `DBUS_SESSION_BUS_ADDRESS` | host env → fallback derived from `XDG_RUNTIME_DIR` | User D-Bus session socket |
| `HOME` | host env → fallback `/home/odio` | PulseAudio cookie lookup path |

Volumes mounted (all read-only):

| Volume | Purpose |
|---|---|
| `./config.yaml` | odio configuration |
| `$XDG_RUNTIME_DIR/bus` | user D-Bus session socket |
| `$XDG_RUNTIME_DIR/systemd` | user systemd folder (utmp unavailable) |
| `/run/utmp` | user systemd monitoring (utmp available) |
| `/var/run/dbus/system_bus_socket` | system D-Bus socket |
| `$XDG_RUNTIME_DIR/pulse` | PulseAudio socket |
| `$HOME/.config/pulse/cookie` | PulseAudio cookie |

**Note:** `bind` must be set to `all` in `config.yaml` for Docker remote access (bridge network). Zeroconf won't work in bridge network mode. Host network mode is strongly discouraged.

To build locally instead:
```bash
docker build -t odio-api .
# or simply: task docker:build
```
The Docker build is fully self-contained — Tailwind CSS is downloaded and compiled inside the builder stage.

#### Command-line Flags

- `--config <path>` — specify a custom YAML configuration file
- `--version` — print version and exit
- `--help` — show help message

## Configuration

Configuration file locations (in order of precedence):
- Specified with `--config <path>`
- `~/.config/odio-api/config.yaml` (user-specific)
- `/etc/odio-api/config.yaml` (system-wide)
- A default configuration is available in `share/config.yaml`

Disabling a backend disables the backend and all its routes.

```yaml
bind: lo
logLevel: info

api:
  enabled: true
  port: 8018
  ui:
    enabled: true
  cors:
    origins: ["https://odio-pwa.vercel.app"] # default for PWA
    # origins: ["https://app.example.com"]  # specific origins
```

### Backend configuration examples

#### systemd (opt-in, whitelist required)

```yaml
systemd:
  enabled: true
  system:
    - bluetooth.service
    - upmpdcli.service
  user:
    - pipewire-pulse.service
    - pulseaudio.service
    - mpd.service                       # see [1]
    - shairport-sync.service            # see [2]
    - snapclient.service                # incompatible with mpris
    - spotifyd.service                  # see [3]
    - firefox-kiosk@netflix.com.service # default support for mpris
    - firefox-kiosk@youtube.com.service # default support for mpris
    - firefox-kiosk@my.home-assistant.io.service
    - kodi.service                      # see [4]
    - vlc.service                       # default support for mpris
    - plex.service                      # see [5]
```
[1] Install `mpd-mpris` or `mpDris2` for MPRIS support
[2] Check my [article on Medium: Shairport Sync/Airplay with PulseAudio and MPRIS support](https://medium.com/@mathieu-requillart/set-up-a-b83d9c980e75)
[3] Default on desktop; on headless, your spotifyd version [must be built with MPRIS support](https://docs.spotifyd.rs/advanced/dbus.html)
[4] Install [Kodi Add-on: MPRIS D-Bus interface](https://github.com/wastis/MediaPlayerRemoteInterface#)
[5] Maybe supported, untested

#### Power Management

```yaml
power:
  enabled: true
  capabilities:
    poweroff: true
    reboot: true
```

#### Network binding

```yaml
bind: lo                      # loopback only (default)
# bind: enp2s0                # single LAN interface
# bind: [lo, enp2s0]          # loopback + LAN (required for UI access from the network)
# bind: [lo, enp2s0, wlan0]   # loopback + ethernet + wifi
# bind: all                   # all interfaces — 0.0.0.0 (Docker, remote access)
```

**Note:** The built-in web UI requires `lo` to be in the bind list. If `lo` is absent, the UI is automatically disabled.

#### Zeroconf / mDNS

```yaml
bind: eno1
zeroconf:
  enabled: true
```

Odio advertises itself via mDNS. Look for `_http._tcp.local.` → instance `odio-api`. Disabled on `lo` binding.

### Security defaults

- **Localhost binding by default** — prevents accidental network exposure
- **Systemd disabled by default** — service control must be explicitly enabled and configured
- **Read-only Docker mounts** — all volume mounts are read-only in the provided `docker-compose.yml`
- **Zeroconf opt-in** — must be enabled, then mDNS adapts to `bind`: disabled on `lo`, enabled on specific interfaces, or `all` interfaces without `lo`

## API Endpoints

### Server Information

```
GET    /server                             # {"hostname":"","os_platform":"","os_version":"","api_sw":"","api_version":"","backends":{"mpris":true,"pulseaudio":true,"systemd":false,"zeroconf":false}}
```

### MPRIS Media Players

```
GET    /players                           # List all media players
POST   /players/{player}/play             # Play
POST   /players/{player}/pause            # Pause
POST   /players/{player}/play_pause       # Toggle play/pause
POST   /players/{player}/stop             # Stop
POST   /players/{player}/next             # Next track
POST   /players/{player}/previous         # Previous track
POST   /players/{player}/seek             # Seek (body: {"offset": 1000000})
POST   /players/{player}/position         # Set position (body: {"track_id": "...", "position": 0})
POST   /players/{player}/volume           # Set volume (body: {"volume": 0.5})
POST   /players/{player}/loop             # Set loop status (body: {"loop": "None|Track|Playlist"})
POST   /players/{player}/shuffle          # Set shuffle (body: {"shuffle": true})
```

### PulseAudio

```
GET    /audio/server                      # Get server info
POST   /audio/server/mute                 # Mute/unmute server
POST   /audio/server/volume               # Set server volume (body: {"volume": 0.5})
GET    /audio/clients                     # List audio clients (sink-inputs)
POST   /audio/clients/{sink}/mute         # Mute/unmute client
POST   /audio/clients/{sink}/volume       # Set client volume (body: {"volume": 0.5})
```

### Systemd Services

```
GET    /services                          # List all monitored services
POST   /services/{scope}/{unit}/start     # Start service (scope: system|user)
POST   /services/{scope}/{unit}/stop      # Stop service (scope: system|user)
POST   /services/{scope}/{unit}/restart   # Restart service
POST   /services/{scope}/{unit}/enable    # Enable service (scope: system|user)
POST   /services/{scope}/{unit}/disable   # Disable service
```

### Power Management

```
GET    /power/                            # Power capabilities {"reboot": true, "power_off": false}
POST   /power/power_off                   # Poweroff (403 if not declared in capabilities)
POST   /power/reboot                      # Reboot (403 if not declared in capabilities)
```

### SSE Event Stream

```
GET    /ws                            # Server-Sent Events stream (text/event-stream)
```

#### Testing with curl

```bash
curl -N http://localhost:8018/ws
```

Expected output:

```
: connected

event: player.updated
data: {"bus_name":"org.mpris.MediaPlayer2.spotify","identity":"Spotify",...}

event: audio.updated
data: [{"id":42,"name":"Spotify","volume":0.75,"muted":false,...}]

event: service.updated
data: {"name":"mpd.service","scope":"user","active_state":"active","running":true,...}
```

#### Simple browser listener

```html
<!DOCTYPE html>
<html>
<head><title>Odio live events</title></head>
<body>
<pre id="log"></pre>
<script>
  const log = document.getElementById('log');
  const es  = new EventSource('http://localhost:8018/events');

  ['player.updated', 'player.added', 'player.removed',
   'audio.updated', 'service.updated'].forEach(type => {
    es.addEventListener(type, e => {
      const entry = `[${type}] ${e.data}\n`;
      log.textContent = entry + log.textContent;
    });
  });

  es.onerror = () => log.textContent = '[error] connection lost\n' + log.textContent;
</script>
</body>
</html>
```

Save as `events.html`, open in a browser — events appear live as they happen. No polling, no page refresh needed.

## Security

### systemd backend

⚠️ **Security Notice**

Systemd control is disabled by default and requires an explicit whitelist. Odio mitigates risks with deliberate security design:

- **Disabled by default** — must explicitly set `systemd.enabled: true` AND configure units. Empty config → auto-disabled even with `enabled: true`.
- **Localhost only** — API binds to `lo` by default. Never expose to untrusted networks or the Internet.
- **User-only mutations** — start/stop/restart/enable/disable only work on user D-Bus. System units are strictly read-only, enforced at the application layer regardless of D-Bus or polkit configuration. This protects against misconfigured or compromised D-Bus setups.
- **Root forbidden by design** — Odio refuses to run as root.
- **No preconfigured units** — nothing managed unless explicitly listed.

**You must knowingly enable this at your own risk.** Odio is free software and comes with no warranty.

### REST API

⚠️ **Security Notice:** No authentication mechanism is provided. **Never expose this API to untrusted networks or the Internet.** Designed for localhost or trusted LAN use only.

## Architecture

### Key Design: The User Session

All multimedia services run as systemd user units, not system-wide daemons. This unlocks a single, unified D-Bus session bus where PulseAudio/PipeWire, MPRIS players, and user systemd units all coexist. Odio listens to that bus and exposes everything via HTTP. Add a new MPRIS player — it appears immediately, zero code or config change.

### Backends

- **MPRIS Backend** — D-Bus communication with media players, smart caching, real-time D-Bus signal updates
- **PulseAudio Backend** — native PulseAudio protocol (pure Go, no libpulse), real-time event monitoring
- **Systemd Backend** — D-Bus with filesystem monitoring fallback (`/run/user/{uid}/systemd/units`)
- **Power Backend** — `org.freedesktop.login1` D-Bus interface

### Performance

- Caching reduces D-Bus calls by ~90%
- D-Bus signal-based updates instead of polling
- Batch property retrieval
- Automatic heartbeat management for position tracking
- Connection pooling and timeout handling

## Development

### Prerequisites

- Go 1.24 or higher

### Running Tests

```bash
go test ./...
go test -cover ./...

go test ./backend/mpris/...
go test ./backend/pulseaudio/...
go test ./backend/systemd/...
```

### Building

The project uses [Task](https://taskfile.dev) for build automation.

```bash
# Install Task (once)
go install github.com/go-task/task/v3/cmd/task@latest

# Build for the current host (CSS + Go binary, version from git)
task build

# Cross-compile for all supported architectures (output: dist/)
task build:all-arch

# Individual targets
task build:linux-amd64     # x86_64
task build:linux-arm64     # RPi 3/4/5 64-bit
task build:linux-armv7hf   # RPi 2/3 32-bit (ARMv7)
task build:linux-armhf     # RPi B/B+/Zero (ARMv6, RPi OS armhf)

# CSS only
task css              # Ensure CSS is available (compile or download from CDN)
task css-local        # Compile locally (requires Tailwind CLI)
task css:watch        # Watch mode for development
```

**Note:** `task build` injects the version via `-ldflags` from `git describe`. The version is visible via `./bin/odio-api --version`.

#### CSS Build Strategy

The UI uses Tailwind CSS with an intelligent multi-architecture build strategy:

- **Development (x64/arm64/armv7)** — `task build` compiles CSS locally using Tailwind CLI
- **Legacy ARM (ARMv6 — Raspberry Pi B/B+)** — `task build` downloads pre-built CSS from CDN (`https://bobbywan.me/odio-css/`)

Tailwind CLI doesn't provide ARMv6 binaries. The CSS is architecture-independent, so it's compiled on x64 and distributed via CDN.

**CDN structure:**
```
https://bobbywan.me/odio-css/
  main/abc1234.css          # commit-specific
  main/latest.css           # latest for branch
  tags/v0.6.0.css           # release tags (never cleaned)
```

CSS files are **not** committed to the repository.

### Packaging (deb / rpm)

Packages are built with [nfpm](https://nfpm.goreleaser.com/) via Task.

```bash
# Install nfpm (once)
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

# Build all packages for all architectures (output: dist/)
task package:all

# Individual targets
task package:deb:linux-amd64     # .deb amd64
task package:deb:linux-arm64     # .deb arm64
task package:deb:linux-armv7hf   # .deb armv7hf
task package:deb:linux-armhf     # .deb armhf (ARMv6, RPi OS)
task package:rpm:linux-amd64     # .rpm x86_64
task package:rpm:linux-arm64     # .rpm aarch64
task package:rpm:linux-armv7hf   # .rpm armv7hl
task package:rpm:linux-armhf     # .rpm armv6hl
```

## Dependencies

- [spf13/viper](https://github.com/spf13/viper) — configuration
- [godbus/dbus](https://github.com/godbus/dbus) — D-Bus bindings
- [coreos/go-systemd](https://github.com/coreos/go-systemd) — systemd D-Bus bindings
- [the-jonsey/pulseaudio](https://github.com/the-jonsey/pulseaudio) — pure-Go PulseAudio native protocol (no libpulse)
- [grandcat/zeroconf](https://github.com/grandcat/zeroconf) — mDNS / DNS-SD

## Contributing

Odio was first pushed on January 25, 2026. It's early stage. v0.4 works out of the box, but there's a long road ahead. Expect bugs.

**Does it work on your setup? What breaks? What's missing?**

Try it. Tell me what works and what doesn't. Show me your setup. If you want to contribute code, even better. Go is a great language for this use case.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

For issues and questions: [GitHub repository](https://github.com/b0bbywan/go-odio-api)

## License

BSD 2-Clause License — see the LICENSE file for details.
