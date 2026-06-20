  <p align="center">
    <a href="https://odio.love"> 
      <img src="https://odio.love/logo.png" alt="odio" width="160" />
    </a>   
  </p>
  <h1 align="center">go-odio-api</h1>
  <p align="center"><em>The universal remote for your Linux multimedia server.</em></p>
  <p align="center">       
    <a href="https://github.com/b0bbywan/go-odio-api/releases"><img src="https://img.shields.io/github/v/release/b0bbywan/go-odio-api?include_prereleases" alt="Release" /></a>
    <a href="https://github.com/b0bbywan/go-odio-api/blob/main/LICENSE"><img src="https://img.shields.io/github/license/b0bbywan/go-odio-api" alt="License" /></a>
    <a href="https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml"><img src="https://github.com/b0bbywan/go-odio-api/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
    <a href="https://github.com/b0bbywan/go-odio-api/actions/workflows/build.yml"><img src="https://github.com/b0bbywan/go-odio-api/actions/workflows/build.yml/badge.svg" alt="Build" /></a>
    <a href="https://goreportcard.com/report/github.com/b0bbywan/go-odio-api"><img src="https://goreportcard.com/badge/github.com/b0bbywan/go-odio-api" alt="Go Report Card" /></a>
    <a href="https://github.com/sponsors/b0bbywan"><img src="https://img.shields.io/github/sponsors/b0bbywan?label=Sponsor&logo=GitHub" alt="GitHub Sponsors" /></a>   
  </p>
  <p align="center">
    <a href="https://docs.odio.love/api/mpris/"><img src="https://img.shields.io/badge/MPRIS-003399" alt="MPRIS" /></a>
    <a href="https://docs.odio.love/api/pulseaudio/"><img src="https://img.shields.io/badge/PulseAudio-0055AA" alt="PulseAudio" /></a>
    <a href="https://docs.odio.love/api/bluetooth/"><img src="https://img.shields.io/badge/Bluetooth-0082FC?logo=bluetooth&logoColor=white" alt="Bluetooth" /></a>
    <a href="https://docs.odio.love/api/systemd/"><img src="https://img.shields.io/badge/systemd-FF6B35" alt="systemd" /></a>
    <a href="https://docs.odio.love/api/power/"><img src="https://img.shields.io/badge/Power-10B981" alt="Power" /></a>
    <a href="https://docs.odio.love/api/zeroconf/"><img src="https://img.shields.io/badge/Zeroconf-6B21A8" alt="Zeroconf" /></a>
    <a href="https://docs.odio.love/api/events/"><img src="https://img.shields.io/badge/SSE%20Events-F97316" alt="SSE Events" /></a>   
  </p>
  <p align="center">   
    Part of the <a href="https://odio.love">odio</a> project — <a href="https://docs.odio.love/api/">full documentation</a>.
  </p>
  <p align="center">
    <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white" alt="Go" /></a>
    <a href="https://htmx.org/"><img src="https://img.shields.io/badge/htmx-3366CC?logo=htmx&logoColor=white" alt="htmx" /></a>
    <a href="https://tailwindcss.com/"><img src="https://img.shields.io/badge/Tailwind%20CSS-06B6D4?logo=tailwindcss&logoColor=white" alt="Tailwind CSS" /></a>
    <a href="https://github.com/features/actions"><img src="https://img.shields.io/badge/GitHub%20Actions-2088FF?logo=githubactions&logoColor=white" alt="GitHub Actions" /></a>
    <a href="https://github.com/b0bbywan/go-odio-api/pkgs/container/go-odio-api"><img src="https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white" alt="Docker" /></a>     
    <a href="https://docs.odio.love/api/installation/"><img src="https://img.shields.io/badge/deb-A81D33?logo=debian&logoColor=white" alt="deb" /></a>
    <a href="https://docs.odio.love/api/installation/"><img src="https://img.shields.io/badge/rpm-294172?logo=redhat&logoColor=white" alt="rpm" /></a>
  </p>

# odio-api
odio is an ultra-lightweight Go daemon that exposes a single clean REST API over your Linux user session's D-Bus: MPRIS players (Spotify, VLC, Firefox, MPD, Kodi), PulseAudio/PipeWire, systemd user services, and power management. No root. No hacks. Just Linux primitives.

Building a Linux multimedia setup is easy. Integrating it cleanly into Home Assistant always felt hacky, scattered integrations, SSH scripts, and fragile glue.


Tested on Fedora 43 Gnome, Debian 13 KDE, Raspbian 13, Openmediavault 8
Raspberry Pi B through Pi 5.
Works without any system tweak.

## Quick Start

