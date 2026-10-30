# Verification — pocket-youtube-video @ 2026-10-31

- Verdict: PASS
- Commit: `__COMMIT__` (layout/favicon fixes + screenshots; this document is the following commit)
- Remote: `__REMOTE__`
- Runtime: `videod` on `127.0.0.1:8732` (mock seeded SQLite, temp data dir) + Vite on `127.0.0.1:5202`; `make build` production bundle
- Toolchain: go 1.26.0, node v22.23.3, Chrome 155.0.8059.84, yt-dlp/mpv/ffmpeg present

## Test gate
| command | result | notes |
|---|---|---|
| `go test ./...` | PASS | 44 tests across store/auth/api/media/provider packages, 0 failures |
| `go test ./tests -count=1 -v` | PASS | 9 black-box integration tests (health, seed counts, downloads, account isolation, credentials, player/library, search, playlists/settings, error envelope) |
| `npm run build` | PASS | `tsc -b && vite build` |
| `node apps/pocket-youtube-video/scripts/capture-screenshots.mjs` | PASS | 30/30 captures, 30 unique hashes, no overflow at any of the 30 configurations, zero console/HTTP errors |

`make test` covers the two Go suites; the whole gate finished green.

## Runtime
`GET /api/health` on the live instance reported SQLite, mock provider, schema applied, auth
`not_configured` (no credentials), and yt-dlp / ffmpeg / mpv detected. Seeded catalogue:
100 channels, 1000 videos, 20 playlists, 50 watch-later and 25 history rows (from the
integration test + API probes). Video detail route `#video/video-0001` renders the seeded
"Demo Video 0001" with channel, description and actions.

## Screenshot matrix
Captured this run by the new repo script `apps/pocket-youtube-video/scripts/capture-screenshots.mjs`
(30 declared configurations, Chromium via playwright-core). The script asserts per shot:
PNG dimensions, rendered theme/profile attributes, a non-empty heading, and zero horizontal
overflow; it aborts on duplicate SHA-256 or any console/HTTP error.
`docs/screenshots/manifest.json` records the run timestamp, capture URL, Chrome path and hashes.

| route | viewport | theme | file | note |
|---|---|---|---|---|
| accounts | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/accounts.png` | profile desktop, lang en, scale 100 |
| channels | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/channels.png` | profile desktop, lang en, scale 100 |
| downloads | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/downloads.png` | profile desktop, lang en, scale 100 |
| home | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/dpi-150.png` | profile desktop, lang en, scale 150 |
| home | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/dpi-200.png` | profile desktop, lang en, scale 200 |
| home | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/dpi-300.png` | profile desktop, lang en, scale 300 |
| history | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/history.png` | profile desktop, lang en, scale 100 |
| home | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/home.png` | profile desktop, lang en, scale 100 |
| library | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/library.png` | profile desktop, lang en, scale 100 |
| home | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/locale-ja.png` | profile desktop, lang ja, scale 100 |
| home | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/locale-zh-CN.png` | profile desktop, lang zh-CN, scale 100 |
| playlists | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/playlists.png` | profile desktop, lang en, scale 100 |
| home | 480×320 | dark | `apps/pocket-youtube-video/docs/screenshots/profile-dap.png` | profile dap, lang en, scale 100 |
| watch-later | 480×320 | dark | `apps/pocket-youtube-video/docs/screenshots/profile-desktop.png` | profile desktop, lang en, scale 100 |
| home | 480×320 | dark | `apps/pocket-youtube-video/docs/screenshots/profile-handheld.png` | profile handheld, lang en, scale 100 |
| home | 480×320 | dark | `apps/pocket-youtube-video/docs/screenshots/profile-tv.png` | profile tv, lang en, scale 100 |
| search | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/search.png` | profile desktop, lang en, scale 100 |
| settings | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/settings.png` | profile desktop, lang en, scale 100 |
| subscriptions | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/subscriptions.png` | profile desktop, lang en, scale 100 |
| video/video-0002 | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/theme-dark.png` | profile desktop, lang en, scale 100 |
| home | 1280×720 | light | `apps/pocket-youtube-video/docs/screenshots/theme-light.png` | profile desktop, lang en, scale 100 |
| home | 1280×720 | amoled | `apps/pocket-youtube-video/docs/screenshots/theme-oled.png` | profile desktop, lang en, scale 100 |
| video/video-0001 | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/video-detail.png` | profile desktop, lang en, scale 100 |
| downloads | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/viewport-1280x720.png` | profile desktop, lang en, scale 100 |
| home | 1920×1080 | dark | `apps/pocket-youtube-video/docs/screenshots/viewport-1920x1080.png` | profile desktop, lang en, scale 100 |
| home | 320×240 | dark | `apps/pocket-youtube-video/docs/screenshots/viewport-320x240.png` | profile desktop, lang en, scale 100 |
| home | 480×320 | dark | `apps/pocket-youtube-video/docs/screenshots/viewport-480x320.png` | profile desktop, lang en, scale 100 |
| home | 640×480 | dark | `apps/pocket-youtube-video/docs/screenshots/viewport-640x480.png` | profile desktop, lang en, scale 100 |
| home | 720×720 | dark | `apps/pocket-youtube-video/docs/screenshots/viewport-720x720.png` | profile desktop, lang en, scale 100 |
| watch-later | 1280×720 | dark | `apps/pocket-youtube-video/docs/screenshots/watch-later.png` | profile desktop, lang en, scale 100 |

