# CFB27 Artifact-Derived Online Dynasty Design

**Date:** 2026-08-07
**Status:** Draft for review

## Goal

Make the private Online Dynasty server answer the client from the Dynasty save
artifact it created, instead of replaying a captured Akron dynasty with the
selected team's strings patched over it.

Concretely: starting a new Online Dynasty should produce a league whose team
list, coaching staff, roster, and season state are read from that session's own
FBCHUNKS artifact, and every mutation the client makes should land back in that
artifact on the host.

## Current Evidence

### What already works

- `dynasty.exe` owns one FBCHUNKS artifact per session under
  `%APPDATA%\Cypress\CFB27\Private\data\dynasties\{id}\DYNASTY-{id}`, swaps it
  atomically with rollback, and tracks `save_sha256` / `save_revision`.
- `cmd/cfb27franchise` can open, mutate, and re-save that artifact through
  `madden-franchise`, and round-trips select-team → advance → reopen under test.
- `cmd/cfb27assetexport` produces an authoritative catalog of 433 coaches,
  143 teams, and 11,730 players from `dynasty-dynasty-binary.FTC`.
- The FranTk 486 schema-binding regression is fixed (see
  `cmd/shared/frantk.mjs`); both tools now read the installed build's schema
  rather than a bundled snapshot.

### What does not

- Every screen the client renders comes from a captured `.bin` in
  `internal/cfb27blaze/fixtures/`, recorded from an Akron dynasty
  (`capturedDynastyTeamKey = 830865409`).
- `HandleFrame` serves those fixtures through three paths, all byte-level
  patches over captured bytes rather than encodes of live state:
  `localizeDynastyHub` (541), `selectedDynastyRosterPayload` (393), a verbatim
  copy (391), and `localizeSelectedTeam` for everything else.
- `localizeSelectedTeam` rewrites Akron's team name, nickname, coach names,
  asset names, and the encoded team key, then overlays scalar fields near
  string anchors. It is anchor-matching against a specific capture, so it
  degrades silently when a payload's shape differs from the recording.
- The created artifact is never read back to build a reply. Only three values
  round-trip from the client to the host: selected team key, current
  week/stage, and the artifact hash/revision.
- `initialize` is a clone of the host's newest `DYNASTY*` save with user
  bindings cleared. There is no from-scratch league generator, and there is no
  evidence one is reachable without the game's own generator.

### Fixture inventory

45 files, ~3.5 MB. Known purpose:

| Fixture | Route | Purpose | Serving path |
|---|---|---|---|
| `dynasty-533-form-133300355` | BootStatus/533 | Full team-selection database | verbatim |
| `dynasty-533-form-133300356` | BootStatus/533 | Team + coach detail database | verbatim; also the identity source for `dynastyTeamIdentityForKey` |
| `dynasty-533-form-133300354` | BootStatus/533 | Staff cards (HC/OC/DC) | `selectedDynastyStaffPayload` |
| `dynasty-1111-cvtp-2`, `-3` | BootStatus/1111 | Coach selection reply | `selectedDynastyCoachPayload` |
| `dynasty-391` | BootStatus/391 | League-wide team database | verbatim |
| `dynasty-393` | BootStatus/393 | Team roster | `selectedDynastyRosterPayload` |
| `dynasty-541` | BootStatus/541 | Dynasty hub | `localizeDynastyHub` |
| `dynasty-301`, `-531`, `-1261` | BootStatus/301, 531, 1261 | Settings payloads | `localizeSelectedTeam` |
| `dynasty-107-notifications-*` | BootStatus/107 | Advance notification batches | `localizeDynastyNotificationPayload` |
| `dynasty-304-notifications-*` | BootStatus/304 | Contract notification batch | as above |
| `dynasty-541-notifications-*` | BootStatus/541 | Hub notification batch | as above |

The remaining 26 progression payloads (161, 175, 177, 191, 193, 221, 223, 271,
275, 311, 313, 321, 323, 361, 363, 411, 413, 501, 561, 800/804, 1131, 1133,
1151, 1251, 1271, 1410) are registered in `capturedDynastyProgressionPayloads`
and served through `localizeSelectedTeam`. **Their semantics are not
established.** Four are large enough to be league-scale databases in their own
right — 275 (601 KB), 223 (557 KB), 1151 (188 KB), 393 (188 KB) — and the rest
range from 994 bytes to 152 KB. Identifying them is the first phase of this
work, not an assumption of it.

