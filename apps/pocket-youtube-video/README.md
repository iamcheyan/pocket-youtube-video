# Pocket YouTube Video

An offline-first YouTube library built as a PocketJS reference application: React + TypeScript + Vite UI, Go `videod` service, SQLite, yt-dlp and mpv. The browser talks only to videod; it does not read local files or invoke media tools.

## Run

Requirements: Go 1.23+, Node.js/npm, and system `yt-dlp`, `mpv`, `ffmpeg`. The media programs are detected at runtime and are not bundled.

```sh
make test
make build
make dev
```

The API listens on `127.0.0.1:8732`; the Vite dev UI uses its default port. Set `POCKET_YOUTUBE_DATA_DIR` to choose the private data directory. Set `POCKET_YOUTUBE_ADDR` to change the bind address. `POCKET_YOUTUBE_SEED=0` disables deterministic demo seeding.

## Mock mode and current limits

The demo account and seeded catalogue work without credentials. Current measured seed is 100 channels, 1000 videos, 20 playlists, 50 watch-later entries and 25 history entries; 50 sample download queue rows are also inserted as non-media fixture states. These rows do not contain downloaded files. Health reports authentication as `not_configured` unless a non-demo account has been added.

OAuth PKCE and cookie import use encrypted local storage. Actual Google OAuth was not exercised because this machine has no credentials. YouTube network access and real download outcomes are not asserted. Headless mpv uses null audio/video outputs; its control IPC can be tested, but visual and audible playback cannot be verified here.

See [ARCHITECTURE.md](docs/ARCHITECTURE.md), [API.md](docs/API.md), [AUTH.md](docs/AUTH.md), [DATABASE.md](docs/DATABASE.md), [DEPENDENCIES.md](docs/DEPENDENCIES.md) and [ROADMAP.md](docs/ROADMAP.md).
