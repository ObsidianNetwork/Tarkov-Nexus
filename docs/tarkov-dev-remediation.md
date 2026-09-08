# tarkov.dev socket remediation — evidence

Validation evidence for Linear **TAR-2** / **TAR-5**, and for Razzmatazzz's three
conditions for unblocking Tarkov Nexus:

> if you make it open source, fix the problems causing the errors, and have the
> app more clearly identify itself to the socket server (e.g., setting the
> `origin` header to something like "TarkovNexus"), then I can unblock it

## Source of truth

The socket server is open source: **[the-hideout/tarkov-socket-server](https://github.com/the-hideout/tarkov-socket-server)**,
`server.mjs` on branch `master`. Every claim below is read from that file, not
inferred from the website client.

The server accepts exactly three message types:

```js
if (message.type === 'pong')    { ws.isAlive = true; return; }
if (message.type === 'command') { sendMessage(sessionID, 'command', message.data); return; }
if (message.type === 'debug')   { sendMessage(sessionID, 'debug',   message.data); return; }

console.warn(`Unrecognized message type ${ws.sessionID} ${ws.remoteAddress}: ${stringMessage}`);
```

## The exact failing messages

Tarkov Nexus invented a fourth message type, `party`, to carry squad member
positions. The server has no such type, so **every one of these messages fell
through to the `Unrecognized message type` warning** and was logged in full.

Affected: all released versions up to and including **3.3.3**.

Sent by `SendPartyPositionUpdate` — once per party member, per position update,
for the whole raid:

```json
{"sessionID":"AB12","type":"party","data":{"type":"partyPosition","remoteId":"CD34","displayName":"Riley","map":"customs","position":{"x":-14.2,"y":2.6,"z":118.9},"rotation":183.4}}
```

Sent by `SendPartyMemberLeft` — once per member departure:

```json
{"sessionID":"AB12","type":"party","data":{"type":"partyMemberLeft","remoteId":"CD34"}}
```

Both produced one `console.warn` per message, each echoing the full message
body. With a squad of three or four moving through a raid this is a sustained
stream of warnings — which matches the reported symptom precisely.

**Nothing consumed these messages.** The server relays only `command` and
`debug`; `party` was dropped after being logged. On our side, squad markers are
drawn from the *local* overlay server on `ws://localhost:44444` — every consumer
of `partyPosition` / `partyMemberLeft` lives in `internal/overlay/`. The
tarkov.dev sends were pure waste in both directions.

## Why the source was unidentifiable

```js
console.info(`Client connected ${ws.sessionID} from ${req.headers.origin}`);
```

The Go client dialled with `websocket.DefaultDialer.Dial(u.String(), nil)` — no
headers. Go sends no `Origin` by default, so every Tarkov Nexus connection
logged `from undefined`, and the warnings above carried only a session ID and an
IP. That is exactly why the traffic could not be attributed to an application.

The server already has a switch for this:

```js
if (process.env.REQUIRE_ORIGIN === 'true' && !req.headers.origin) {
    ws.terminate();
    return;
}
```

An origin-less client is terminated at connect time when `REQUIRE_ORIGIN` is on.
This is a plausible mechanism for the block itself, and it means setting the
header is not merely cosmetic identification — it may be the difference between
connecting and being dropped.

## Forensic confirmation from their commit history

The socket server's own git history corroborates every claim above. Relevant
commits, all in `the-hideout/tarkov-socket-server`:

| Date | Commit | What it did |
|---|---|---|
| 2026-08-12 | `b5cba5f` | Started recording `remoteAddress` per client and logging it on terminate. |
| 2026-08-12 | `dc58ae0` | **Added `console.warn(\`Unrecognized message type …\`)` — it did not exist before.** Also wrapped `JSON.parse` in try/catch. |
| 2026-08-12 | `475473d`, `2fddec2` | Added `x-forwarded-for` resolution, then **added `from ${req.headers.origin}` to the connect log.** |
| 2026-08-17 | `f4e0818` | Switched to `cf-connecting-ip`, added a client `error` handler, added `nativePing`, and **wrote an origin-less kill switch, left commented out.** |
| 2026-08-17 | `436fe62` | **Activated that kill switch behind `REQUIRE_ORIGIN`.** |
| 2026-08-19 | — | TAR-2 filed: "we took steps to block them." |
| 2026-08-21 | — | Razzmatazzz: "we won't be unblocking because…" |
| 2026-08-25 | `cd64d07` … `27ff5c2` | Session-index performance work, then rolled back. Unrelated. |

Read as a sequence, 12–17 August is a forensic hunt: add per-client IP, add an
origin to the connect log, guard the JSON parse, add an error handler, and add a
warning for message types the server does not recognise. Then, two days before
the block was reported, ship a switch that terminates clients with no origin.

Two conclusions follow.

**The `Unrecognized message type` warning was added on 12 August.** Before that
commit, an unknown message type fell through silently. `type:"party"` is the
only message Tarkov Nexus sent that could reach that line, and the timing fits —
but the history shows *when* the warning appeared, not which client prompted it.
Treat this as strong correlation plus a confirmed defect on our side, not as
proof that our traffic was the sole cause.

**The origin logging was added the same day, for the same reason.** For every
Tarkov Nexus connection it printed `from undefined`. That is the whole of "we
couldn't readily identify the source of these clients," in one line of code.

### What this does *not* prove

`REQUIRE_ORIGIN` is a plausible block mechanism but is not confirmed to be the
live one. TarkovMonitor sets no `Origin` header either — verified in
`research/TarkovMonitor/TarkovMonitor/SocketClient.cs`, which constructs the
client with a bare URI and no headers. Enabling `REQUIRE_ORIGIN` globally would
therefore terminate the-hideout's own flagship tool, so it is unlikely to be on
for everyone.

The more probable live block is IP-level at Cloudflare — the server reads
`cf-connecting-ip`, so it sits behind it, and "we took steps to block them" fits
a WAF rule better than an env var.

**Practical consequence: shipping these fixes will not restore access by
itself.** If the block is IP-based, Razzmatazzz has to lift it, which matches
his wording — "then I *can* unblock it." The fixes satisfy his conditions; the
unblocking is a manual step on their side.

### The website was never touched

`the-hideout/tarkov-dev` has no block-, socket-, or abuse-related commit between
2026-07-20 and 2026-08-31, the date of this audit — only dependency bumps, a PvP
season feature, a new map, and translations. That is negative evidence from one repository's
history: it is consistent with the proxy not being what they reacted to, but it
cannot rule out a WAF rule, a deployment change, or any action taken outside
version control.

## Second audit — invalid map names (found after the first pass)

A re-read of our code against `research/tarkov-dev/src/data/maps.json` turned up
a second class of bad traffic that the message-type work did not cover.

tarkov.dev's `map` command navigates the paired browser to `/map/<value>`, and
a `playerPosition` is only drawn when its `map` equals the key of the map being
viewed (`src/pages/map/index.js:1963`). For the interactive projection that key
is the `normalizedName`. So an unrecognised name both lands the user on a 404 on
their site and silently prevents the position marker from rendering.

This repo has **two** map resolvers, and they disagreed with each other. Both
emit values that reach the wire:

| Resolver | Bad output | Reaches tarkov.dev via | Fixed to |
|---|---|---|---|
| `internal/game/map_resolver.go` | `labs` | filename-derived map, manual override | `the-lab` |
| `internal/maps/resolver.go` | `night-factory` | `RaidInfo.NormalizedMap` → `SendMapChange` on raid start | `factory` |

`night-factory` was sent on **every night Factory raid**, which makes it the more
serious of the two.

**Precision on `night-factory`.** It is not simply absent — `factory` carries
`altMaps: ["night-factory"]`, and `src/features/maps/index.js:169` expands an
alt into a real route key, but *only* when tarkov.dev's GraphQL API also returns
a map with that normalizedName:

```js
const altApiMap = maps.find(map => map.normalizedName === altKey);
if (!altApiMap) {
    // alt map is missing; so we skip it
    continue;
}
```

So `night-factory` is a conditional alias whose validity depends on their live
API, which currently answers our requests with HTTP 422
("GraphQL server unavailable"), so it could not be confirmed either way.

That uncertainty is exactly why `factory` is the right value to send. It is a
canonical normalizedName *and* an interactive `map.key`, so it is correct in
both worlds. If an alias does not resolve, `allMaps[currentMap]` is `undefined`
and their page renders its error view rather than a map. Sending an unverifiable
alias gives the user a broken map view, which is worse than landing on a known
route.

`labs` has no such defence: it appears nowhere in their data, as a
normalizedName, a map key, or an alt.

Their canonical set is streets-of-tarkov, ground-zero, customs, factory,
interchange, the-lab, the-labyrinth, lighthouse, reserve, shoreline, woods,
openworld. `knownMaps` deliberately holds only these — aliases are excluded
because their resolution is conditional on an API we cannot check.

`openworld` is a special case: it is a canonical group name but has no
interactive projection, so it is navigable while never matching a
`playerPosition`. Neither resolver emits it, so it is inert in practice; it is
kept in the list only to mirror their canonical set.

### Guard

Fixing the data alone would leave the same failure one typo away, so the
allowlist now lives in `internal/websocket/client.go` as `knownMaps`, checked
before anything goes on the wire. It fails closed and logs a warning naming the
rejected map, so a stale list is diagnosable from the in-app log rather than
becoming invisible breakage.

`TestKnownMapsMatchesTarkovDev` compares that list against the vendored
`maps.json` in both directions. Note its limit: `research/` is excluded from
publication, so in the published tree the fixture is absent and the test skips.
It guards drift here, not there.

We never emit `the-labyrinth` or `openworld`; neither resolver has an entry for
them. That fails closed — the update is dropped rather than mis-sent — so it is
a gap in coverage, not a source of bad traffic.

### 2D / 3D projections — not achievable

Position markers cannot be made to work on the 2D and 3D map projections. The
effect that draws them opens with a hard guard
(`src/pages/map/index.js:1951`):

```js
useEffect(() => {
    if (!mapData || mapData.projection !== 'interactive') {
        return;
    }
    ...
    if (playerPosition && (playerPosition.map === mapData.key || playerPosition.map === null)) {
```

Non-interactive projections are static images rendered by a different component,
with the Leaflet container set to `display: none` (line 2044). No value we send
changes this — including `map: null`, which their code accepts as a wildcard but
which never reaches evaluation on a non-interactive projection.

This is a limit of what tarkov.dev implements, not of our client, and
TarkovMonitor is subject to the same one. Nothing to do.

### Also fixed

- **Session ID was not URL-escaped.** `sessionid=%s-tm` was built with
  `fmt.Sprintf` while tarkov.dev's own client uses `encodeURIComponent`. The
  Remote ID comes from a free-text config field, so an `&` or `#` would
  truncate the query — the server would then read a different session, or none,
  in which case it terminates the connection. Now `url.QueryEscape`.

## Corrections to earlier analysis in this repo

Recorded so nobody re-litigates settled points:

- **The `pong` message was never a problem.** Tarkov Nexus sent
  `{"sessionID":"…","type":"pong"}` where the website sends a bare
  `{"type":"pong"}`. The server checks `message.type === 'pong'` *before* it
  looks at `sessionID`, so the extra field was ignored. It has been made bare
  anyway for protocol fidelity, but it caused no server error.
- **A stale or wrong Remote ID causes no server error.** `sendMessage` iterates
  `wss.clients` and sends only to matching sessions; zero matches is a silent
  no-op, not a throw. Sending position updates with no browser paired is
  harmless to the server.
- **The tarkov.dev reverse proxy did not cause these errors.** It talks to the
  website, not the socket server, and cannot reach the code above. It remains a
  real problem for other reasons (see below), but it is not what Razzmatazzz
  reported and is not required for unblocking.

## Changes made

In `internal/websocket/client.go` unless noted.

| Change | Why |
|---|---|
| `Origin: TarkovNexus` on the handshake | Condition 3. Makes every connection attributable in their logs; also satisfies `REQUIRE_ORIGIN`. |
| `User-Agent: TarkovNexus/<version>` | Identifies the client version alongside the origin. |
| Removed `SendPartyPositionUpdate` / `SendPartyMemberLeft` | Condition 2 — these were the messages producing the warnings. Also removed from `internal/integration/integration.go` and `ui-wails/app_party_central.go`. |
| Party member departures now broadcast to the local overlay | Previously only sent to tarkov.dev, so the local marker never cleared. Fixing the first bug exposed and fixed this one. |
| Bare `{"type":"pong"}` | Protocol fidelity; matches the website client. |
| Reject empty map names before sending | Prevents a `command` with a blank value reaching a paired browser. |
| Capped exponential reconnect backoff | A rejected client previously retried at a fixed 5s until `maxReconnectAttempts`; the cap spreads those attempts out instead of hammering a server that is already refusing us. |
| Dial errors now report HTTP status | Distinguishes a 403 block from a network failure. |

### Note on the ping mechanism

The server chooses its heartbeat based on the origin:

```js
if (!req.headers.origin?.startsWith('http')) {
    ws.nativePing = true;
}
```

`Origin: TarkovNexus` does not start with `http`, so the server uses
**protocol-level ping frames**. `gorilla/websocket` answers those automatically
from its default ping handler while a read is in flight, and our read loop is
always in a `ReadJSON`, so the heartbeat is satisfied. The JSON-`ping` branch in
`handleMessage` remains correct for the other case and is harmless.

## Still outstanding

- `go build ./...`, `go vet ./...` and `go test ./...` all pass against the
  reviewed commit, including the socket wire-contract tests in
  `internal/websocket`. The livenet proxy test is run separately with
  `go test -tags livenet ./internal/overlay/ -run TestLiveProxy`.
- **Live wire validation is still outstanding, and TAR-2 should not be closed
  without it.** Once access is restored, run one full raid with a squad and
  record on the issue: the connection is accepted and held for the raid; their
  log shows `Client connected <id> from TarkovNexus` rather than `from
  undefined`; no `Unrecognized message type` warning appears for the session;
  and map navigation plus the position marker both work end to end.
- Condition 1 (open source) is tracked separately.

## Separate concern — not part of unblocking

*As audited*, `internal/overlay/server.go` reverse-proxied the entire tarkov.dev
website through `localhost:44444`, stripping `X-Frame-Options`,
`Content-Security-Policy` and `X-Content-Type-Options`, while
`internal/overlay/proxyinject.go` auto-accepted their cookie consent, hid their
navigation and branding, unchecked their map filters, overrode their saved map
settings and replaced `window.L.Map`.

*Now*: only `X-Frame-Options` is removed — their CSP and `X-Content-Type-Options`
pass through unmodified, as does their CORS policy. The injected script no longer
touches consent, branding, filters or their stored settings, and captures the map
through `L.Map.addInitHook` with a self-restoring `addTo` fallback. Requests
carry `X-Tarkov-Nexus-Version` and a truthful referer.

Razzmatazzz never raised any of this and it was not a condition of unblocking.
What remains is inherent to the design: the map window still loads the site
through our server and still removes its framing protection. Tracked in
`docs/plans/map-window-redesign/`.
