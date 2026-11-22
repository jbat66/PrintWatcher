# Changelog

## v2026.11.22.2

- Show an always-visible, bottom-centered remaining-time overlay on enlarged mobile video without reducing the video area.

## v2026.11.22.1

- Remove the connected-printer count from the header.
- Add a full-screen toggle to the left of Upload, with an Exit full screen label while active.

## v2026.11.21.4

- Toggle enlarged camera viewing with one click or tap on desktop and mobile.
- Restore camera image visibility after a failed request recovers, preventing a hidden image and hidden placeholder from leaving a black view.

## v2026.11.21.3

- Condense the enlarged desktop camera caption and controls without changing the remaining-time value size.
- Add mobile double-tap video-only landscape viewing, native fullscreen/orientation requests where supported, and a rotated fallback.

## v2026.11.21.2

- Include U1/Klipper in Upload with printer-specific G-code validation and storage routing.
- Move Enable sounds to the header and add independent per-printer mute and acknowledgement controls.
- Limit browser alerts to printer errors, filament-change requests, and confirmed print completion.

## v2026.11.21.1

- Double-click LIVE cameras to enlarge them with a prominent Time Remaining display; double-click again or press Escape to return.
- Add per-printer browser alert sound selection, repeating state-change alerts, acknowledgement, and mute controls.
- Bundle four MP3 alert sounds in the Docker image and preserve sound choices in configuration backups.

## v2026.11.20.3

- Fetch fresh app HTML, JavaScript, and CSS on refresh to prevent stale browser assets after updates.
- Match Chamber temperature typography and label placement to Hotend and Bed.

## v2026.11.20.2

- Show the current Hotend, Bed, and Chamber temperatures in Fahrenheit beside Celsius, while retaining Celsius targets.

## v2026.11.20.1

- Improve Klipper remaining time and ETA using successful unchanged-file print history, then slicer timing, with file progress as a fallback.

## v2026.11.17.4

- Wake the Snapmaker U1 built-in camera on demand and wait for a fresh JPEG for SNAP and LIVE.

## v2026.11.17.3

- Advertise the built-in Klipper camera as LIVE-capable once discovered.
- Estimate the current Klipper layer from commanded Z and slicer metadata when the firmware omits it.

## v2026.11.17.2

- Use the Snapmaker U1 built-in camera for SNAP and one-frame-per-second LIVE view.
- Clarify the Klipper printer type and API key labels.

## v2026.11.17.1

- Add Snapmaker U1 support through Moonraker with API-key authentication, monitoring, print controls, and G-code file management.

## v2026.11.03.1

- Allow 100% checkpoints and independent Pushover delays, with camera capture after the delay.

## v2026.11.02.2

- Center each primary stat reading within its box while preserving the current font and box sizes.
- Keep labels clear of centered readings in narrow layouts.

## v2026.11.02.1

- Adopt date-based release versions and matching annotated tags for every change.
- Center the current local time at the top of the dashboard.
- Place elapsed time, remaining time, ETA, hotend, and bed readings beside their labels, with larger desktop values in the existing information boxes.
- Read the embedded `VERSION` for native development builds and display the version with a single leading `v`.
- Verify the running container's reported version before completing publication.
