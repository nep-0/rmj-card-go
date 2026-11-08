# RMJ Card Go Service

Go service and QQ official bot for Formula Mahjong statistics, using `github.com/nep-0/rmj`. The Formula summary card here is separate from the `rmj` module's playercard feature.

The `modern` card shows a recent-placement trend; `QH` shows placement and point tiles. Both use the same Formula identity, rank, promotion, radar, and placement-distribution data.
The identity panel uses the newest non-empty club name from the player's latest match-history page; `雀庄未知` is shown only when no club name is available.

Recent results prefer the latest 50 match records, sorted by match time; aggregate recent history remains the fallback when records are unavailable or contain no matching player. Display order is oldest to newest. Five-game summary groups show placement sequences, and promotion projections share the chart's bounded time axis.
Each projection connects the latest placement to the required average at the next game, then stays horizontal through its forecast window, matching the original card.
The history and forecast plot shares a fixed, horizontally centered 850-pixel-wide area; forecast length changes the time scale, not the plot alignment.

## Run

```sh
go run .
```

The HTTP service listens on `:3000` by default. Set `PORT` to change the port. Formula API access is performed by the `rmj` client.

PNG rendering is pure Go: `oksvg`/`rasterx` draw shapes, and `golang.org/x/image/font` draws text using embedded Noto Sans SC regular and bold fonts. No browser, external executable, system fonts, or CGO is required. Font licensing is included in `fonts/OFL.txt`. SVG output remains available.

Cards fetch the QQ avatar when Formula supplies a valid QQ number. The image is center-cropped into the circular identity slot and embedded in SVG/PNG. Missing QQ numbers, unavailable avatars, or invalid image responses retain the placeholder; avatar requests have a five-second timeout and a 2 MB response limit.
PNG card responses are cached for five minutes, with a maximum of 512 images and 64 MiB; simultaneous requests for the same render share one generation.

## Container

Build and run locally:

```sh
docker build -t rmj-card .
docker run --rm -p 3000:3000 -v rmj-card-data:/app/data rmj-card
```

The `/app/data` volume stores persistent QQ match-report configuration. Mount a named volume or host directory there when deploying the container.
The GitHub Actions workflow publishes `ghcr.io/<owner>/<repository>` on pushes to `main` and version tags; pull requests build without publishing.

## HTTP API

- `GET /health` returns `{"status":"ok"}`.
- `GET /api/player-cards/{name}.png` returns a PNG card. `?style=modern` is the default; `?style=QH` selects QH.
- `GET /api/player-cards/{name}.svg` returns the SVG card.
- `GET /api/player-stats/{name}.txt` returns aggregate statistics.
- `GET /api/player-opponents/good/{name}.txt` and `/api/player-opponents/bad/{name}.txt` return opponent rankings.

## QQ official bot

Enable the gateway bot with `QQ_BOT_ENABLED=true`, `QQ_BOT_APPID`, and `QQ_BOT_SECRET`. Group commands include `/help`, `/bind 玩家名`, `/card [玩家名]`, `/stats [玩家名]`, `/good [玩家名]`, `/bad [玩家名]`, `/add 场所ID`, `/start`, `/end`, and `/admin 用户ID`.
QQ bindings persist as the upstream-compatible `data/qq-name-cache.json` flat QQ-ID-to-player-name object. Match-report groups, administrators, running state, and deduplication IDs use the upstream-compatible `data/qq-match-reports.json` flat group-ID-to-state object. Existing files at those paths can be reused directly; mount `/app/data` when using the container.
The `QQ_BOT_PUBLIC_BASE_URL` value is used to build the `/card` response link; the chosen PNG is served by the HTTP API.
- `QQ_BOT_PUBLIC_BASE_URL`: public HTTPS origin serving this service; required by `/card`.
- `QQ_BOT_STYLE`: `modern` (default) or `QH`.
- `QQ_BOT_COMMAND_PREFIX`: command prefix (default `/`).
- `QQ_BOT_MATCH_REPORTS_FILE`: registry JSON path (default `data/qq-match-reports.json`).
- `QQ_NAME_CACHE_FILE`: QQ name cache path (default `data/qq-name-cache.json`).

## Environment

Copy `.env.example` and set the QQ credentials and public HTTPS origin before enabling the bot. `HOST` defaults to an empty host, which binds all interfaces, and `PORT` defaults to `3000`. `DISABLE_SVG` accepts `1`, `true`, or `yes` (case-insensitive) to disable SVG responses.

`QQ_BOT_PANEL_GROUPS_FILE` stores the JSON array of groups whose command panel is enabled (default `data/qq-panel-groups.json`). `/add` registers each configured group; startup synchronizes the shared QQ command panel for those groups when `QQ_BOT_PANEL_ENABLED=true` (default). Set it to `false` to disable synchronization. `QQ_BOT_CARD_CACHE_MAX_ENTRIES` and `QQ_BOT_CARD_CACHE_MAX_BYTES` set PNG cache limits (defaults `512` and `67108864`); invalid or nonpositive values use defaults.

Panel synchronization reuses an existing matching panel, preserves its API version, and adds persisted group targets; otherwise it creates a group-specific panel. Startup stops if enabled synchronization fails. Group registration remains persisted after a synchronization failure so startup can retry.

## Checks

```sh
go test ./...
go vet ./...
```