## Architecture

### Phase 1 — Identify before replacing

Nothing may be replaced before its contract is known. For each of the 26
unidentified payloads, produce:

- a decoded TDF field tree (`cmd/cfb27fixtureextract`, `cmd/inspectpayload`);
- the client screen or transition that requests it, from the ordered request
  transcript in the run directory's `cfb27-blaze.jsonl`;
- a classification: *league data* (derivable from the artifact), *session
  state* (derivable from the Dynasty service), *static configuration* (keep as
  a fixture), or *unknown*.

The deliverable is a checked-in fixture map. Payloads that stay *unknown* keep
their fixture and are explicitly listed as such, so the gap is visible rather
than implied.

### Phase 2 — Artifact projection service

Add a read side to `internal/dynasty` that projects a session's artifact into
typed JSON, backed by a long-lived `cmd/cfb27franchise` read mode rather than a
process spawn per request:

- `GET /sessions/{id}/league` — teams, conferences, ratings, branding
- `GET /sessions/{id}/staff` — coaches by team, with archetype/pipeline/prestige
- `GET /sessions/{id}/roster?teamKey=` — players with ratings and positions
- `GET /sessions/{id}/season` — stage, week, year, schedule

This is the piece that makes "pull the selected team's data" real: the data
comes from the league the session created, so a mid-dynasty roster change or a
coaching hire is reflected without re-recording anything.

Caching keys off `save_revision`, which already increments on every mutation.

### Phase 3 — Encode replies instead of patching bytes

Replace fixtures in dependency order, most-derivable first, one route per
change with a synthetic protocol test:

1. **541 hub** — smallest surface, already has a dedicated localizer to
   compare against, and its expected content is fully covered by
   `/season` + `/league`.
2. **393 roster** — `selectedDynastyRosterPayload` already assigns catalog
   players onto captured record slots; encoding the list directly removes the
   slot-count ceiling and the `BACC` record-scanning heuristic.
3. **533/133300354 staff** and **1111 coach selection** — covered by `/staff`.
4. **391 / 533-133300355 team databases** — the largest behavioural risk,
   because `dynastyTeamIdentityForKey` currently *reads* 133300356 as its
   identity source. That dependency has to move to the catalog first.

Each step keeps the fixture in the tree until its replacement passes a
side-by-side field-tree comparison against the recording for the Akron case.

### Phase 4 — Retire the Akron anchor

When no route depends on captured bytes, remove `capturedDynastyTeamKey`,
`localizeSelectedTeam`, and the anchor-based scalar overlay helpers. Until then
they stay, because they are the only thing keeping the unidentified payloads
coherent.

## Failure Handling

- A projection endpoint that cannot read the artifact returns a structured
  error; the Blaze layer must not fall back to a fixture silently, because that
  reproduces exactly the failure mode this work exists to remove.
- A schema that fails to bind is already a hard error (`assertTableSchemaBound`).
  Keep it that way — the 486 regression shipped precisely because a degraded
  read looked like a successful one.
- A save written by a different game build than the installed `Dynasty_Assets`
  must fail with the build mismatch named, not with blank fields.

## Testing

- Fixture map: every route in `capturedDynastyProgressionPayloads` is either
  classified or explicitly listed as unknown; the test fails when a new
  fixture is added without a classification.
- Projection: golden JSON per endpoint against a known seed artifact.
- Encoding: for each replaced route, the encoded reply's decoded field tree
  matches the recording when the session is set to the captured team.
- Round-trip: create session → select team → advance → restart the stack →
  reload, asserting the artifact hash and the projected state agree.

## Scope Boundaries

This does not implement a from-scratch league generator; `initialize` continues
to clone a seed save until evidence shows a generator is reachable. It does not
change the bridge, TLS, or authentication paths. It does not target Offline
Dynasty, and it does not extend the loopback-only safety boundary defined in
the 2026-08-04 private Online Dynasty design.