```bash
# 1. Install
curl -fsSL https://apt.odio.love/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/odio.gpg
echo "deb [signed-by=/usr/share/keyrings/odio.gpg] https://apt.odio.love stable main" | sudo tee /etc/apt/sources.list.d/odio.list
sudo apt update && sudo apt install odio-api

# 2. Start
systemctl --user enable --now odio-api.service

# 3. Test (start any MPRIS player first — Spotify, VLC, MPD…)
curl http://localhost:8018/players
curl http://localhost:8018/audio/server
```

→ See [Installation](#installation) for RPM, Docker, or building from source.

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

Each backend has a full reference on **[docs.odio.love](https://docs.odio.love/api/)** — summaries below.

### Media Player Control (MPRIS)

Auto-discovers all MPRIS players in real time — Spotify, VLC, Firefox, MPD, Kodi, etc. Add a player and it appears immediately, zero config. Full playback control (play/pause/stop/next/previous, seek, position, volume, shuffle, loop), real-time state via D-Bus signals, smart caching, position heartbeat. → [reference](https://docs.odio.love/api/mpris/)

### Audio Management (PulseAudio/PipeWire)

Server info and default output, global and per-client volume/mute, real-time audio events via native PulseAudio monitoring (pure Go, no libpulse). Limited PipeWire support via `pipewire-pulse`. → [reference](https://docs.odio.love/api/pulseaudio/)

### Service Management (systemd)

Explicit whitelist required — nothing managed unless listed in `config.yaml`. List/monitor whitelisted system + user services, start/stop/restart/enable/disable user services, real-time state via D-Bus signals. Disabled by default. → [reference](https://docs.odio.love/api/systemd/)

⚠️ **Security model:** Odio enforces user-session mutations only at the application layer, regardless of D-Bus or polkit configuration. System units are strictly read-only. See [Security](#security) for full details.


### Bluetooth Sink (A2DP)

Acts as a Bluetooth audio receiver (A2DP sink) so phones and computers can stream to it — and the reverse, connecting *to* nearby speakers/headphones as an output (scan/connect/disconnect; devices stream live via `bluetooth.discovered`/`bluetooth.updated` SSE events). Disabled by default.

Setup needs a few system steps (Odio isn't root): add the user to the `bluetooth` group, install the PulseAudio/PipeWire Bluetooth module, and set `Name` + `Class=0x240428` in `/etc/bluetooth/main.conf` so it advertises as an audio sink. Full guide → [reference](https://docs.odio.love/api/bluetooth/) · [live example on a Pi B](UI.md#bluetooth-on-pi-b)

### Power Management

Remote reboot and power-off via the REST API — no SSH for day-to-day ops. Disabled by default, uses `org.freedesktop.login1`. On desktop, logind handles permissions automatically; on headless systems a polkit rule is required to allow the user to reboot/power-off (full rule → [reference](https://docs.odio.love/api/power/)).

### Software Upgrades

Agnostic upgrade frontend — Odio implements neither detection nor upgrade logic. It reads a result file written by an external detector (current/latest version, availability) and can trigger two systemd **user** units: one to re-check, one to run the upgrade. Capabilities are additive: the result file alone enables status reads (`GET /upgrade`), and each configured unit enables its trigger (`POST /upgrade/check`, `POST /upgrade/start`). Configured units are registered as internal — triggerable, but hidden from `/services` and the event stream. Disabled by default.

Run progress streams over a Unix socket, not a file or the journal: progress is ephemeral, and on SD-card systems a socket avoids write wear (per-user journals are also unavailable on Raspberry Pi). The upgrade script connects to the socket and writes one JSON line per milestone, relayed verbatim as `upgrade.progress` SSE events. The progress stream is the run's trunk (`begin`→`progress`→`end`); the unit only decorates a run we triggered, adding a state before the first line (started, awaiting progress) and the authoritative job result after. So an upgrade triggered through the unit (`POST /upgrade/start`) takes its verdict from the systemd job result (authoritative, and it covers a script killed before its `end`), independent of the script's self-report; an upgrade launched out of band (e.g. the script run from the CLI, streaming to the socket without going through the unit) is driven entirely by the stream, taking its verdict from the script's `end` — the unit's lifecycle never touches it. The in-flight run state is also mirrored in `GET /upgrade` (under `run`), so a client connecting or reloading mid-upgrade still sees it. A running upgrade is snapshotted to the state file on graceful shutdown, so a restart — e.g. odio-api upgrading itself — resumes the badge ring at its last percent: a unit run re-attaches to its still-queryable unit (emitting the verdict if it finished while down), and a CLI run resumes blind and waits for the script, which reconnects and resends its tail — notably `end` — onto the live run. When a run finishes the `upgrade.info` event carries its verdict, but only a **failure** is kept under `last_run` (`{success, finished_at, step, error}`) and persisted to a small state file, so a client loading after the fact still sees an unresolved failure; a successful (re)run clears it. It survives restarts. `error` is best-effort (from the script's `end` report; absent when the systemd job fails without one).

### Real-time Event Stream (SSE)

`GET /events` streams live state changes to any HTTP client — no polling. One event type per backend (`player.updated`, `audio.updated`, `service.updated`, `bluetooth.updated`, `power.action`, `upgrade.info`, …), filterable with `types`, `backend`, and `exclude` query params (`server.info` is always delivered). → [reference](https://docs.odio.love/api/events/)

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

- Wayland Remote Control, Authentication, Photos Casting...

## Installation

### APT Repository (Debian / Raspberry Pi OS)

```bash
curl -fsSL https://apt.odio.love/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/odio.gpg
echo "deb [signed-by=/usr/share/keyrings/odio.gpg] https://apt.odio.love stable main" | sudo tee /etc/apt/sources.list.d/odio.list
sudo apt update
sudo apt install odio-api
```

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

A pre-built multi-arch image is available on GHCR (amd64, arm64, arm/v7):

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

**Drop-in overrides (`conf.d/`):** any `*.yaml` / `*.yml` file dropped in a `conf.d/` directory next to the loaded config is merged on top of the main config in alphabetical order. Snippets override the main file, so `99-local.yaml` wins over `10-base.yaml`. Use this to layer per-host overrides without touching the base config — typical layout:

```
/etc/odio-api/
├── config.yaml
└── conf.d/
    ├── 10-services.yaml
    └── 99-local.yaml
```

Disabling a backend disables the backend and all its routes.

```yaml
bind: lo
logLevel: info

api:
  enabled: true
  port: 8018
  ui:
    enabled: true
  sse:
    enabled: true
  cors:
    origins: ["https://odio-pwa.vercel.app"] # default for PWA
    # origins: ["https://app.example.com"]  # specific origins
```

### Backend configuration examples

#### Network binding

```yaml
bind: lo                      # loopback only (default)
# bind: enp2s0                # single LAN interface
# bind: [lo, enp2s0]          # loopback + LAN (required for UI access from the network)
# bind: [lo, enp2s0, wlan0]   # loopback + ethernet + wifi
# bind: all                   # all interfaces — 0.0.0.0 (Docker, remote access)
```

**Note:** The built-in web UI requires `lo` to be in the bind list. If `lo` is absent, the UI is automatically disabled.

#### systemd (opt-in, whitelist required)

Each entry is a bare service name or an object `{name, url}` (mixable). When `url` is set the dashboard renders a clickable link; the shorthand `:8080` resolves to the current host client-side.

```yaml
systemd:
  enabled: true
  timeout: 90s                 # fsnotify stable-state timeout
  system:
    - bluetooth.service
  user:
    - mpd.service
    - spotifyd.service
    - name: snapclient.service
      url: "http://<snapserver>:1780"
    - name: mympd.service
      url: ":8080"
```

Players that need extra setup for MPRIS (MPD, shairport-sync, spotifyd, Kodi…) are covered in the [systemd reference](https://docs.odio.love/api/systemd/).

#### Other backends

```yaml
bluetooth:
  enabled: true
  powerOnStart: false          # power on adapter at startup
  idleTimeout: 30m             # auto power-off after inactivity (0 = never)
  scanTimeout: 60s             # auto-stop a scan (0 = never)
power:
  enabled: true
  capabilities: { poweroff: true, reboot: true }
pulseaudio:
  enabled: true
  serve_cookie: true           # exposes GET /audio/cookie for network audio clients
zeroconf:
  enabled: true                # mDNS (_http._tcp.local. → odio-api); disabled on `lo`
upgrade:                       # agnostic upgrade frontend (opt-in)
  enabled: true
  resultFile: /var/cache/odio/upgrades.json # required; alone it enables read-only GET /upgrade
  checkUnit: odio-check-upgrade.service     # optional internal user unit → enables POST /upgrade/check
  upgradeUnit: odio-upgrade.service         # optional internal user unit → enables POST /upgrade/start
  # progressSocket default: $XDG_RUNTIME_DIR/odio-api/upgrade.sock (tmpfs, no SD writes)
  # stateFile default: $XDG_STATE_HOME/odio-api/upgrade-run.json (persistent; last-run verdict)
```

For `upgrade`, the result file is the source of truth for availability; the script reports live progress over the socket (`begin`/`progress`/`end`). Units are optional — omit both for a read-only status badge, or add either to enable its POST. Full per-option detail on [docs.odio.love](https://docs.odio.love/api/).

### Security defaults

- **Localhost binding by default** — prevents accidental network exposure
- **Systemd disabled by default** — service control must be explicitly enabled and configured
- **Read-only Docker mounts** — all volume mounts are read-only in the provided `docker-compose.yml`
- **Zeroconf opt-in** — must be enabled, then mDNS adapts to `bind`: disabled on `lo`, enabled on specific interfaces, or `all` interfaces without `lo`

## API Endpoints

Full per-route reference with request bodies and responses lives on **[docs.odio.love/api](https://docs.odio.love/api/)**. Overview:

| Group | Routes | Reference |
|---|---|---|
| Server | `GET /server` | — |
| MPRIS | `GET /players`, `/players/{player}/cover`, `POST /players/{player}/{play,pause,play_pause,stop,next,previous,seek,position,volume,loop,shuffle}` | [mpris](https://docs.odio.love/api/mpris/) |
| PulseAudio | `GET /audio`, `/audio/{server,clients,outputs,cookie}`, `POST /audio/server/{mute,volume}`, `/audio/{clients,outputs}/{id}/{mute,volume}`, `/audio/outputs/{id}/default` | [pulseaudio](https://docs.odio.love/api/pulseaudio/) |
| systemd | `GET /services`, `POST /services/{scope}/{unit}/{start,stop,restart,enable,disable}` | [systemd](https://docs.odio.love/api/systemd/) |
| Bluetooth | `GET /bluetooth`, `/bluetooth/devices`, `POST /bluetooth/{power_up,power_down,pairing_mode,scan,scan/stop,connect,disconnect}` | [bluetooth](https://docs.odio.love/api/bluetooth/) |
| Power | `GET /power/`, `POST /power/{power_off,reboot}` | [power](https://docs.odio.love/api/power/) |
| Upgrade | `GET /upgrade`, `POST /upgrade/{check,start}` | [below](#software-upgrades-1) |
| SSE | `GET /events` | [events](https://docs.odio.love/api/events/) |

### Software Upgrades

Opt-in, disabled by default — see [Software Upgrades](#software-upgrades) for the model.

```
GET    /upgrade                           # Detector status, live run state, and available triggers (can_check/can_upgrade)
POST   /upgrade/check                     # Re-run detection (check unit); 202 Accepted — registered only when the check unit is configured
POST   /upgrade/start                     # Run the upgrade (upgrade unit); 202 Accepted, 409 if already running — registered only when the upgrade unit is configured
```

The detector's result file contract is `current`, `latest`, `upgrade_available` (required); `checked_at` is optional and must be RFC 3339 if present (dropped otherwise); anything else the detector writes (e.g. `roles`, `manifest`) is optional and passed through untouched. `GET /upgrade` returns the contract fields flat, the free ones grouped under `extra`, `run` while an upgrade is in flight, and `can_check`/`can_upgrade` telling clients which triggers are available:

```jsonc
// GET /upgrade
{
  "current": "2026.5.0b3",
  "latest": "2026.6.0b1",
  "upgrade_available": true,
  "checked_at": "2026-06-15T20:46:34Z",
  "extra": { "roles": ["setup", "..."], "manifest": { "odios": "2026.6.0b1" } },
  "run": { "state": "running", "percent": 42, "step": "mpd" },  // only during a run
  "can_check": true,                                            // POST /upgrade/check available
  "can_upgrade": true                                           // POST /upgrade/start available
}
```

Detector status and run lifecycle stream as `upgrade.info`; live run progress streams as `upgrade.progress` (its own type, like `player.position`):

```jsonc
// upgrade.info — detector status (on result-file change): the contract fields + "extra", no "run" or capability flags
{"current": "2026.5.0b3", "latest": "2026.6.0b1", "upgrade_available": true, "checked_at": "...", "extra": { /* ... */ }}

// upgrade.info — run lifecycle; success is the run's verdict (systemd job for a unit run, the script's end for a CLI run)
{"state": "running"}
{"state": "finished", "success": false, "step": "mpd", "error": "disk full"}

// upgrade.progress — emitted by the upgrade script over the socket; minimum contract below, add any field you want
{"event": "begin",    "total": 7}
{"event": "progress", "percent": 42, "current": 3, "step": "mpd"}
{"event": "end",      "success": false, "error": "disk full"}
```

### SSE Event Stream

```
GET /events                        # all events (text/event-stream)
GET /events?backend=mpris,audio    # filter by backend (mpris, audio, systemd, bluetooth, power, upgrade)
GET /events?types=player.updated   # filter by event type
GET /events?exclude=player.position
GET /events?keepalive=60
```

```bash
curl -N http://localhost:8018/events
```

Each line is `event: <type>` then `data: <json>`. Full event catalogue, payloads, and a browser `EventSource` example: [events reference](https://docs.odio.love/api/events/).

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
- [HTMX](https://htmx.org/)
- [TailwindCSS](https://tailwindcss.com/)

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
