# Screenshots

All images were captured from the running Vite application (`http://127.0.0.1:5202`) backed by the live `videod` instance (`http://127.0.0.1:18803`), using headless Chromium through the browser automation target with `page.setViewport` and `page.screenshot`. They are not placeholder images, screenshots of a different project, or copied images.

Reproduce: `make build`, start `videod` on 127.0.0.1:8732 and `POCKET_YOUTUBE_API=http://127.0.0.1:8732 npx vite --host 127.0.0.1 --port 5202 --strictPort`, then `make screenshots`. The script aborts on duplicate hashes, wrong PNG size, horizontal overflow, an unrendered heading, or any console/HTTP error, and rewrites `manifest.json` with the run timestamp. The URL carries theme/language/profile/DPI (`?theme=dark&lang=en&profile=desktop&scale=100#home`), so each capture is reproducible. Data loads from the live seeded SQLite DB.

Verification: all 30 PNGs were checked by reading the PNG IHDR (`width`, `height`), and SHA-256 hashes were compared. Result: **30 files / 30 unique hashes**. Any capture sharing the exact configuration and page state with another was omitted rather than saved as a duplicate; e.g. `home.png` is the en / DPI 100 / 1280×720 dark Home capture; the separate 1280×720 viewport capture uses the Downloads page to avoid identical bytes.

| File | Dimensions | Theme | View / profile | Description |
|---|---|---|---|---|
| `accounts.png` | 1280×720 | Dark | Accounts / Desktop | Explicit unconfigured-authentication state. |
| `channels.png` | 1280×720 | Dark | Channels / Desktop | Channel catalogue. |
| `downloads.png` | 1280×720 | Dark | Downloads / Desktop | 50 queue fixtures and state labels. |
| `dpi-150.png` | 1280×720 | Dark | Home / Desktop | 1.5× DPI. |
| `dpi-200.png` | 1280×720 | Dark | Home / Desktop | 2× DPI. |
| `dpi-300.png` | 1280×720 | Dark | Home / Desktop | 3× DPI. |
| `history.png` | 1280×720 | Dark | History / Desktop | Per-video completion percentage, watched/total seconds and resume-enabled play button. |
| `home.png` | 1280×720 | Dark | Home / Desktop | Six home sections with live seeded data. |
| `library.png` | 1280×720 | Dark | Library / Desktop | Offline library empty state (fixtures have no files). |
| `locale-ja.png` | 1280×720 | Dark | Home / Desktop | Japanese UI. |
| `locale-zh-CN.png` | 1280×720 | Dark | Home / Desktop | Simplified Chinese UI. |
| `playlists.png` | 1280×720 | Dark | Playlists / Desktop | 20 synced playlists, create form and clickable detail cards. |
| `profile-desktop.png` | 480×320 | Dark | Watch later / Desktop | Desktop profile at 480×320. |
| `profile-handheld.png` | 480×320 | Dark | Home / Handheld | Handheld density/navigation profile. |
| `profile-tv.png` | 480×320 | Dark | Home / TV | TV text/nav scaling. |
| `search.png` | 1280×720 | Dark | Search / Desktop | FTS result for “Demo Video 0001”. |
| `settings.png` | 1280×720 | Dark | Settings / Desktop | Theme/locale/profile/DPI plus media & storage settings and tool probes. |
| `subscriptions.png` | 1280×720 | Dark | Subscriptions / Desktop | Seeded channel subscriptions. |
| `theme-dark.png` | 1280×720 | Dark | Video detail / Desktop | Distinct dark-theme capture. |
| `theme-light.png` | 1280×720 | Light | Home / Desktop | Light palette. |
| `theme-oled.png` | 1280×720 | OLED/AMOLED | Home / Desktop | Background sampled as pure #000000. |
| `video-detail.png` | 1280×720 | Dark | Video detail / Desktop | Metadata and watch/download/favorite actions. |
| `viewport-1280x720.png` | 1280×720 | Dark | Downloads / Desktop | 1280×720 layout. |
| `viewport-1920x1080.png` | 1920×1080 | Dark | Home / Desktop | 1920×1080 layout. |
| `viewport-320x240.png` | 320×240 | Dark | Home / Desktop | 320×240 responsive layout. |
| `viewport-480x320.png` | 480×320 | Dark | Home / Desktop | 480×320 responsive layout. |
| `viewport-640x480.png` | 640×480 | Dark | Home / Desktop | Mid-size viewport from the required matrix. |
| `profile-dap.png` | 480×320 | Dark | Home / DAP | DAP device profile: 200px rail, denser 16:10 cards, compact rows. |
| `viewport-720x720.png` | 720×720 | Dark | Home / Desktop | 720×720 layout. |
| `watch-later.png` | 1280×720 | Dark | Watch later / Desktop | Seeded watch-later entries. |
