# CFB27 Multi-User Online Dynasty Design

**Date:** 2026-08-07
**Status:** Draft for review

## Goal

Let several people, each running their own copy of CFB27 on their own PC, join
one Online Dynasty hosted on a Cypress private server — each controlling a
different school, with the league state shared and persisted on the host.

## What already exists

The transport problem is largely solved. The portable VPN release
(`tools/cypress-servers/release/`) already:

- binds the Blaze listener to a configurable private-VPN address
  (`vpnBindAddress`, default `100.64.0.1`) while keeping Dynasty and
  diagnostics on loopback;
- installs a firewall rule scoped to the VPN subnet;
- ships a client package whose `Setup-Client.ps1` takes a `-ServerAddress`, so
  a player can point their game at the host;
- validates the player's game build by SHA-256 against a compatibility manifest.

So "reach the host over the network" is designed, built, and packaged. The
older `deploy/start-cfb27-private-host.ps1` is a single-machine development
script and is not the path for this.

## What blocks multiple players

These are properties of the current server, verified in the code.

### 1. The server knows exactly one player

`internal/cfb27blaze/handlers.go` defines identity as package constants:
`LocalBlazeID = 1001`, `localPersonaID = 1001`, `localNucleusAccountID = 1001`,
`localSessionKey = "cypress-local-session-key"`. `handleLocalLogin` returns
that same identity to every connection, named from the server-wide
`-profile` flag. Two players connecting are indistinguishable — same Blaze ID,
same persona, same session key, same display name.

The HTTP/2 Nexus token stubs (`granttokenbyauthorizationcode`, `gettokeninfo`)
likewise mint one fixed identity with `pid`/`uid` `1001`.

### 2. Dynasty state is per-server, not per-player

Every piece of mutable Dynasty state lives on the `Service` struct and is
shared by all connections:

| Field | Consequence with two players |
|---|---|
| `selectedTeam atomic.Int64` | The second player's team choice overwrites the first. Both players' payloads are then localized to the last team picked. |
| `activeDynastySession atomic.Int64` | One dynasty for everyone; a second player cannot be in a different league. |
| `dynastyContract`, `dynastyHub`, `dynastyAdvance atomic.Uint32` | Occurrence counters that select which captured notification batch to send. Two players interleave them, so player B's first hub visit receives player A's second-visit notifications. |

This is the deepest change. Making the server multi-user means moving this
state onto a per-connection session keyed by the authenticated player.

### 3. There is no join path

The Dynasty service already has `users`, `teams`, and `max_users` in SQLite,
with `POST /sessions/{id}/users` and `POST /sessions/{id}/teams` endpoints. The
Blaze layer never calls them. Its only outbound calls are `EnsureSeeded`,
`CreateSession`, `SelectTeam`, and `AdvanceSession`. Those tables are unused
scaffolding, not a working membership model.

`EnsureSeeded` also takes `sessions[0]` unconditionally, so even the concept of
"which dynasty am I joining" does not exist yet.

### 4. The join command set is unknown — this is the gate

Every capture in the repository is a single player creating a dynasty. The
July 22 live capture contains only BootStatus commands 1, 2, and 110 (boot and
menu); the Dynasty fixtures come from a single-player create-and-play session.

**Nothing in the repository shows what the client sends to list, be invited to,
or join an existing Online Dynasty.** Those commands cannot be implemented from
what we have. Until that command set is known, the rest of this design cannot
be finished — it would be guesswork encoded as protocol.

### 5. The league data is still a replayed capture

Covered separately in `2026-08-07-cfb27-artifact-derived-dynasty-design.md`.
It matters here because per-player views (my team, my roster, my recruits)
cannot come from a single captured Akron dynasty patched for one team at a
time.

## Architecture

### Phase 1 — Establish the join command set (blocking)

No other phase can be completed first. Options, in order of preference:

1. **Capture it.** If any environment can still produce a real client
   performing a join, record it with the existing gateway and extract the
   ordered command sequence. This is the only approach that yields ground
   truth.
2. **Derive it from the client.** Locate the Dynasty component's command
   dispatch table in `CollegeFB27.exe` and enumerate the handlers around the
   known create path (BootStatus 101) — the same technique already used to find
   the ProtoSSL certificate decision. Yields command IDs and rough shapes, not
   field semantics.
3. **Probe it.** With a working single-player dynasty, drive the client's
   Online Dynasty menus and record which unimplemented commands it issues.
   `HandleFrame` already logs every unknown route with its decoded payload, so
   the server tells us what it was asked for. This is cheap and should be done
   regardless.

