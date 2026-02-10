# go-odio-api

[![CI](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml/badge.svg)](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/b0bbywan/go-odio-api)](https://goreportcard.com/report/github.com/b0bbywan/go-odio-api)

A lightweight REST API for controlling Linux audio and media players, built in Go. Provides unified interfaces for MPRIS media players, PulseAudio/PipeWire audio control, and systemd service management.

**Target Environment:** Designed for multimedia systems running with a user session (XDG_RUNTIME_DIR). Ideal for headless music servers, home audio systems, and dedicated media players.

## Features

### Media Player Control (MPRIS)
- List and control MPRIS-compatible media players (Spotify, VLC, Firefox, etc.)
- Full playback control: play, pause, stop, next, previous
- Volume and position control
- Shuffle and loop mode management
- Real-time player state updates via D-Bus signals
- Smart caching with automatic cache invalidation
- Position heartbeat for accurate playback tracking

### Audio Management (PulseAudio/PipeWire)
- List audio sinks and sources
- Volume control for sink-inputs only
- Real-time audio events via native PulseAudio monitoring
- Limited PipeWire support with pipewire-pulse

### Service Management (systemd)
- List and monitor systemd services
- Enable, disable, and restart services
- Real-time service state updates via D-Bus signals
- Tracking via filesystem monitoring for systemd without utmp (/run/user/{uid}/systemd/units)

⚠️ **Security Notice**

Yes, systemd control is controversial and potentially dangerous if misused. Odio mitigates risks with these deliberate security designs:

- **Disabled by default**: Systemd backend off unless explicitly enabled + units configured in `config.yaml` (empty config → auto-disabled, even with `systemd.enabled: true`).
- **Localhost only**: API binds to `127.0.0.1` by default. Never expose to untrusted networks/Internet.
- **No preconfigured units**: Nothing managed unless explicitly listed in config.
- **User-only mutations**: All mutations (start/stop/restart/enable/disable) use the *user* D-Bus connection only. System units are strictly read-only. While properly configured systems enforce this via D-Bus policies, odio adds mandatory application-layer enforcement which should protect against misconfigured or compromised D-Bus setups.
- **Hardened permission checks**: All public methods (`StartService`, `EnableService`, etc.) route through a unique code entrypoint called `Execute()` which **mandatorily** calls check actions are permitted in the configuration:
  | Scope | Check | Error |
  |-------|-------|-------|
  | System | Always blocked | `PermissionSystemError` |
  | User | Must be explicitly configured/watched | `PermissionUserError` |

**Root/sudo is not supported by design**: Odio runs as an unprivileged user with a user D‑Bus session. Running it as root is strictly forbidden and will be refused by the program. It is not supported, will not work by default, and should never be attempted. Issues or requests related to this will not be accepted, unless they improve security.

**You must knowingly enable this at your own risk.**
Odio is free software and comes with no warranty. Enabling systemd integration is at your own risk.

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
- XDG_RUNTIME_DIR=/run/user/1000 → DBus and Pulse runtime directory
- DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus → user DBus session
- HOME=/home/odio → ensures PulseAudio cookie is found

Volumes:
- /run/user/1000/bus → user DBus session (read-only)
- /run/user/1000/systemd → user systemd instance (read-only)
- /run/user/1000/pulse → PulseAudio socket (read-only)
- /var/run/dbus/system_bus_socket → system DBus socket (read-only)
- /run/utmp → login sessions (read-only)
- ./config.yaml → application configuration at /etc/odio-api/config.yml (read-only)
- ./cookie → PulseAudio authentication cookie at $HOME/.config/pulse/cookie (read-only)

The container exposes port 8080 and is configured to automatically restart unless stopped. With this configuration, audio and DBus-dependent functionality works seamlessly inside Docker.

Security note: All mounts are read-only, minimizing the container’s ability to modify the host system.

## Configuration

Configuration file can be placed at:
- `/etc/odio-api/config.yaml` (system-wide)
- `~/.config/odio-api/config.yaml` (user-specific)
- A default configuration is available in `share/config.yaml`

Disabling a backend will disable the backend and its routes !

Example configuration:

```yaml
systemd:
  enabled: true
  system:
    - bluetooth.service
    - upmpdcli.service
  user:
    - mpd.service
    - pipewire-pulse.service
    - pulseaudio.service
    - shairport-sync.service
    - snapclient.service
    - spotifyd.service

pulseaudio:
  enabled: true

mpris:
  enabled: true
  timeout: 5s

api:
  enabled: true
  port: 8080

logLevel: warn
```

## API Endpoints

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

### Server Informations

```
GET    /server                             # {"hostname":"","os_platform":"","os_version":"","api_sw":"","api_version":"","backends":{"mpris":true,"pulseaudio":true,"systemd":true}}
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

- Smart caching reduces D-Bus calls by ~90%
- Batch property retrieval (GetAll vs individual Gets)
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

```bash
# Standard build
go build -o odio-api

# Build with optimizations
go build -ldflags="-s -w" -o odio-api

# Cross-compile for different architectures
GOOS=linux GOARCH=amd64 go build -o odio-api-amd64
GOOS=linux GOARCH=arm64 go build -o odio-api-arm64
```

### Debian Package

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

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Built with [godbus](https://github.com/godbus/dbus) for D-Bus integration
- MPRIS specification by freedesktop.org
- PulseAudio and PipeWire projects
- systemd project for service management

## Support

For issues, questions, or contributions, please visit the [GitHub repository](https://github.com/b0bbywan/go-odio-api).
