# Architecture

Pocket YouTube Video is a PocketJS style React + TypeScript + Vite client backed by `videod`, a Go HTTP service. The browser calls JSON REST endpoints only. SQLite, yt-dlp, mpv, local storage and authentication are accessed only in Go.

```text
React/Vite UI → videod REST API → SQLite / credential store / yt-dlp / mpv
```

## Runtime

`backend/cmd/videod` initializes the user data directory, encrypted credential key, versioned SQLite schema (see `internal/store/migrate.go`) and deterministic demo seed, then mounts catalog, auth and media handlers. `frontend` is served by Vite in development; production assets can be reverse-proxied by videod or packaged into a `.pj` runtime later. That static serving integration is not implemented yet.

Black-box HTTP integration tests live in `backend/tests` (Go's `internal/` rule keeps them inside the module) and run from `make test`; `apps/pocket-youtube-video/tests/README.md` points at them.

The backend packages are divided into `internal/api` (catalog and account data), `internal/store` (SQLite schema/seed), `internal/auth` (PKCE, cookie import and AES-GCM), `internal/provider` (provider contract and mock/YouTube adapters), and `internal/media` (downloads, player, library).

## State and boundaries

All user catalog/history/settings data belongs to an account. The `demo` account is synthetic and deterministic. OAuth and cookie material is encrypted before persistence. The UI never reads the database or local files and never starts external processes.

## Long-running operations

Downloads are submitted as queued jobs and their current state is polled through `GET /api/downloads`; media operations use JSON IPC. No SSE endpoint is implemented. API clients poll at a modest interval while a job/player is active.

## Limits

The current executable and front end are not packaged as `.pj`. Real YouTube credentials and account sync are unverified. Headless mpv can verify process/IPC behavior but not display or audio output. See [DEPENDENCIES.md](DEPENDENCIES.md) and [ROADMAP.md](ROADMAP.md).
