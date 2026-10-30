# POCKETJS VERIFICATION — pocket-youtube-video

Round: PocketJS refactor. Device acceptance is a later pass.

## Shape

- Pocket app: `apps/pocket-youtube-video/pocket/`
- Frozen React reference: `apps/pocket-youtube-video/frontend/` (design reference; not extended).
- TSX files in the port: **6** (`main.tsx`, `app.tsx`, `ui.tsx`, three screens).
- Native tree only: `View`/`Text` from `@pocketjs/framework/solid/components`; no DOM, iframe, texture screenshot, or `dangerouslySetInnerHTML`.

## Manifest

- id: `dev.pocket-youtube-video.pocket`
- entry: `src/main.tsx`
- output: `pocket-youtube-video`
- viewport: `360×240 @2x` → physical `720×480`
- capabilities: `text.glyphs.baked`, `input.buttons`
- target: `rocknix-h700-kms`

## Admission / compile

```sh
cd apps/pocket-youtube-video/pocket
./setup-sdk.sh
SDK=${POCKETJS_SDK:-$HOME/development/.pocketjs-star-drift-sdk}
bun "$SDK/tools/pocket.ts" check \
  --runtime-config "$POCKET_VERIFY/pjs/profiles/pocket.runtime-config.ts" \
  --target rocknix-h700-kms --manifest pocket.json
bun "$SDK/tools/pocket.ts" compile \
  --runtime-config "$POCKET_VERIFY/pjs/profiles/pocket.runtime-config.ts" \
  --target rocknix-h700-kms --manifest pocket.json --output out.pocket
```

Admission: `sha256:5b8aa7da08393f792f6fb584d36b45b8f64af23bf1bf9655e8b81e1fdbae056e`; TypeScript passes for the app module graph. Package: `out.pocket`, sha256 `e93a10ea587f0588b997febbe2501f0f520fba7c58fecfb2edd86f6a430e265b`.

## Frames

Rendered with the SDK wasm core at `renderScaled(2)` (720×480), not a browser:

```sh
cd "$POCKET_VERIFY"
bun tools/render-pocket.ts \
  --dir repos/pocket-youtube-video/apps/pocket-youtube-video/pocket \
  --script repos/pocket-youtube-video/apps/pocket-youtube-video/pocket/frames.json \
  --out repos/pocket-youtube-video/docs/pocket
```

| file | state | sha256 (first 16) |
|---|---|---|
| `docs/pocket/screen-home.png` | home | `8dddb0621b7371bc` |
| `docs/pocket/screen-home-focus.png` | home-focus | `f42477021e704734` |
| `docs/pocket/screen-detail.png` | detail | `55242df269577ce3` |
| `docs/pocket/screen-detail-action.png` | detail-action | `cbcb4ef908f1e9aa` |
| `docs/pocket/screen-settings.png` | settings | `9389ac8e1f1e4e6a` |
| `docs/pocket/screen-settings-toggled.png` | settings-toggled | `637092d22539c338` |

All six files have distinct bytes; the renderer rejects duplicate frames. The
tape is: move focus, open the item, run the first detail action, switch to
settings, move a row, toggle it.

Screens: Library/subscriptions, player detail, account settings. Interactions: PREV/PLAY/NEXT/SAVE change visible state; no media or account request.

## Adapter

- **fixture** — this port runs from JSON baked into the package and labels the
  header `fixture`. `client.ts` also contains the `http` and `bridge` adapters
  behind the same `list()/get()/action()` surface.
- `http` uses `@pocketjs/framework/net-api`; valid only in the host dev profile.
- `bridge` uses `host.__pvc(JSON.stringify(request))`; the runtime forwards to
  the Go daemon on `127.0.0.1:<port>` with a hard timeout.
- Device gap: videod bridge plus yt-dlp/mpv and OAuth. None are present on the device image.
- Daemon contract: /api/library, /api/channels, /api/downloads; auth=mock.

## Not done / open questions

- Device run, saves/annotations and the bridge itself are out of this round.
- Real media/audio, OAuth, emulator, engine and system writes stay unimplemented
  where the original product already marked them mock or unavailable.
- Long lists use the six-item fixture surface; paging/virtual lists are a
  follow-up once the bridge provides paginated data.
