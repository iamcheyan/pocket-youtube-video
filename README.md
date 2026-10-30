# pocket-youtube-video

An offline-first YouTube library as a PocketJS reference app: React + TypeScript + Vite UI,
Go `videod` service, SQLite, yt-dlp and mpv. The browser only talks to videod.

- Application docs: [`apps/pocket-youtube-video/README.md`](apps/pocket-youtube-video/README.md)
- Architecture / API / auth / DB: `apps/pocket-youtube-video/docs/`
- Screenshots: `apps/pocket-youtube-video/docs/screenshots/` (index in `docs/SCREENSHOTS.md`)

## Run

```sh
make test          # Go packages + black-box integration suite
make build         # videod binary + production frontend bundle
make dev           # videod (127.0.0.1:8732) + Vite dev server
```

For the capture stack used by the screenshots:

```sh
POCKET_YOUTUBE_ADDR=127.0.0.1:8732 POCKET_YOUTUBE_DATA_DIR=/tmp/yt/data \
  apps/pocket-youtube-video/backend/videod
cd apps/pocket-youtube-video/frontend && \
  POCKET_YOUTUBE_API=http://127.0.0.1:8732 npx vite --host 127.0.0.1 --port 5202 --strictPort
make screenshots            # or: make verify-screenshots
```

`GET /api/health` reports schema version, auth state and detected tools. Data directory:
`POCKET_YOUTUBE_DATA_DIR` (default private app data dir). See `VERIFICATION.md` for the
2026-10-31 run.
