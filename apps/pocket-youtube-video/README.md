# Pocket YouTube Video

An offline-first YouTube library built as a PocketJS reference application: React + TypeScript + Vite UI, Go `videod` service, SQLite, yt-dlp and mpv. The browser talks only to videod; it does not read local files or invoke media tools.

## Run

Requirements: Go 1.23+, Node.js/npm, and system `yt-dlp`, `mpv`, `ffmpeg`. The media programs are detected at runtime and are not bundled. The database schema is versioned and migrated automatically at startup; `GET /api/health` reports the applied `schema_version`.

```sh
make test               # unit/package tests + black-box integration suite
make test-integration   # integration suite only
make build              # videod binary + production frontend bundle
make dev                # videod + Vite dev server in one command
```

The API listens on `127.0.0.1:8732`; the Vite dev UI uses its default port. Set `POCKET_YOUTUBE_DATA_DIR` to choose the private data directory. Set `POCKET_YOUTUBE_ADDR` to change the bind address. `POCKET_YOUTUBE_SEED=0` disables deterministic demo seeding.

## Mock mode and verification status

Device profiles include Desktop, TV, Handheld, DAP and Compact; narrow viewports switch to a drawer that closes after navigation.

The demo account and seeded catalogue work without credentials. Measured from a clean database after two cold starts: 100 channels, 1000 videos, 20 playlists, 50 watch-later entries, 25 history entries, plus 50 download-fixture rows. Seeding is idempotent. Fixture `completed` rows deliberately have no media files and are not listed in the offline library.

OAuth PKCE, cookie import and AES-256-GCM credential storage are implemented; health reports `not_configured` when no non-demo authenticated account exists. **Real Google authorization and authenticated YouTube sync were not exercised** (no client credentials/account). A public, brief YouTube sample was, however, downloaded successfully through `videod` (state `completed`, 100%, ~25 MB webm), served over HTTP Range (`206`, 1024 bytes), listed in the library, deleted, and played through mpv IPC (`duration`, `pause`, `volume`, `speed`, `seek` verified). Network availability is not guaranteed for future runs.

Headless mpv uses `--vo=null --ao=null`: control and state are verified, but **rendered pixels and audible output are not**. See [DEPENDENCIES.md](docs/DEPENDENCIES.md) for exact versions and [SCREENSHOTS.md](docs/SCREENSHOTS.md) for the 28 real Chromium captures.

See [ARCHITECTURE.md](docs/ARCHITECTURE.md), [API.md](docs/API.md), [AUTH.md](docs/AUTH.md), [DATABASE.md](docs/DATABASE.md), [DEPENDENCIES.md](docs/DEPENDENCIES.md) and [ROADMAP.md](docs/ROADMAP.md).
