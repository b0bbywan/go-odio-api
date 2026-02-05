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
- Start, stop, restart, and reload services
- Real-time service state updates
- D-Bus integration for efficient monitoring

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

### From Package

Debian/Ubuntu packages are available in releases:

```bash
# Download the .deb file from releases
dpkg -i go-odio-api_*.deb
```

## Configuration

Configuration is done via `config.yaml`:

```yaml
server:
  host: "localhost"
  port: 8080

mpris:
  enabled: true
  timeout: 5s

pulseaudio:
  enabled: true
  timeout: 5s

systemd:
  enabled: true
  timeout: 5s
```

## API Endpoints

### MPRIS Media Players

```
GET    /api/players              # List all media players
GET    /api/players/:busName     # Get player details
POST   /api/players/:busName/play
POST   /api/players/:busName/pause
POST   /api/players/:busName/playpause
POST   /api/players/:busName/stop
POST   /api/players/:busName/next
POST   /api/players/:busName/previous
POST   /api/players/:busName/seek
POST   /api/players/:busName/position
PUT    /api/players/:busName/volume
PUT    /api/players/:busName/loop
PUT    /api/players/:busName/shuffle
```

### PulseAudio

```
GET    /api/pulseaudio/sinks     # List audio sinks
GET    /api/pulseaudio/sources   # List audio sources
PUT    /api/pulseaudio/volume    # Set volume
```

### Systemd Services

```
GET    /api/systemd/services     # List services
POST   /api/systemd/start
POST   /api/systemd/stop
POST   /api/systemd/restart
POST   /api/systemd/reload
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
