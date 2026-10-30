# API reference

Base path: `/api`. Requests and responses use JSON, except `/api/media/{video_id}`, which streams local media and supports HTTP byte ranges. Errors use `{"error":{"code":"...","message":"..."}}`; every route in this document follows that envelope.

## Accounts and runtime

- `GET /health`: server version, runtime tool probes, auth configuration and mock flag.
- `GET /accounts`, `POST /accounts/{id}/activate`, `DELETE /accounts/{id}`, `POST /accounts/logout`.
- `POST /auth/cookie` with `{name,base_url,cookie}`.
- `POST /auth/oauth` with `{name,base_url,scopes}`. Returns authorization URL and local callback.

## Catalog

- `GET /home` returns home sections.
- `GET /subscriptions?page=1`, `GET /channels?q=`, `GET /channels/{id}`.
- `GET /playlists?q=` (video counts and metadata). A dedicated `/api/playlists/{id}` detail/CRUD/import route is not implemented.
- `GET /videos/{id}`.
- `GET /search?q=&type=video|channel|playlist|library` (default `video`). Each item carries `type` plus id/title; `library` searches only completed downloads with a local file. An unknown type returns 400 `invalid_search_type`.
- `GET /history`, `POST /history/progress` with `{video_id,position_sec,duration_sec}`.
- `GET /watch_later`, `POST /watch_later/{video_id}`, `DELETE /watch_later/{video_id}`.
- Favorites use corresponding `/favorites` routes.
- `POST /sync` accepts `{what:subscriptions|playlists|watch_later|all}`.

## Downloads and media

- `POST /downloads` accepts `{video_id,mode,quality,audio_bitrate,priority}` or an explicit URL for engine testing.
- `GET /downloads?state=` polls jobs. `POST /downloads/{id}/pause|resume|retry|cancel`; `PUT /downloads/order` reorders jobs.
- `GET /library?q=&sort=&filter=`, `DELETE /library/{video_id}`.
- `GET /media/{video_id}` streams stored bytes with Range support.
- `POST /player/play|pause|stop|seek|volume|speed|fullscreen`; `GET /player/state`.

## Settings

`GET /settings`, `PUT /settings` persist UI language/theme/profile/DPI and media preferences (`download_dir`, `library_dir`, `subtitle_language`, `default_quality`, `storage_limit_gb`, `concurrency`) per account. GET includes defaults; PUT rejects unknown keys, invalid enums/ranges and directory values containing `..`.

Long operations are polled; no SSE endpoint is exposed.