Count: 30 screenshots, 30 unique hashes (0 duplicates). `tools/check_shots.py`: all PNGs valid,
dimensions match, minimum per-image RGB stddev above the blank threshold, no blank frames.

## Fixed
- UI scale was applied twice to layout lengths: `html{font-size:calc(16px * var(--ui-scale))}`
  already scales every `rem`, yet the sidebar (`15.25rem * scale`) and grid columns
  (`minmax(13rem * scale, 1fr)` and the 11rem/10rem variants) multiplied by `--ui-scale` again.
  At 200 % the document was 1850 px wide on a 1280 px viewport, at 300 % 4110 px — the DPI
  screenshots could only be taken by ignoring overflow. → `--sidebar:15.25rem`,
  `--density:1rem`, grid tracks `minmax(min(13rem,100%),1fr)` (same for 11rem/10rem)
  → `apps/pocket-youtube-video/frontend/src/style.css`. Now scrollWidth == clientWidth at
  100/150/200/300 %, verified in the capture run (dpi-150/200/300) and by a direct check.
- Handheld profile at 480×320 overflowed by 3 px: the topbar's search field refused to shrink
  (`flex:1` without `min-width:0`), pushing the user chip past the edge. → `min-width:0` on the
  narrow-viewport `.searchbox` → same file; `profile-handheld.png` (480×320) is now 480/480.
- Every page produced a console 404 (`/favicon.ico` did not exist). → added
  `apps/pocket-youtube-video/frontend/public/favicon.svg` and the `<link rel="icon">` in
  `index.html`; capture now ends with "no console errors".
- Added the missing repo-root `README.md` (run commands, ports 8732/5202, screenshot target)
  and a `make screenshots` / `make verify-screenshots` entry point, because the previous
  screenshots were produced by an ad-hoc browser session with no script in the repo.

## Blocked / open questions
- Real Google OAuth / authenticated sync was not exercised (no client credentials or account);
  `health.auth.status` is honestly `not_configured`. The mock-seeded UI is fully verified.
- No live network download was attempted in this run; yt-dlp is detected but the offline demo
  catalogue is what the screenshots show.

## Not verified here
- 掌机真机（RG34XX / PJOS-LINUX）未验证；本轮只在本机 headless Chrome 里跑真实进程。
- Rendered video pixels / audible output (mpv runs headless with `--vo=null --ao=null`).
