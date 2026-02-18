# go-odio-api

[![CI](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml/badge.svg)](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml)
[![Build](https://github.com/b0bbywan/go-odio-api/actions/workflows/build.yml/badge.svg)](https://github.com/b0bbywan/go-odio-api/actions/workflows/build.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/b0bbywan/go-odio-api)](https://goreportcard.com/report/github.com/b0bbywan/go-odio-api)

A lightweight and reliable REST API for controlling Linux audio and media players, built in Go. Provides unified interfaces for MPRIS media players, PulseAudio/PipeWire audio control, and systemd service management.

**Target Environment:** Designed for multimedia systems running with a user session (XDG_RUNTIME_DIR). Ideal for headless music servers, home audio systems dedicated media players and classic desktop sessions.

Tested and validated on Fedora 43 Gnome, Debian 13 KDE, Raspbian 13 Raspberry B and B+. Works without any system tweak.

## Features

### Media Player Control (MPRIS)
- List and control MPRIS-compatible media players (Spotify, VLC, Firefox, etc.)
- Full playback control: play, pause, stop, next, previous
- Volume and position control
- Shuffle and loop mode management
- Real-time player state updates via D-Bus signals
- Smart caching with automatic cache invalidation
- Position heartbeat for accurate playback tracking

```yaml
# config.yaml
mpris:
  enabled: true
```

### Audio Management (PulseAudio/PipeWire)
- List audio clients and server info with default output
- Output and client volume control/mute
- Real-time audio events via native PulseAudio monitoring
- Limited PipeWire support with pipewire-pulse

```yaml
# config.yaml
pulseaudio:
  enabled: true
```

### Service Management (systemd)
- List and monitor systemd services
- Start, Stop, Restart, Enable, Disable services
- Real-time service state updates via D-Bus signals
- Tracking via filesystem monitoring for systemd without utmp (/run/user/{uid}/systemd/units)
- Disabled by default

⚠️ **Security Notice**

Yes, systemd control is controversial and potentially dangerous if misused. But systemd units are really easy to setup and manage, and have great potential for features so I added it anyway, with a strong concern for security. Odio mitigates risks with these deliberate security designs:

- **Disabled by default**: Systemd backend off unless explicitly enabled + units configured in `config.yaml` (empty config → auto-disabled, even with `systemd.enabled: true`).
- **Localhost only**: API binds to `lo` by default. Never expose to untrusted networks/Internet.
- **No preconfigured units**: Nothing managed unless explicitly listed in config.
- **User-only mutations**: All mutations (start/stop/restart/enable/disable) use the *user* D-Bus connection only. System units are strictly read-only. While properly configured systems enforce this via D-Bus policies, odio adds mandatory application-layer enforcement which should protect against misconfigured or compromised D-Bus setups.
- **Root/sudo is not supported by design**: Odio runs as an unprivileged user with a user D‑Bus session. Running it as root is strictly forbidden and will be refused by the program. Issues or requests related to this will not be accepted, unless they improve security.

**You must knowingly enable this at your own risk.**
Odio is free software and comes with no warranty. Enabling systemd integration is at your own risk.

Useful examples for audio servers or media centers, some are provided in `share/`:

```yaml
# config.yaml
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

    - firefox-kiosk@netflix.com.service # default suppport for mpris
    - firefox-kiosk@youtube.com.service # default suppport for mpris
    - firefox-kiosk@my.home-assistant.io.service
    - kodi.service                      # see [4]
    - vlc.service                       # default suppport for mpris
    - plex.service                      # see [5]
```
[1] Install `mpd-mpris` or `mpDris2` for mpris support
[2] Check my [article on medium to use Shairport Sync/Airplay with pulseaudio and mpris support](https://medium.com/@mathieu-requillart/set-up-a-b83d9c980e75)
[3] Default on desktop, on headless your spotifyd version [must be built with mpris support](https://docs.spotifyd.rs/advanced/dbus.html)
[4] Install [Kodi Add-on:MPRIS D-Bus interface](https://github.com/wastis/MediaPlayerRemoteInterface#)
[5] Maybe supported, untested

### Power Management / Login1

This feature allows remote power-off or reboot of your appliance via the REST API — eliminating the need for SSH in most day-to-day scenarios.

The feature is **disabled by default** and only becomes active when:
- `power.enabled = true` in the configuration
- **at least one** capability is enabled (`reboot` and/or `power_off`)
- only power off and reboot available.

It uses D-Bus **org.freedesktop.login1** interface, like when you power off from your desktop session.

Backend validates declared capabilities at startup and fails fast if mismatched.

```yaml
# config.yaml
power:
  enabled: true
  capabilities:
    poweroff: true
    reboot: true
```

### REST API

Lightweight and fast REST API (<50ms 95% response time, 0% CPU on idle mode, tested on Raspberry Pi B and B+)

Enabled by default, binds to localhost for security. Configure network interface binding and port as needed:

```yaml
# config.yaml
bind: lo
# bind: enp2s0    # Specific network interface
# bind: wlan0     # WiFi interface
# bind: all       # All interfaces (Docker, remote access)
api:
  enabled: true
  port: 8018
```

⚠️ **Security Notice:** No authentication mechanism is provided. **Never expose this API to untrusted networks or the Internet.** Designed for localhost or trusted LAN use only.

### Zeroconf / mDNS
Odio-api advertises itself using Zeroconf (mDNS). This allows users to discover the API without knowing the host IP or port. Disabled by default and with `lo` bind.

Enable in configuration:

```yaml
bind: eno1
zeroconf:
  enabled: true
```

Developers can discover the API with any mDNS/Bonjour browser on the network. Look for the service type `_http._tcp.local.` and instance name `odio-api`.

### Extensive and Safe Default Configuration

Odio is designed with security and ease-of-use in mind through sensible defaults:

- **Modular backends** - Each backend (MPRIS, PulseAudio, systemd, zeroconf) can be independently enabled or disabled. Run only what you need: media control without audio management, systemd without MPRIS, etc. Even the API can be disabled, though odio loses its interest then.
- **Zero host configuration required** - Works out-of-the-box on any Linux system with a user session. No system-wide setup, daemon configuration, or special permissions needed for testing.
- **Localhost binding by default** - API listens on localhost by default, preventing accidental exposure to untrusted networks
- **Network interface binding** - Bind to network interfaces (`lo`, `eth0`, `wlan0`, `all`) instead of hardcoded IPs, surviving DHCP changes and making configurations portable across networks
- **Systemd disabled by default** - Service control must be explicitly enabled and configured with a whitelist, no services managed by default
- **Read-only Docker mounts** - All volume mounts are read-only by default in the `docker-compose.yml` provided, minimizing container's ability to modify the host system
- **Zeroconf optin** - mDNS must be enabled and adapts based on network binding: disabled on localhost, enabled on specific interfaces, broadcasts on all interfaces when `bind: all`

The default configuration requires no changes for local-only usage and provides clear, explicit opt-ins for remote access or privileged operations.

### Logs
Different log levels, exhaustive info and debug logs to provide in issues to help debugging.

## Installation

### Packages (deb / rpm)

Pre-built packages for amd64, arm64, armhf (ARMv6) and armv7hf are available as artifacts on each [build workflow run](https://github.com/b0bbywan/go-odio-api/actions/workflows/build.yml).

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

To run as a systemd user service, create `~/.config/systemd/user/odio-api.service`:

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

Then enable and start the service:

```bash
systemctl --user daemon-reload
systemctl --user enable odio-api.service
systemctl --user start odio-api.service
```

**Headless systems:** On fully headless systems, lingering needs to be enabled:

`sudo loginctl enable-linger <username>`

This ensures the Pulseaudio/Pipewire, user D-Bus session and XDG_RUNTIME_DIR are available even without an active login session.

### Docker

A pre-built multi-arch image is available on GHCR (amd64, arm64, arm/v6, arm/v7):

```
ghcr.io/b0bbywan/go-odio-api:latest
```

To build locally instead:
```bash
docker build -t odio-api .
```

#### Quick start

```bash
# 1. Prepare configuration (bind: all required for Docker)
cp share/config.yaml config.yaml
# Edit config.yaml: set bind: all

# 2. (Optional) Provide your PulseAudio cookie
cp ~/.config/pulse/cookie ./cookie

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
| `./cookie` → `$HOME/.config/pulse/cookie` | PulseAudio cookie |

**Note:** `bind` must be set to `all` in `config.yaml` for Docker (bridge network). Zeroconf won't work in bridge network mode. Host network mode is strongly discouraged.

#### Command-line Flags

- --config <path> specify a custom **yaml** configuration file.
- --version       print the version of go-odio-api and exit.
- --help          show help message

## Configuration

Configuration file can be placed at:
- `/etc/odio-api/config.yaml` (system-wide)
- `~/.config/odio-api/config.yaml` (user-specific)
- specified with `-config <path>`
- A default configuration is available in `share/config.yaml`

Disabling a backend will disable the backend and its routes !

Example configuration:

```yaml
bind: lo
logLevel: info

api:
  enabled: true
  port: 8018

zeroconf:
  enabled: false

systemd:
  enabled: false
  system:
    -
  user:
    -

pulseaudio:
  enabled: true

mpris:
  enabled: true
  timeout: 5s

```

## API Endpoints

### Server Informations

```
GET    /server                             # {"hostname":"","os_platform":"","os_version":"","api_sw":"","api_version":"","backends":{"mpris":true,"pulseaudio":true,"systemd":false, "zeroconf": false}}
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
GET    /power/                             # Power capabilites {"reboot": true, "power_off": false}
POST   /power/power_off                    # Poweroff (403 if not declared in capabilities)
POST   /power/reboot                       # Reboot (403 if not declared in capabilities)
```

## Architecture

### Backends

The application uses a modular backend architecture:

- **MPRIS Backend**: Communicates with media players via D-Bus, implements smart caching and real-time updates through D-Bus signals
- **PulseAudio Backend**: Interacts with PulseAudio/PipeWire for audio control, supports real-time event monitoring
- **Systemd Backend**: Manages systemd services via D-Bus with native signal-based monitoring

### Key Components

- **Cache System**: Optimized caching with TTL support to minimize D-Bus calls
- **Event Listeners**: Real-time monitoring via D-Bus signals for instant state updates
- **Heartbeat**: Automatic position tracking for playing media without constant polling
- **Graceful Shutdown**: Clean resource cleanup on application termination

### Performance Optimizations

- caching reduces D-Bus calls by ~90%
- Batch property retrieval
- D-Bus signal-based updates instead of polling
- Automatic heartbeat management for position tracking
- Connection pooling and timeout handling

## Development

### Prerequisites

- Go 1.24 or higher

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific backend tests
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

- **Development (x64/arm64/armv7)**: `task build` automatically compiles CSS locally using Tailwind CLI
- **Legacy ARM (ARMv6 - Raspberry Pi B/B+)**: `task build` downloads pre-built CSS from CDN (https://bobbywan.me/odio-css/)
- **CI/CD**: Two workflows handle CSS distribution:
  - **CSS changes**: Builds and uploads `commit_hash.css` + `latest.css`
  - **Non-CSS changes**: Initializes `latest.css` for new branches

**Why?** Tailwind CLI doesn't provide ARMv6 binaries. The CSS is architecture-independent (just text), so it's compiled on x64 and distributed via CDN.

**For Raspberry Pi B/B+ users:**
```bash
git pull
task build  # Downloads CSS automatically (commit-specific or latest), then builds Go binary
```

**CDN Structure:**
```
https://bobbywan.me/odio-css/
  main/abc1234.css             # Commit-specific CSS
  main/latest.css              # Latest CSS for main branch
  claude-branch-name/def5678.css
  claude-branch-name/latest.css
  tags/v0.6.0.css              # Release tags (versioned, never cleaned)
```

**Download strategy:** Task tries `branch/commit_hash.css` first, then falls back to `branch/latest.css` if not found.

**Note:** CSS files are NOT committed to the repository. They're generated locally or downloaded from CDN.

### Packaging (deb / rpm)

Packages are built with [nfpm](https://nfpm.goreleaser.com/) via Task. Requires the binary to be built first (handled automatically via task dependencies).

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

- [spf13/viper](https://github.com/spf13/viper) - Go configuration with fangs
- [godbus/dbus](https://github.com/godbus/dbus) - D-Bus bindings for Go
- [coreos/go-systemd](https://github.com/coreos/go-systemd) - Go bindings to systemd socket activation, journal, D-Bus, and unit files
- [the-jonsey/pulseaudio](https://github.com/the-jonsey/pulseaudio) - Pure-Go (no libpulse) implementation of the PulseAudio native protocol.
- [grandcat/zeroconf](https://github.com/grandcat/zeroconf) - mDNS / DNS-SD Service Discovery in pure Go

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the BSD 2-Clause License - see the LICENSE file for details.

## Acknowledgments

- Built with [godbus](https://github.com/godbus/dbus) for D-Bus integration
- MPRIS specification by freedesktop.org
- PulseAudio and PipeWire projects
- systemd project for service management

## Support

For issues, questions, or contributions, please visit the [GitHub repository](https://github.com/b0bbywan/go-odio-api).
