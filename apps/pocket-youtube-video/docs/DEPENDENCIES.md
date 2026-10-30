# Dependencies and verification limits

## Runtime programs

`yt-dlp`, `mpv`, and `ffmpeg` are system dependencies, not bundled. `videod` probes each with its version flag (`yt-dlp --version`, `mpv --version`, `ffmpeg -version`) and reports path/availability/version in `GET /api/health`.

Verified on this host: yt-dlp `2026.08.19`, mpv `v0.41.0`, ffmpeg `8.1.2`. The application does not pin a minimum version; use current releases whose CLI and mpv JSON IPC behaviour match those tested here. SQLite uses `modernc.org/sqlite` (pure Go, no sqlite3 CLI/cgo).

## Frontend build

Node.js/npm, React, TypeScript and Vite are required. At capture time the npm registry was initially unreachable, so the dependency tree was installed offline from the local npm cache after aligning the pinned versions (React 19.3.1, Vite 8.3.5, `@vitejs/plugin-react` 6.1.2, TypeScript 7.0.2). `npm run build` and `make build` both completed.

## Hardware limits

This HX90 has no physical display or audio output, so mpv is launched with `--vo=null --ao=null`. IPC state transitions (`play`, `pause`, `stop`, `seek`, `volume`, `speed`, `fullscreen`) and progress were validated against a real downloaded file. **Rendered video pixels and audible output were not verified** and require a display/audio device.

## YouTube network — observed results

Network access to YouTube **was available** during this run. A public, brief animated-short sample (`f7NwyBnIRTE`) was used:

- `yt-dlp ytsearch1:"Big Buck Bunny"` returned real search JSON (exit 0).
- A 3-second, 640×360 excerpt was fetched separately with `--download-sections '*0-3'` for a bounded check (28 KiB).
- Through `videod`, a real 360p download completed: job state `completed`, progress `100`, output `f7NwyBnIRTE.webm` (~25 MB). `GET /api/media/f7NwyBnIRTE` with `Range: bytes=0-1023` returned `206` and 1024 bytes.
- `GET /api/library` then listed the downloaded asset; `DELETE /api/library/f7NwyBnIRTE` removed both the record and the file.
- mpv IPC against that real file reported duration 634.6s, pause true, volume 42, speed 1.25, and seek time-pos 1.0.

These observations apply to this host/network at this time. They do not guarantee future YouTube availability, format selection, or authenticated access.

## YouTube account limits

No Google OAuth client credentials or YouTube account are configured on this host. The PKCE flow, loopback callback, token exchange and cookie import are implemented and code-tested, but **real Google authorization and authenticated subscription sync were not exercised**. Health and UI therefore report authentication as `not_configured`.
