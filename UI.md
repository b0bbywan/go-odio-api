# Odio Dashboard – Your Central Multimedia Control Interface

- **Total control**: MPRIS media players, global + per-app volume, systemd user services, power management
- **100% local, zero cloud**
- **Responsive everywhere**: phone, tablet, desktop, TV/HTPC  
- **Installable PWA** to manage multiple devices (HTPC + NAS + Raspberry Pi) [from one app](https://odio-pwa.vercel.app/)
- **lightweight**

The built-in Odio UI is accessible at:

**http://localhost:8018/ui** / **http://your-host.local:8018/ui**

### Configuration

UI must be enabled in configuration

```yaml
api:
  enabled: true
  port: 8018
  ui:
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
  - **Audio Server**: default sink, global volume, active clients with per-app volume control
  - **Media Players**: live list of MPRIS players (Spotify/go-librespot, MPD, Chrome, Kodi, Firefox instances…)
    - Dynamic cover art
    - Full metadata (title, artist, album)
    - Progress bar + seek support
    - Play/pause/stop/next/prev controls
  - **Services**: systemd user service status, stop/restart from the UI
  - **Power Management**: poweroff/restart (via systemd-logind, configurable capacities, disabled by default in odio-api config.)

- **Lightweight**
  - Low CPU and RAM usage, even with dashboard open
  - SSE planned for reduced overhead

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


### How to Install the Central PWA App

1. Open https://odio-pwa.vercel.app/ on your phone or tablet
2. Add to home screen (Chrome/Android: menu → Add to home screen | Safari/iOS: Share → Add to Home Screen)
3. Add your Odio instances (e.g. http://htpc.local:8018/ui, http://nas.local:8018/ui)

<img width="440" height="927" alt="image" src="https://github.com/user-attachments/assets/d32d330b-dbd4-4294-a0b6-b48f62be3dd8" />
<img width="441" height="927" alt="image" src="https://github.com/user-attachments/assets/6a4a005d-68c8-4aca-ad8f-cb9e3d966e52" />


### Known issues
The first version of the UI is fully based on polling, it creates some issues with volume and position sliders which will be solved in the next UI release.

To save resources, polling is disabled when a section is collapsed, or when the UI is hidden.

### Stack

- built-in UI: HTMX + Tailwind + go:embed.
  Every static file is served directly by the api, no external access necessary.
- [Progressive Web Application](https://github.com/b0bbywan/odio-pwa) Svelte + Vite + Vercel.
  Odio-UI are embedded as iframes

### Roadmap Highlights for the UI

- SSE for instant live updates
- Notifications

The UI is currently in v0.6.0 – feedback, issues & PRs are super welcome on GitHub!

Try it now: http://localhost:8018/ui
Central PWA app: https://odio-pwa.vercel.app/
