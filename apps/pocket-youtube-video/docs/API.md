# API reference

Base path: `/api`. Requests and responses use JSON, except `/api/media/{video_id}`, which streams local media and supports HTTP byte ranges. Errors use `{"error":{"code":"...","message":"..."}}`; every route in this document follows that envelope.

## Accounts and runtime

- `GET /health`: server version, `schema_version`, runtime tool probes, auth configuration and mock flag.
- `POST /accounts/login` `{kind:"cookie"|"oauth", payload}` is the unified entry point; `kind=cookie` requires `payload.{name,base_url,cookie}`, `kind=oauth` requires `payload.{name,base_url,scopes[]}` and returns the authorization URL plus loopback callback.
- `GET /accounts`, `POST /accounts/{id}/activate`, `DELETE /accounts/{id}`, `POST /accounts/logout`.
- `POST /auth/cookie` with `{name,base_url,cookie}`.
- `POST /auth/oauth` with `{name,base_url,scopes}`. Returns authorization URL and local callback.

## Catalog

- `GET /home` returns home sections.
- `GET /subscriptions?page=1&limit=20`, `GET /channels?q=`, `GET /channels/{id}`. Collections honour `limit`/`offset` (default 20, max 100) and `page` as an offset shortcut.
- `GET /playlists?q=` (list with counts), `POST /playlists` `{title,description}` (creates a `user-…` playlist).
- `GET /playlists/{id}` returns the playlist with ordered `videos`; `POST /playlists/{id}` `{video_id}` adds a video; `DELETE /playlists/{id}/videos/{video_id}` removes one; `DELETE /playlists/{id}` deletes a user playlist. Synced (`playlist-NN`) playlists are read-only and return 409 on mutation.
- `GET /videos/{id}`.
- `GET /search?q=&type=video|channel|playlist|library` (default `video`). Each item carries `type` plus id/title; `library` searches only completed downloads with a local file. An unknown type returns 400 `invalid_search_type`.
- `GET /history?limit=&offset=` is paginated and returns each entry with `position_sec`, `watched_sec`, `duration_sec` and `completion_percent`.
- `POST /history` or `POST /history/progress` writes `{video_id,position_sec,duration_sec}` (204). `DELETE /history` clears all; `DELETE /history/{video_id}` removes one entry.
- `GET /watch_later`, `POST /watch_later/{video_id}`, `DELETE /watch_later/{video_id}`.
- Favorites use corresponding `/favorites` routes.
- `POST /sync` accepts `{what:subscriptions|playlists|watch_later|all}`. It refreshes local catalogue timestamps/counts and reports whether a provider-backed sync happened: with no authenticated provider it returns `provider_sync:"not_configured"` rather than claiming a remote fetch.

## Downloads and media

- `POST /downloads` accepts `{video_id,mode,quality,audio_bitrate,priority}`, an explicit `url`, or `{video_ids:[...]}` for batch queueing (returns `{queued,results[]}` with per-video errors).
- `GET /downloads?state=` polls jobs; `state` accepts the six lifecycle values and an unknown value returns 400 `invalid_state`. Listings honour `limit`/`offset`. `POST /downloads/{id}/pause|resume|retry|cancel`; `PUT /downloads/order` reorders jobs.
- `GET /library?q=&sort=&filter=`, `DELETE /library/{video_id}`.
- `GET /media/{video_id}` streams stored bytes with Range support.
- `POST /player/play|pause|stop|seek|volume|speed|fullscreen`; `GET /player/state`. `play` accepts `{video_id,url}` and starts mpv at the stored history position, returning `resume_sec`.

## Settings

`GET /settings`, `PUT /settings` persist UI language/theme/profile/DPI and media preferences (`download_dir`, `library_dir`, `subtitle_language`, `default_quality`, `storage_limit_gb`, `concurrency`) per account. GET includes defaults; PUT rejects unknown keys, invalid enums/ranges and directory values containing `..`.

Long operations are polled; no SSE endpoint is exposed. The UI polls `/api/player/state` while a video is open and writes progress back every 5 seconds.
