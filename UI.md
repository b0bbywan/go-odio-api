# Odio Dashboard – Your Central Multimedia Control Interface

- **Total control**: MPRIS media players, global + per-app volume, systemd user services, power management
- **100% local, zero cloud**
- **Responsive everywhere**: phone, tablet, desktop, TV/HTPC  
- **Installable PWA** to manage multiple devices (HTPC + NAS + Raspberry Pi) [from one app](https://odio-pwa.vercel.app/)
- **lightweight**

The built-in Odio UI is accessible at:

**http://localhost:8018/ui** / **http://your-host.local:8018/ui**

### Configuration

UI is enabled by default in configuration

```yaml
api:
  enabled: true
  port: 8018
  ui:
    enabled: true
  sse:
    enabled: true
```

Note: Necessary cors for the PWA are included by default but can be overridden.

### Key Features
- **Fully Responsive & Mobile-First**
  - Vertical layout on phones/tablets
  - Collapsible sections (▼/▲ arrows) to save space
  - Auto-refresh only when the tab is visible (saves battery & CPU)
  - Layout automatically adapts according to your configuration and enabled backends

- **Installable PWA**
  - Central app to manage multiple Odio instances: https://odio-pwa.vercel.app/
  - Add to home screen (Android/iOS)

- **Dynamic & Adaptive Sections**
  - **Audio Server**: default sink selector (click to switch output), global volume, active clients with per-app volume control
  - **Media Players**: live list of MPRIS players (Spotify/go-librespot, MPD, Chrome, Kodi, Firefox instances…)
    - Dynamic cover art
    - Full metadata (title, artist, album)
    - Progress bar + seek support
    - Play/pause/stop/next/prev controls
  - **Services**: systemd user service status, stop/restart from the UI
  - **Power Management**: poweroff/restart (via systemd-logind, configurable capacities, disabled by default in odio-api config.)

- **Lightweight & Real-Time**
  - Low CPU and RAM usage, even with dashboard open
  - SSE-powered live updates — sections refresh only when state changes, no polling

### Real-World Examples

#### HTPC / Desktop
Full control of Kodi, Netflix/YouTube kiosks, MPD, per-app volume, safe poweroff/restart.
I use `odio-ui.service` provided in `share` to start the interface in kiosk mode at boot so it looks like the HTPC homepage.

<img width="1658" height="963" alt="Capture d’écran du 2026-02-17 00-32-56" src="https://github.com/user-attachments/assets/0a9697da-902b-41d0-8977-908ef66f1168" />


#### Headless NAS
Multiroom Spotify Connect (via Snapcast) MPD player, MiniDLNA, Docker monitoring. 
No audio output on my NAS, it streams to pulseaudio and snapcast. Zeroconf and power Disabled

<img width="1209" height="549" alt="image" src="https://github.com/user-attachments/assets/e458019b-2c98-4ed8-8949-1d12489c0605" />


#### Raspberry Pi B+
Audio and Service only. MPRIS and power disabled

<img width="1274" height="754" alt="image" src="https://github.com/user-attachments/assets/b88e12df-77df-402a-8ba8-7b95556e6423" />

#### Bluetooth on Pi B

https://github.com/user-attachments/assets/07e0f04e-8758-452e-9561-4984c1dee554

**Quick explanation**: This shows the full feature in video
- My Phone wasn't paired yet to this adapter.
- You can see I start by powering `On` Bluetooth: The status indicator goes green
- By clicking `Pairing`, a 60s timer starts that will accept any new connection, as most Bluetooth speakers and receivers work.
- Once connected and trusted, a notification is played with `pactl` and appears in audio clients. It's a custom script not handled by Odio.
- Then my phone appears both in Audio Clients, and MPRIS players with the current song played. 1 connected client appears in the Bluetooth section.
- The play and volume commands are executed directly on the phone through BubbleUPnP, and reported automatically on the UI. Also tested with Spotify and Youtube.
- Bluetooth is finally powered off and my phone disconnected.

### How to Install the Central PWA App

1. Open https://odio-pwa.vercel.app/ on your phone or tablet
2. Add to home screen (Chrome/Android: menu → Add to home screen | Safari/iOS: Share → Add to Home Screen)
3. Add your Odio instances (e.g. http://htpc.local:8018/ui, http://nas.local:8018/ui)

<img width="440" height="927" alt="image" src="https://github.com/user-attachments/assets/d32d330b-dbd4-4294-a0b6-b48f62be3dd8" />
<img width="441" height="927" alt="image" src="https://github.com/user-attachments/assets/6a4a005d-68c8-4aca-ad8f-cb9e3d966e52" />


### How SSE live updates work

The backend streams real-time events to the UI via Server-Sent Events (`/ui/events`). Each event maps to a UI section (Audio, MPRIS, Systemd, Bluetooth) and triggers a targeted `innerHTML` swap via the HTMX SSE extension.

- **Debouncing**: events are batched in 200ms windows to avoid redundant renders when multiple properties change at once (e.g. track change + playback status + audio cork)
- **Section state preservation**: collapsible `<details>` sections remember their open/closed state across swaps — folding a section keeps it folded even as updates arrive
- **Position handling**: MPRIS position updates (`player.position`) are emitted every 5s by a heartbeat that polls D-Bus for playing players. Between updates, the seek bar is interpolated client-side at 500ms intervals for smooth progress display
- **Cover art cache-busting**: cover art URLs include the current track ID as a query parameter so the browser fetches the new image on track change
- **Dropdown protection**: the audio sink dropdown blocks SSE swaps on its section while open, preventing loss of user selection mid-interaction

### Known issues
Position seekers may lag up to 5s behind external seeks (e.g. seeking directly in Spotify) since D-Bus does not emit a signal for position changes.

### Stack

- built-in UI: HTMX 2 + HTMX SSE extension + Tailwind + go:embed.
  Every static file is served directly by the api, no external access necessary.
- [Progressive Web Application](https://github.com/b0bbywan/odio-pwa) Svelte + Vite + Vercel.
  Odio-UI are embedded as iframes

### Roadmap Highlights for the UI

- Notifications

#### What's new in v0.11.0
- **SSE live updates**: the UI now uses Server-Sent Events via the HTMX SSE extension. Sections update in real-time when state changes — no more polling.
- **HTMX 2 upgrade**: migrated from HTMX 1.9 to 2.0 with SSE extension v2
- **Section state preservation**: collapsed sections stay collapsed across SSE updates
- **Position & cover accuracy**: track changes reset the seek bar correctly, cover art updates immediately via cache-busting

Feedback, issues & PRs are super welcome on GitHub!

Try it now: http://localhost:8018/ui
Central PWA app: https://pwa.odio.love/
