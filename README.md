# Star Defender

A side-scrolling shoot-'em-up for the browser, in the spirit of the early-80s
arcade defend-the-planet games. Fly over a wrapping planet surface, shoot down
the invaders and stop them carrying off your colonists.

Everything is written in Go: the game is compiled to WebAssembly and draws on
a `<canvas>`, and a small Go web server hosts it and stores high scores and
user-made levels. There are no third-party dependencies.

- 15 campaign levels with eight enemy types and three boss fights
- Synthesised sound effects and music (Web Audio, no sample files)
- Server-side high-score table
- Level editor with test play; saved levels are playable by everyone on the server

## Running

Requires Go 1.24 or newer.

```bash
make run
```

Then open <http://localhost:8080>. To use another port:

```bash
make run PORT=9000
```

Without `make`:

```bash
GOOS=js GOARCH=wasm go build -o web/game.wasm ./cmd/client
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/
go run ./cmd/server -port 9000
```

The port is taken from `-port`, else the `PORT` environment variable, else
8080. Use `-addr 127.0.0.1:9000` to also choose the interface. Other server
flags: `-web web` (static files) and `-data data` (where `scores.json` and
`levels.json` are kept).

## Playing

| Key | Action |
| --- | --- |
| Arrows / WASD | Fly (left/right also turns the ship) |
| Space / Z | Fire |
| B / X / Shift | Smart bomb - destroys everything on screen |
| H / C | Warp to a random spot |
| P | Pause |
| M / N | Mute everything / music only |
| Esc | Quit to the menu |

**Snatchers** dive for colonists and carry them skywards. Shoot the snatcher,
catch the falling colonist and set them down again for bonus points. A
colonist who reaches the top of the screen is lost and the snatcher mutates
into a **reaver**. Lose every colonist and the planet is destroyed, turning
all remaining snatchers into reavers.

Other enemies: **bombers** lay mines, **hives** burst into **wasps**,
**stalkers** strafe and snipe (and come looking for you if you dawdle),
**turrets** shoot from the ground and **dreadnoughts** are the bosses.

A level ends when every enemy, including reinforcement waves, is destroyed.
Surviving colonists are worth a bonus, and every 10,000 points earns an extra
ship and smart bomb. The radar at the top shows the whole planet.

## Level editor

Choose **LEVEL EDITOR** on the title screen.

- **Terrain** - drag in the level to sculpt the ground
- **Colonist** and the enemy tools - click to place; right-click (or the
  **Erase** tool) removes
- **Waves** - reinforcements that warp in after a number of seconds
- Width (in screens), enemy speed, fire rate and terrain colour are set at the top
- **Test play** runs the level immediately; Esc returns to the editor
- **Save** stores the level on the server. It then appears under
  **SINGLE LEVEL** on the title screen. Campaign levels can be loaded as
  templates; saving one creates your own copy.

## Layout

| Path | Contents |
| --- | --- |
| `cmd/server` | Web server: static files plus the JSON API |
| `cmd/client` | The game (WebAssembly): rendering, input, audio, editor |
| `internal/game` | Platform-independent game simulation |
| `internal/level` | Level format, validation and the 15 built-in levels |
| `web` | HTML and CSS; the build puts `game.wasm` and `wasm_exec.js` here |

### API

| Method | Path | |
| --- | --- | --- |
| GET | `/api/scores?limit=10` | Top scores |
| POST | `/api/scores` | Submit `{name, score, level}` |
| GET | `/api/levels` | List built-in and custom levels |
| GET | `/api/levels/{id}` | One level |
| POST | `/api/levels` | Create or update a custom level |
| DELETE | `/api/levels/{id}` | Delete a custom level |

The API has no accounts or authentication: anyone who can reach the server can
submit scores and save, overwrite or delete custom levels. That suits a LAN or
a group of friends; put it behind something stronger before exposing it to the
internet.

## Tests

```bash
make test
```
