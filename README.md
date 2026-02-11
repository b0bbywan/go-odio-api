# go-odio-api

[![CI](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml/badge.svg)](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/b0bbywan/go-odio-api)](https://goreportcard.com/report/github.com/b0bbywan/go-odio-api)

A lightweight REST API for controlling Linux audio and media players, built in Go. Provides unified interfaces for MPRIS media players, PulseAudio/PipeWire audio control, and systemd service management.

**Target Environment:** Designed for multimedia systems running with a user session (XDG_RUNTIME_DIR). Ideal for headless music servers, home audio systems, and dedicated media players.

**Headless systems:** On fully headless systems, lingering needs to be enabled:

`loginctl enable-linger <username>`

This ensures the Pulseaudio/Pipewire, user D-Bus session and XDG_RUNTIME_DIR are available even without an active login session.

## Features

### API
Lightweight and fast REST API (<50ms 95% response time, 0% CPU on idle mode, tested on Raspberry B and B+)
enabled by default, listen on localhost by default. Change with your own IP according to your needs
Port can also be configured

```
# config.yaml
listen: 127.0.0.1
api:
  enabled: true
  port: 8018
```

### Media Player Control (MPRIS)
- List and control MPRIS-compatible media players (Spotify, VLC, Firefox, etc.)
- Full playback control: play, pause, stop, next, previous
- Volume and position control
- Shuffle and loop mode management
- Real-time player state updates via D-Bus signals
- Smart caching with automatic cache invalidation
- Position heartbeat for accurate playback tracking

```
# config.yaml
mpris:
  enabled: true
```

### Audio Management (PulseAudio/PipeWire)
- List audio sinks and sources
- Volume control for sink-inputs only
- Real-time audio events via native PulseAudio monitoring
- Limited PipeWire support with pipewire-pulse

```
# config.yaml
pulseaudio:
  enabled: true
```

### Service Management (systemd)
- List and monitor systemd services
- Enable, disable, and restart services
- Real-time service state updates via D-Bus signals
- Tracking via filesystem monitoring for systemd without utmp (/run/user/{uid}/systemd/units)
- Disabled by default for obvious security reasons

⚠️ **Security Notice**

Yes, systemd control is controversial and potentially dangerous if misused. Odio mitigates risks with these deliberate security designs:

- **Disabled by default**: Systemd backend off unless explicitly enabled + units configured in `config.yaml` (empty config → auto-disabled, even with `systemd.enabled: true`).
- **Localhost only**: API binds to `127.0.0.1` by default. Never expose to untrusted networks/Internet.
- **No preconfigured units**: Nothing managed unless explicitly listed in config.
- **User-only mutations**: Mutating ops (start/stop//restart/enable/disable) use *user* D-Bus connection only. System units strictly read-only, by default on your system if it's properly configured, inside odio if it's not.
- **Hardened permission checks**: All public methods (`StartService`, `EnableService`, etc.) route through a unique code entrypoint called `Execute()` which **mandatorily** calls check actions are permitted in the configuration:
  | Scope | Check | Error |
  |-------|-------|-------|
  | System | Always blocked | `PermissionSystemError` |
  | User | Must be explicitly configured/watched | `PermissionUserError` |

**Root/sudo is not supported by design**: Odio runs as an unprivileged user with a user D‑Bus session. Running it as root is strictly forbidden and will be refused by the program. It is not supported, will not work by default, and should never be attempted. Issues or requests related to this will not be accepted, unless they improve security.

**You must knowingly enable this at your own risk.**
Odio is free software and comes with no warranty. Enabling systemd integration is at your own risk.

```
# config.yaml (Useful service examples for audio servers or media centers, some are provided in `share/`)
systemd:
  enabled: true
  system:
    - bluetooth.service
    - upmpdcli.service
  user:
    - mpd.service                       (use mpd-mpris and mpDris2 for mpris support)
    - pipewire-pulse.service
    - pulseaudio.service
    - shairport-sync.service            (uncompatible with mpris but volume and mute can be controlled through pulseaudio)
    - snapclient.service                (same)
    - spotifyd.service                  (if mpris support available in spotifyd)

    - firefox-kiosk@netflix.com.service (will be detected by mpris)
    - firefox-kiosk@youtube.com.service (will be detected by mpris)
    - firefox-kiosk@my.home-assistant.io.service
    - kodi.service                      (install Kodi Add-on:MPRIS D-Bus interface)
    - vlc.service                       (will be detected by mpris)
    - plex.service                      (maybe, untested)
```

### Zeroconf / mDNS
The API advertise itself using Zeroconf (mDNS). This allows users to discover the API without knowing the host IP or port. Disabled with default 127.0.0.1, enabled otherwise

Disable in configuration:
```
zeroconf:
  enabled: false
```

Developers can discover the API with any mDNS/Bonjour browser on the network. Look for the service type `_http._tcp.local.` and instance name `odio-api`.


### Bluetooth Sink (A2DP)

Odio can act as a Bluetooth audio receiver (A2DP sink) using D-Bus, allowing phones, computers, and other Bluetooth devices to stream audio to it.

#### Configuration

To ensure the device is correctly identified by phones and computers, you must edit `/etc/bluetooth/main.conf`:

```ini
[General]
Name=Odio       # Bluetooth name shown during device discovery
Class=0x240428
```

Class of Device (CoD) breakdown:
- `0x24` → Major Device Class: **Audio/Video**
- `0x0428` → Minor + services :
  - **Audio Sink**
  - Loudspeaker
  - Rendering device

This configuration makes Odio appear as a standard Bluetooth speaker or audio receiver.

After modifying the configuration file, restart the Bluetooth service:
```bash
sudo systemctl restart bluetooth
```

#### Usage

Bluetooth is intentionally not left in an automatic or always-on state.
All Bluetooth operations are explicitly controlled through the Odio API.

- **Power up**:
  Bluetooth is enabled, but the device is not discoverable.
- **Power Down** There is no idle timeout on bluetooth (yet), so you have to explicitely turn off bluetooth
- **Pairing mode**:
  The device becomes visible to nearby Bluetooth devices and accepts new pairings.
  After a successful pairing (or when the timeout expires), Bluetooth automatically returns to its normal state:
    - Not discoverable
    - Not pairable
- Audio profile: **A2DP** (high-quality audio streaming).

This behavior matches how most Bluetooth speakers and audio receivers work.

### Logs
Different log levels, exhaustive info and debug logs to provide in issues.


## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/b0bbywan/go-odio-api.git
cd go-odio-api

# Build
go build -o odio-api

# Run
./odio-api
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

### Docker
You can also run odio as a container!

#### Build
Build the Go binary and Docker image
```bash
docker build -t odio:latest .
```
The Dockerfile uses a multi-stage build to compile the Go binary and copy it into a minimal runtime image.
**Note**: the image includes DBus so that DBus-dependent functionality works correctly inside the container.

#### Run
A docker-compose.yml is provided in the repository for the most common use cases. It runs the container as a non-root user (UID 1000) and mounts the necessary host directories for DBus, systemd, and PulseAudio. You can adapt it for more specific setups if needed

Environment variables:
- XDG_RUNTIME_DIR=/run/user/1000                            DBus and Pulse runtime directory
- DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus     user DBus session
- HOME=/home/odio                                           ensures `PulseAudio cookie` is found

Volumes:
- ./config.yaml                    (odio configuration)       (read-only)
- /run/user/1000/bus               (user DBus session)        (read-only)
- /run/user/1000/systemd           (user systemd folder)      (read-only)
- /run/utmp                        (user systemd monitoring)  (read-only)
- /var/run/dbus/system_bus_socket  (system DBus socket)       (read-only)
- /run/user/1000/pulse             (PulseAudio socket)        (read-only)
- ./cookie    (`PulseAudio cookie`$HOME/.config/pulse/cookie) (read-only)

The container exposes port 8018 by default and is configured to automatically restart unless stopped. With this configuration, audio and DBus-dependent functionality works seamlessly inside Docker.

**Note:** `listen` should be set to `0.0.0.0` in `config.yaml` for remote access with docker. Zeroconf won't work in bridge network mode. It's strongly advised against using host network mode.

All mounts are read-only, minimizing the container’s ability to modify the host system.

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
bind: 127.0.0.1
logLevel: warn

api:
  enabled: true
  port: 8018

zeroconf:
  enabled: true

systemd:
  enabled: true
  system:
    -
  user:
    -

pulseaudio:
  enabled: true

mpris:
  enabled: true
  timeout: 5s

bluetooth:
  enabled: true
  timeout: 5s
  pairingTimeout: 60s

```

## API Endpoints

### Server Informations

```
GET    /server                             # {"hostname":"","os_platform":"","os_version":"","api_sw":"","api_version":"","backends":{"mpris":true,"pulseaudio":true,"systemd":true, "zeroconf": true}}
```

### Bluetooth Sink
```
GET    /bluetooth                         # Get Bluetooth status (powered, pairing mode state)
POST   /bluetooth/power_up                # Turns Bluetooth on and makes the device ready to connect to already paired devices.
POST   /bluetooth/power_down              # Turns Bluetooth off and disconnects any active Bluetooth connections.
POST   /bluetooth/pairing_mode            # Enables Bluetooth pairing mode for 60s (configurable).
                                          # Returns to non-discoverable state after timeout or successful pairing.
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

## Architecture

### Backends

The application uses a modular backend architecture:

- **MPRIS Backend**: Communicates with media players via D-Bus, implements smart caching and real-time updates through D-Bus signals
- **PulseAudio Backend**: Interacts with PulseAudio/PipeWire for audio control, supports real-time event monitoring
- **Systemd Backend**: Manages systemd services via D-Bus with native signal-based monitoring
- **Bluetooth Backend**: Act as a Bluetooth audio receiver (A2DP sink) via D-Bus

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

- Go 1.21 or higher

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

# Build everything (CSS + Go binary)
task build

# Or build components separately
task css              # Ensure CSS is available (generate or download)
task css-local        # Compile CSS locally (requires Tailwind CLI)
task css:watch        # Watch mode for development

# Standard Go build (without Task)
go build -o bin/go-odio-api

# Build with optimizations
go build -ldflags="-s -w" -o bin/go-odio-api

# Cross-compile for different architectures
GOOS=linux GOARCH=amd64 go build -o bin/go-odio-api-amd64
GOOS=linux GOARCH=arm64 go build -o bin/go-odio-api-arm64
GOOS=linux GOARCH=arm GOARM=6 go build -o bin/go-odio-api-armv6
```

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

### Debian Packaging

```bash
# Build Debian package
cd debian
dpkg-buildpackage -us -uc -b
```

### RPM Packaging
```bash
mkdir -p ~/rpmbuild/RPMS/
rpmbuild -ba odio-api.spec
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
