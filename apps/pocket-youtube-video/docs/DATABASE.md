# Database

`videod` uses SQLite through `modernc.org/sqlite`, a pure Go driver. Startup creates the schema idempotently. Foreign keys are enabled on the database connection. The schema is currently initialized by `internal/store.Migrate`; schema version tracking/migration history remains to be added.

## Core tables

`accounts`, `oauth_tokens`, `channels`, `videos`, `playlists`, `playlist_videos`, `downloads`, `watch_later`, `favorites`, `history`, `settings`, and FTS5 `video_search`.

Catalog, history, watch later, favorites and settings are keyed or queried by account. Deleting an account cascades catalog and encrypted token rows. Search uses FTS5. History stores playback position and last watch time. Download rows store format choices, state, progress, output path and account/video identity.

## Credentials

Token/cookie BLOB columns contain a packed GCM nonce followed by ciphertext. OAuth and cookie values are not stored in plaintext. The external key file is not part of SQLite; see [AUTH.md](AUTH.md).

## Deterministic demo seed

`store.Seed` inserts account `demo`, 100 channels, 1000 videos, 20 playlists, 50 watch-later entries and 25 history entries. `store.SeedDownloads` then inserts 50 deterministic download-fixture rows after the media schema exists; both steps are idempotent. Seeded identifiers and dates are fixed. This count is measured directly from the database/API, not inferred from UI sample content.

## Operations

The default DB is `$XDG_DATA_HOME`-style `~/.local/share/pocket-youtube-video/videod.sqlite`; set `POCKET_YOUTUBE_DB` or `POCKET_YOUTUBE_DATA_DIR` to override. Back up with videod stopped and preserve the separate `credential.key` file; losing it makes encrypted credentials unrecoverable.
