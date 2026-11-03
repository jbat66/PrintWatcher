# <img src="web/logo.png" alt="" width="64"> PrintWatcher
![Build Status](https://img.shields.io/github/actions/workflow/status/jbat66/PrintWatcher/build.yaml) ![License](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)

A self-hosted dashboard for monitoring multiple 3D printers — OctoPrint and PrusaLink — from a single web interface.

This was built for my own personal use and the way I want to interact with my printers, as well as an ongoing experiment in agentic coding.

> **Still Under Development** — expect rough edges, breaking changes, and evolving APIs. Feedback and contributions welcome.

## What it does

Each printer gets a row in the dashboard: webcam/snapshot, progress/ETA, temps, and smart plug power state/control. Updates push live via SSE. Everything's configured through the settings page — no config files, no restart needed.

## Supported platforms

- **PrusaLink** — supported, but only tested on MK4S and Core One
- **OctoPrint** — technically supported, but nothing to test against at this point


Plugin architecture — new platforms should be straightforward to add.

## Features

### Core (both platforms)

- Real-time dashboard updates via SSE, no manual refresh
- Print control — pause, resume, cancel
- File Manager per printer — browse every file on its storage, one-click reprint, download, or delete, thumbnail and material/tool previews for every file (native on OctoPrint, cached from the file's own metadata on PrusaLink)
- Print history per printer, with configurable retention (days, 0 = keep forever) and real thumbnails per entry
- Pushover notifications — print started/checkpoints/complete/failed/error, each independently configurable: priority, sound, image source (camera/thumbnail/none), and custom title/message templates
- Smart plug power control via a directly-configured Tasmota device, independent of any platform plugin, assignable to any printer — HTTP-direct (default) or MQTT (push-based, opt-in per plug)
- Auto-off after idle timeout and thermal runaway protection (second layer on top of firmware protection) for printers with an assigned smart plug
- Camera feed via HTTP or RTSP cameras, assignable to any printer — snapshot/live/plate-thumbnail toggle buttons under each card's image, with an optional global setting to hide the whole section for a printer while it's unreachable
- Multi-user login with per-account passwords, no roles/tiers
- Config backup/restore as YAML
- Printer reordering, optional free-text "model" field, dark mode, responsive layout

### OctoPrint

- Auto-detects camera stack (MJPEG or camera-streamer), printer name, and installed plugins
- Native smart plug power control + energy monitoring, auto-detected via the Tasmota or PSU Control plugin
- Layer progress display (needs the DisplayLayerProgress plugin)
- Live webcam streaming, with a snapshot/live toggle

### PrusaLink

- Snapshot only — no live stream from the printer's own camera (assign an HTTP or RTSP camera for a live feed)
- No native power control over the API — use a directly-configured smart plug instead
- Keepalive ping — keeps idle printers' wifi up; paused while the printer is off
- File upload from the dashboard, with an optional "print immediately"
- PrusaLink printers with an assigned smart plug are polled only once a minute while offline with every plug off
- Slicer print-host target — point PrusaSlicer/OrcaSlicer's "Send to printer" (PrusaLink mode) at PrintWatcher, pinned to one specific printer; "Upload" relays automatically once the printer's online, "Upload and Print" also powers it on first if it's off — no manual step either way
- Print history enriched with real per-print metadata — material, filament used/cost, layer height, duration vs. estimate, and per-tool breakdown for multi-material prints — parsed directly from the file's own slicer metadata, since none of this is exposed over PrusaLink's API

## Quick start

```bash
docker run -d \
  --name printwatcher \
  -p 8080:8080 \
  -v printwatcher-data:/data \
  ghcr.io/jbat66/printwatcher:latest
```

Open `http://<docker host IP>:8080` — first run prompts you to create a login. Once in, click the settings gear and add your first printer. You'll need the printer's URL and OctoPrint API key or Prusalink user/pass.

### Docker Compose

```yaml
services:
  printwatcher:
    image: ghcr.io/jbat66/printwatcher:latest
    ports:
      - "8080:8080"
    volumes:
      - printwatcher-data:/data
    restart: unless-stopped

volumes:
  printwatcher-data:
```

### Unraid

The container template is [UNRAID/printwatcher.xml](UNRAID/printwatcher.xml). It uses `ghcr.io/jbat66/printwatcher:latest`, maps host port `8086` to container port `8080`, and stores data under `/mnt/user/appdata/printwatcher`. The template icon and update URL point to this repository. HTTP and RTSP cameras require outbound connectivity from the container; no additional incoming camera port is needed.

The Build and Publish workflow tests and publishes to GitHub Container Registry using the built-in `GITHUB_TOKEN`, matching Prusa-Print-Monitor. Every release uses `vYYYY.MM.DD.I`, where the date is in America/Denver and `I` starts at 1 each day. Update `VERSION` for each logical change, commit it with the changes, and push a matching annotated Git tag. Tag pushes publish `latest`, the exact versioned image, and a GitHub release. Pushes to `main` run CI; manual publishing is also available. No Docker Hub account or extra registry secrets are required. The package must be public for Unraid to pull it without signing in; check the package settings after its first publication.

## Configuration

All printer management is done through the web UI — open the settings page to add, edit, reorder, and remove printers. No config files needed.

### Login

First run redirects to a setup page to create the first account. Add or remove additional users from Settings → Users. No roles or permission tiers — every account has full access.

### Smart plugs

OctoPrint printers with the Tasmota or PSU Control plugin installed get power control automatically — nothing to configure. For everything else (PrusaLink or an OctoPrint printer without the plugin), add a Tasmota device directly under Settings → Smart Plugs and assign it to a printer. Plugs are managed independently of printers, so deleting a printer unassigns its plug instead of deleting it.

Each plug can use either connection mode:
- **Direct HTTP** (default) — polled over the network on each printer's own poll interval, no extra setup
- **MQTT** — push-based via a broker instead of polling; configure the broker under Settings → MQTT Broker (host:port, optional TLS/username/password), then set the plug's mode to MQTT and enter its Tasmota topic

Both modes can be mixed freely across plugs.

### Cameras

Enter an HTTP camera base URL such as `http://192.168.1.50` for cameras exposing `/snapshot` and `/stream`, or a complete RTSP URL such as `rtsp://user:password@192.168.1.50:554/stream1`. URL-encode special characters in credentials. RTSP uses TCP and is decoded on the server to JPEG snapshots and browser-compatible MJPEG live video (up to 1280 pixels wide at 5 fps). Notification image attachments also support RTSP.

The Docker image includes FFmpeg. For native installations, install FFmpeg and ensure `ffmpeg` is on the server process PATH. Unreachable streams time out; up to eight RTSP decoders can run concurrently across live views and snapshots. Configure RTSP camera orientation and image quality in its own web interface; the in-app camera controls apply to compatible HTTP cameras only. LAN camera addresses are supported; loopback, link-local and multicast addresses are blocked.

Any printer type can get a webcam feed by assigning an HTTP camera or RTSP stream under Settings → Cameras — useful for PrusaLink, which has no webcam support of its own. Assigning a camera overrides whatever webcam a printer's own plugin would otherwise show. Cameras are managed independently of printers, so deleting a printer unassigns its camera instead of deleting it.

### Slicer print targets

Create a target under Settings → Slicer print targets, pinned to one printer. In PrusaSlicer or OrcaSlicer, add a physical printer using the **PrusaLink** host type and set:

- **Host:** the URL shown on the target, `http://<printwatcher-host>:<port>/ingest/<label>` (the target's label or numeric id both work)
- **API key / password:** the target's own API key, sent by the slicer as the `X-Api-Key` header. It is separate from your login and from the status API key, and this endpoint does not use the login session.

"Upload" stages the file and relays it to the printer as soon as it is online; "Upload and Print" also powers the printer on first if needed and then starts the print. A wrong key returns `401` and an unknown label or id returns `404`. Staged files are kept under `PRINTWATCHER_DATA_DIR/ingest/` until they have been relayed.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PRINTWATCHER_PORT` | `8080` | HTTP server port |
| `PRINTWATCHER_DATA_DIR` | `/data` | Data directory: the SQLite database (`printwatcher.db`) plus `ingest/<job id>/` staging folders for slicer uploads that haven't been relayed to a printer yet. Mount it on persistent storage with room for a few uploaded print files. |

## Getting your OctoPrint API key

1. Open your OctoPrint web interface
2. Go to **Settings** (wrench icon) → **API**
3. Copy the **Global API Key**, or create a new one under **Application Keys**

## Tech stack

- **Go** backend — single binary, low resource usage
- **SQLite** — no external database needed
- **Vanilla HTML/CSS/JS** frontend — no build step, no framework
- **Docker** — amd64 container

## Building from source

```bash
# Requires Go 1.26+ and CGO (for SQLite)
make build

# Or with Docker
make docker
```

## Contributing

PrintWatcher is in early development. If you'd like to contribute:

- Open an issue to discuss before submitting large changes
- Bug reports with `docker logs` output are especially helpful
- Plugin implementations for Moonraker/Klipper are welcome

## License

AGPL-3.0 — see [LICENSE](LICENSE) for details.
