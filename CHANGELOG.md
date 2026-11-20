# Changelog

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
