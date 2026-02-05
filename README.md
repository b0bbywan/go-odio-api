# go-odio-api

[![CI](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml/badge.svg)](https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/b0bbywan/go-odio-api/branch/main/graph/badge.svg)](https://codecov.io/gh/b0bbywan/go-odio-api)
[![Go Report Card](https://goreportcard.com/badge/github.com/b0bbywan/go-odio-api)](https://goreportcard.com/report/github.com/b0bbywan/go-odio-api)

A lightweight REST API for controlling Linux audio and media players, built in Go. Provides unified interfaces for MPRIS media players, PulseAudio/PipeWire audio control, and systemd service management.

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
- Headless tracking via filesystem monitoring (for systemd without utmp)
- User session detection based on DESKTOP environment variable

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

## Configuration

Configuration file can be placed at:
- `/etc/odio-api/config.yaml` (system-wide)
- `~/.config/odio-api/config.yaml` (user-specific)
- A default configuration is available in `share/config.yaml`

Example configuration:

```yaml
services:
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
POST   /audio/server/volume               # Set server volume
GET    /audio/clients                     # List audio clients (sink-inputs)
POST   /audio/clients/{sink}/mute         # Mute/unmute client
POST   /audio/clients/{sink}/volume       # Set client volume
```

### Systemd Services

```
GET    /services                          # List all monitored services
POST   /services/{scope}/{unit}/enable    # Enable service (scope: system|user)
POST   /services/{scope}/{unit}/disable   # Disable service
POST   /services/{scope}/{unit}/restart   # Restart service
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
- D-Bus development libraries
- PulseAudio/PipeWire libraries
- systemd with D-Bus support

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
dpkg-deb --build debian
```

## Dependencies

- [godbus/dbus](https://github.com/godbus/dbus) - D-Bus bindings for Go
- [gin-gonic/gin](https://github.com/gin-gonic/gin) - HTTP web framework
- PulseAudio client library
- systemd D-Bus API

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
