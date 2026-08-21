# CFB27 Authoritative Dynasty Runtime Design

## Goal

Replace team-name-only mutation of captured Dynasty payloads with a local, schema-backed data source that preserves every coach field and provides persistent league state for loading and week advancement.

## Evidence and root cause

The current Blaze service replays captured component 2098 payloads and mutates a small set of fields. `dynastyHeadCoachForTeam` derives portraits from a captured coach key and the contract handler returns `CHNM="Local Coach"`. This produces internally inconsistent records: the screenshots show the right school and coach name but generic or wrong portraits, archetypes, pipelines, and related values.

The supplied slot-0 assets contain the authoritative records. `Coach.FTX` defines 137 fields. The zlib-compressed `dynasty-dynasty-binary.FTC` contains 632 coach slots, 433 active records, unique head assets, and portrait IDs. Independent decoding establishes these anchors:

- Ryan Day: `Unique_C_DayRyan_665`, portrait `618`, level `76`, head coach, team index `68`, pipeline `Ohio`, archetype `CEO`.
- Brent Venables: `Unique_C_VenablesBrent_668`, portrait `898`, level `59`, head coach, team index `69`, pipeline `Kansas`, archetype `Architect`.

## Architecture

### Asset export boundary

A local Node exporter uses the MIT-licensed `madden-franchise` package, which supports CFB27 FTC files, and the user's extracted FTX tree. It selects one numbered asset slot, builds the schema file map from that same slot, reads `Coach` and `Team`, and writes a versioned JSON cache. Game-derived FTC/FTX files and generated cache data remain local and untracked.

The cache contains source hashes, schema revision, every active coach field, and normalized indexes by record ID, team index, position, and team name. Export is atomic: a failed decode never replaces the last valid cache.

### Blaze coach resolution

The Go server loads the cache at startup. Captured team keys are joined to authoritative teams by the team identity already present in form `133300356`; row order is never used as a team identifier. Coach-selection responses replace the captured coach fields with authoritative values, including portrait, names, level, archetype, offensive/defensive schemes, pipeline, position, and stable coach record ID. Contract confirmation uses the same selected coach instead of `Local Coach`.

If the cache is absent, stale, ambiguous, or schema-incompatible, the service records a diagnostic and refuses coach-specific mutation rather than fabricating a record.

### Persistent dynasty state

The existing SQLite dynasty service becomes the owner of league identity, selected team/coach, current season week, stage, readiness, and action results. Blaze request handlers read and update this state. Captured fixtures remain wire-schema templates only; state-bearing identifiers and values come from the league model.

Advancement is explicit and transactional: validate commissioner/readiness, record the requested transition, advance one legal stage/week, persist it, then emit the capture-backed acknowledgements and notifications for that transition. Duplicate requests are idempotent. Unknown commands and illegal transitions remain errors.

## Verification

- Exporter integration tests decode the supplied slot and assert the Ryan Day and Brent Venables anchors plus complete active-coach coverage.
- Go catalog tests reject missing/stale/mixed-slot caches and resolve head coaches and assistants by team/position.
- Blaze tests assert authoritative portrait/data fields for Ohio State and Oklahoma and prove `Local Coach`, derived portrait `12`, and captured fallback values are absent.
- Dynasty tests cover persistence across restart, legal week advancement, duplicate requests, readiness, and rollback on failure.
- Focused Go, Node, launcher, and full repository tests run before deployment.

## Constraints

- Do not commit EA-derived FTC, FTX, FTB, BIN, ACP, or generated coach-cache contents.
- Preserve the existing dirty worktree and unrelated user changes.
- Add every production behavior through a failing test first.
- Do not replace unknown Dynasty commands with blanket success.
