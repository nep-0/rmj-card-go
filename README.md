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

Enable the gateway bot with `QQ_BOT_ENABLED=true`, `QQ_BOT_APPID`, and `QQ_BOT_SECRET`. Group commands include `/help`, `/bind 玩家名`, `/card [玩家名]`, `/stats [玩家名]`, `/good [玩家名]`, `/bad [玩家名]`, `/add 场所ID`, `/start`, `/end`, and `/admin 用户ID`. Bindings remain in memory; match-report groups, administrators, running state, and deduplication IDs persist to JSON.
`/add` configures match reporting for the current group and grants the caller admin access. Group admins use `/start` to establish a baseline and begin reports, `/end` to stop, and `/admin 用户ID` to grant another admin.
The `QQ_BOT_PUBLIC_BASE_URL` value is used to build the `/card` response link; the chosen PNG is served by the HTTP API.
- `QQ_BOT_PUBLIC_BASE_URL`: public HTTPS origin serving this service; required by `/card`.
- `QQ_BOT_STYLE`: `modern` (default) or `QH`.
- `QQ_BOT_COMMAND_PREFIX`: command prefix (default `/`).
- `QQ_BOT_MATCH_REPORTS_FILE`: registry JSON path (default `data/qq-match-reports.json`).
- `QQ_BOT_MATCH_REPORT_INTERVAL_MS`: match polling interval (default `30000`).

## Checks

```sh
go test ./...
go vet ./...
```
