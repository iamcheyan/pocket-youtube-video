# Roadmap and verification status

## Implemented and verified

- Go `videod`, SQLite schema/seed with FTS5, account-scoped catalog/home/subscriptions/channels/playlists/video/history/watch-later/favorites/search/settings APIs.
- Deterministic demo data measured from a clean DB after two cold starts: 100 channels, 1000 videos, 20 playlists, 50 download fixtures, 50 watch-later entries, 25 history entries (seed is idempotent).
- AES-256-GCM credential storage with 0600 auto-created key; OAuth auth-code + PKCE + loopback callback and cookie import; encrypted account ID as AAD. DB-byte and API-response checks found no plaintext cookie; account metadata contains no secret material.
- Account switching isolation: after activating a newly imported cookie account, channels/history/downloads returned zero; switching back to `demo` restored its five first-page videos.
- Download manager: queued/downloading/paused/completed/failed/cancelled states, audio/video/both modes, 360p–2160p, 128/192/320 kbps, priorities, bulk order, pause/resume via SIGSTOP/SIGCONT, retry/cancel, startup recovery and yt-dlp progress/output parsing (stub + real download verified).
- mpv JSON IPC play/pause/stop/seek/volume/speed/fullscreen + state; tested against a synthetic lavfi source and a real downloaded webm.
- Offline media: account-scoped Range streaming (206 verified), library listing/deletion with path-escape protection.
- History: per-video progress, watched seconds and completion percentage; playback resumes from the stored position.
- Playlists: detail with ordered videos, user-created playlists (add/remove/delete) and read-only synced playlists.
- Downloads: single or batch queueing with per-item failure reporting; writes are serialized to avoid SQLite busy errors.
- Versioned schema migrations with `schema_migrations`, an in-place legacy upgrade path and `schema_version` in health.
- Black-box integration suite in `backend/tests` (wired into `make test`) covering health, seed totals, stub-download lifecycle, state filtering, account isolation, credential file permissions, media/library, typed search, playlist/settings validation and error envelopes.
- Device profiles: Desktop, TV, Handheld, DAP and Compact, each with distinct density/navigation CSS; narrow viewports use a working drawer that closes after navigation.
- React/Vite UI: home/subscriptions/channels/playlists/video detail/downloads/watch later/history/library/search/settings/accounts; Light/Dark/OLED/System themes; en/zh-CN/ja; desktop/TV/handheld profiles; 100–300% DPI; keyboard D-pad focus and Gamepad polling. 27 real Chromium screenshots cover pages, themes, viewports, profiles, locales and DPI.

## Limited or unverified

- Real Google OAuth login and authenticated YouTube sync were not exercised (no client credentials/account).
- Video pixels/audio output cannot be verified headless; only mpv IPC and state are proven.
- Provider playlist import and playlist reordering are not implemented; local create/detail/add/remove works.
- Resume is passed to mpv as `--start`; position polling is 5 s, so a crash can lose up to 5 s of progress.
- `.pj` packaging and having `videod` serve the built front-end assets are not implemented; Vite proxies `/api` in development.
- `POST /api/sync` does not reach a provider: without authenticated credentials it refreshes local timestamps and reports `provider_sync: not_configured`.
- Playlist import from a provider and in-playlist reordering are not implemented.
- The seeded `completed` download fixtures intentionally have no media files and are not listed in the offline library.
- No SSE endpoint; long operations are polled.

## `.pj` packaging migration

1. Make `make build` produce immutable `videod` and `frontend/dist` artifacts.
2. Embed UI assets in `videod` and serve them on the same origin; remove the dev-only proxy dependency.
3. Keep yt-dlp/mpv/ffmpeg external and probe them at startup.
4. Place the data dir and credential key inside the PJOS sandbox with explicit permissions.
5. Add a package manifest, capability declarations, launcher entry and rollback handling.
6. Add a SystemKeyringStore while keeping the file-encrypted fallback (no plaintext fallback).
7. Re-run all API, credential, real-download/stub, headless-mpv and screenshot checks against the packaged artifact.
