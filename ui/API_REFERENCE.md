# UI API Reference

This document describes the JSON API response structures used by the UI layer.

## MPRIS Player API

**Endpoint:** `GET /players`

**Response:** Array of Player objects

```json
[
  {
    "bus_name": "org.mpris.MediaPlayer2.spotify",
    "identity": "Spotify",
    "playback_status": "Playing",  // "Playing", "Paused", "Stopped"
    "loop_status": "None",         // "None", "Track", "Playlist"
    "shuffle": false,
    "volume": 0.75,                // 0.0 - 1.0
    "position": 123456789,         // microseconds
    "rate": 1.0,
    "metadata": {
      "xesam:artist": "Artist Name",
      "xesam:title": "Track Title",
      "xesam:album": "Album Name",
      "mpris:trackid": "/org/mpris/MediaPlayer2/track/123",
      "mpris:length": 180000000    // microseconds
    },
    "capabilities": {
      "can_play": true,
      "can_pause": true,
      "can_go_next": true,
      "can_go_previous": true,
      "can_seek": true,
      "can_control": true
    }
  }
]
```

**UI Type Mapping:**

```go
type Player struct {
    Name     string            `json:"bus_name"`        // ⚠️ NOT "name"
    Metadata map[string]string `json:"metadata"`
    Status   string            `json:"playback_status"` // ⚠️ NOT "status"
    Position int64             `json:"position"`
    Volume   float64           `json:"volume"`
}
```

## Server Info API

**Endpoint:** `GET /server`

**Response:**

```json
{
  "hostname": "my-server",
  "os_platform": "linux/amd64",
  "os_version": "Ubuntu 22.04 LTS",
  "api_sw": "odio-api",
  "api_version": "0.5.0",
  "backends": {
    "bluetooth": false,
    "mpris": true,
    "pulseaudio": true,
    "systemd": true
  }
}
```

## PulseAudio Server API

**Endpoint:** `GET /audio/server`

**Response:**

```json
{
  "server_string": "PulseAudio (on PipeWire 1.4.10)",
  "default_sink": "alsa_output.pci-0000_00_1f.3.analog-stereo",
  "volume": 0.65,
  "muted": false
}
```

## PulseAudio Clients API

**Endpoint:** `GET /audio/clients`

**Response:** Array of AudioClient objects

```json
[
  {
    "index": 42,
    "name": "ALSA plug-in [spotify]",
    "application": "Spotify",
    "volume": 0.75,
    "muted": false
  }
]
```

## Systemd Services API

**Endpoint:** `GET /services`

**Response:** Array of Service objects

```json
[
  {
    "name": "bluetooth.service",
    "description": "Bluetooth service",
    "load_state": "loaded",
    "active_state": "active",    // "active" or "inactive"
    "sub_state": "running"       // "running", "dead", etc.
  }
]
```

## Common Mistakes

### ❌ Wrong JSON Tags

```go
// WRONG - will not parse correctly
type Player struct {
    Name   string `json:"name"`           // Backend sends "bus_name"
    Status string `json:"status"`         // Backend sends "playback_status"
}
```

### ✅ Correct JSON Tags

```go
// CORRECT - matches backend response
type Player struct {
    Name   string `json:"bus_name"`
    Status string `json:"playback_status"`
}
```

## Debugging Tips

1. **Check backend types:** Always refer to `backend/mpris/types.go` for exact JSON tags
2. **Use debug logs:** Run with `--log-level=debug` to see API request/response flow
3. **Inspect network:** Use browser DevTools Network tab to see actual JSON responses
4. **Test templates:** Run `go test ./ui/...` to catch parsing errors before runtime

## Related Files

- Backend types: `backend/mpris/types.go`
- UI types: `ui/types.go`
- API client: `ui/client.go`
- Handlers: `ui/handler.go`