Deliverable: a documented command list for list/invite/join/leave with request
field shapes. Everything below assumes it exists.

### Phase 2 — Per-connection identity

Replace the identity constants with a resolved player record:

- The client package's setup already collects a per-player value
  (`-ServerAddress`); extend it to carry a player name and a stable local
  player ID.
- The bridge passes that identity to the server on connect. The Nexus token
  stubs mint a per-player `pid`/`uid` instead of `1001`, and `handleLocalLogin`
  echoes the resolved identity.
- The server assigns Blaze IDs from a registry backed by the Dynasty service's
  `users` table, so a returning player keeps the same ID.

Authentication here is trust-on-first-use inside a private VPN, not a security
boundary. It must not be described as one.

### Phase 3 — Per-player session state

Introduce a `playerSession` struct holding what is currently on `Service`:
selected team, active dynasty, and the per-screen occurrence counters. Key it
by Blaze ID, look it up per frame in `HandleFrame`, and keep only genuinely
global things (event log, catalog, TLS config) on `Service`.

The notification-occurrence counters must move with it, otherwise players
corrupt each other's notification sequencing.

### Phase 4 — Membership and team ownership

Wire the existing REST endpoints into the Blaze layer:

- Joining calls `POST /sessions/{id}/users` and records the player.
- Team selection calls `POST /sessions/{id}/teams` and is rejected when the
  team is already owned by another user in that session — currently nothing
  prevents two players from taking the same school.
- `max_users` is enforced at join.
- Team ownership is written into the artifact, extending the existing
  `select-team` mutation, which today assumes exactly one `FranchiseUser`
  record and one user-controlled coach. Supporting N users means populating N
  `FranchiseUser` records rather than the `records.find(first non-empty)` it
  does now.

### Phase 5 — Advance coordination

`handleDynastyAdvance` currently advances the league whenever any client asks,
guarded only by an idempotency key. With several users it needs a real rule:
advance when every user has marked ready, or when the commissioner forces it.
The `advance_requests` table and the `is_admin` column already exist to build
this on. Players who have not advanced need to be told the week moved, which
depends on Phase 6.

### Phase 6 — Notification fan-out

Notifications are currently written only to the connection that triggered the
request, replaying a captured batch. A shared league needs the server to push
state changes to other connected players. `Service.connections` already tracks
live connections, and `blaze.WriteFrame` can target any of them; what is
missing is a per-player outbound queue and knowledge of which notifications are
league-wide versus personal.

## Security boundary

The 2026-08-04 design set a strict rule: loopback only, every non-loopback
connection rejected, zero permitted external connections proven per run. The
VPN release already relaxes this deliberately — Blaze binds to a VPN address
with a subnet-scoped firewall rule.

That relaxation should be stated plainly rather than left implicit:

- The bridge's outbound blocking still protects players from reaching EA. That
  property must not be weakened; it is what keeps a private session private.
- The server's inbound exposure is now the VPN subnet. Anyone on that VPN can
  connect and, with trust-on-first-use identity, claim any player name.
- The server must never be bound to a public interface. Setup should refuse a
  bind address that is not private, and say why.

## Failure handling

- An unknown command stays an explicit error. The current logging of unknown
  routes with decoded payloads is what makes Phase 1 option 3 possible;
  blanket success would destroy it.
- A join that exceeds `max_users`, or requests an owned team, returns a
  structured refusal — not a silent success that desyncs the artifact.
- Losing a player's connection mid-advance must not leave the artifact
  half-mutated; the existing atomic swap plus rollback already covers the file,
  but the readiness state needs the same treatment.

## Testing

- Two synthetic Blaze clients connecting concurrently receive distinct Blaze
  IDs and distinct selected teams, and neither observes the other's team.
- Notification occurrence counters advance independently per player.
- Team selection conflict returns a refusal and leaves the artifact unchanged.
- Advance requires all users ready; a forced advance by a non-commissioner is
  refused.
- A player reconnecting keeps their identity, team, and dynasty.

## Scope boundaries and risk

The honest risk: **Phase 1 may not be solvable from what is available.** If the
join command set cannot be captured or derived, multi-user joining cannot be
implemented faithfully, and the realistic ceiling is several people taking
turns against one hosted league rather than a true simultaneous Online Dynasty.
That should be established before any of Phases 2–6 are built, because all of
them are wasted effort if the client cannot be made to issue a join.

This design does not cover matchmaking, playing games against each other, or
any realtime gameplay traffic — only the Dynasty league service.
