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

A lightweight Go daemon exposing your Linux user session as one REST API: MPRIS players (Spotify, VLC, Firefox, MPD, Kodi…), PulseAudio/PipeWire, systemd user services, Bluetooth, power, remote controls. No root, pure Go, no system tweak.

Tested on Fedora 43 Gnome, Debian 13 KDE, Raspberry Pi OS 13 and OpenMediaVault 8, from the Raspberry Pi B to the Pi 5. Packages for amd64, arm64, armv7hf and armhf (ARMv6), multi-arch Docker image.

## Quick start

```bash
curl -fsSL https://apt.odio.love/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/odio.gpg
echo "deb [signed-by=/usr/share/keyrings/odio.gpg] https://apt.odio.love stable main" | sudo tee /etc/apt/sources.list.d/odio.list
sudo apt update && sudo apt install odio-api

systemctl --user enable --now odio-api.service
curl http://localhost:8018/players
```

On a headless host, `sudo loginctl enable-linger <user>` keeps the user session alive without a login. RPM, Docker and source builds: [installation](https://docs.odio.love/api/installation/).

## Clients

<img width="1658" height="963" alt="odio web UI" src="https://github.com/user-attachments/assets/0a9697da-902b-41d0-8977-908ef66f1168" />

- **Web UI** at `http://localhost:8018/ui`, responsive, 100% local ([more](UI.md))
- **[PWA](https://pwa.odio.love/)**, installable, switches between several nodes
- **[odio-ha](https://github.com/b0bbywan/odio-ha)**, the Home Assistant integration (HACS custom repository): global audio and each whitelisted service as `media_player` entities, MPRIS players in progress

## Backends

systemd, Power, Input and Upgrade are off by default.

| Backend | What it does |
|---|---|
| [MPRIS](https://docs.odio.love/api/mpris/) | Auto-discovers players; playback, seek, volume, shuffle, loop, tracklist |
| [PulseAudio](https://docs.odio.love/api/pulseaudio/) | Global, per-output and per-client volume/mute, default output, opt-in mono / network sink / LAN discovery. PipeWire via `pipewire-pulse` |
| [systemd](https://docs.odio.love/api/systemd/) | Whitelisted units: user units controllable, system units read-only |
| [Bluetooth](https://docs.odio.love/api/bluetooth/) | A2DP sink for phones, and connecting out to speakers/headphones |
| [Power](https://docs.odio.love/api/power/) | Reboot, power-off, pending-reboot flag |
| [Input](https://docs.odio.love/api/input/) | IR, GPIO, HDMI-CEC, Bluetooth and USB remotes mapped to actions |
| [Upgrade](https://docs.odio.love/api/upgrade/) | Frontend for an external detector and upgrade unit, live progress |
| [Zeroconf](https://docs.odio.love/api/zeroconf/) | mDNS announce on the bound interfaces |
| [Events](https://docs.odio.love/api/events/) | SSE stream of every state change, `GET /events` |

Some backends need system setup (groups, polkit, BlueZ): see each page.

## Configuration

Read from `--config <path>`, `~/.config/odio-api/config.yaml`, then `/etc/odio-api/config.yaml`. `*.yaml` files in a `conf.d/` next to it are merged on top in alphabetical order. A disabled backend has no routes. Defaults are in [`share/config.yaml`](share/config.yaml), every option on [docs.odio.love](https://docs.odio.love/api/configuration/).

```yaml
bind: lo                 # or an interface, a list, or `all`; the UI needs `lo`
api:
  port: 8018
systemd:
  enabled: true
  user:
    - mpd.service
    - name: snapclient.service
      url: "http://<snapserver>:1780"
      open: panel        # tab (default), panel or self
```

## API

| Group | Routes |
|---|---|
| Server | `GET /server` |
| MPRIS | `GET /players`, `/players/{player}/{cover,tracklist}`, `POST /players/{player}/{play,pause,play_pause,stop,next,previous,seek,position,volume,loop,shuffle}`, `POST /players/{player}/tracklist/{add,goto,remove}` |
| PulseAudio | `GET /audio`, `/audio/{server,clients,outputs,cookie,processing}`, `POST /audio/server/{mute,volume}`, `/audio/{clients,outputs}/{id}/{mute,volume}`, `/audio/outputs/{id}/default`, `/audio/processing/{feature}/{enable,disable}` |
| systemd | `GET /services`, `POST /services/{scope}/{unit}/{start,stop,restart,enable,disable}` |
| Bluetooth | `GET /bluetooth`, `/bluetooth/devices`, `POST /bluetooth/{power_up,power_down,pairing_mode,scan,scan/stop,connect,disconnect}` |
| Power | `GET /power`, `POST /power/{power_off,reboot}` |
| Input | `GET /input` |
| Upgrade | `GET /upgrade`, `POST /upgrade/{check,start}` |
| Events | `GET /events?backend=…&types=…&exclude=…` |

## Security

There is no authentication: keep the API on localhost (the default bind) or a trusted LAN, never the Internet.

- odio-api refuses to run as root.
- systemd is off by default and manages only whitelisted units. Only user units can be changed; system units are read-only whatever D-Bus or polkit allow.
- The `input` group lets any process of that user read every input device. odio-api itself opens only remotes (IR, `gpio-keys`, CEC, devices listed by name) and publishes only bound keys.
- The Docker compose mounts everything read-only.

## Development

Needs Go 1.24+ and [Task](https://taskfile.dev).

```bash
go test -race ./...
task build              # CSS + binary, version from git describe
task build:all-arch     # cross-compile to dist/
task package:all        # deb + rpm via nfpm
task css:watch          # Tailwind watch mode
```

Tailwind has no ARMv6 build, so on Pi B/Zero `task build` downloads the prebuilt CSS from `https://bobbywan.me/odio-css/` instead of compiling it. CSS is not committed.

## Contributing

Early stage, expect bugs. Tell me what works on your setup and what doesn't, issues and PRs welcome on [GitHub](https://github.com/b0bbywan/go-odio-api).

## License

BSD 2-Clause, see [LICENSE](LICENSE).
