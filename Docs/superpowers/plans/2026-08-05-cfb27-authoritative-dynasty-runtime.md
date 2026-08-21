# CFB27 Authoritative Dynasty Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Load every CFB27 coach from the supplied local FTC/FTX assets, use authoritative coach fields in Blaze responses, and persist legal dynasty load/advance state.

**Architecture:** A Node asset exporter creates an untracked, versioned JSON cache from one coherent Dynasty_Assets slot. Go loads that cache, resolves captured team keys by team identity, patches request-specific wire templates, and stores league progression in SQLite.

**Tech Stack:** Node.js 24, `madden-franchise` 4.3.6, Go, Cypress Blaze/TDF codec, SQLite.

## Global Constraints

- Asset root is `C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets`; select exactly one numbered slot.
- Never commit game-derived asset files or generated cache contents.
- Preserve existing user changes.
- Every production behavior gets a failing test first.

---

### Task 1: Export authoritative coach data

**Files:**
- Create: `tools/cypress-servers/package.json`
- Create: `tools/cypress-servers/cmd/cfb27assetexport/main.mjs`
- Create: `tools/cypress-servers/cmd/cfb27assetexport/main.test.mjs`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: numbered asset slot containing `franchise-schemas.FTX`, included FTX files, and `dynasty-dynasty-binary.FTC`.
- Produces: JSON `{version, source, teams, coaches}` with all active Coach fields and SHA-256 source hashes.

- [ ] Write a failing integration test asserting 433 active slot-0 coaches and the literal Ryan Day/Brent Venables anchor values from the design.
- [ ] Run `npm.cmd test -- --test-name-pattern="exports authoritative coaches"` and confirm failure because the exporter is absent.
- [ ] Implement schema-map construction, FTC decode, Team join, all-field serialization, and atomic cache output.
- [ ] Re-run the focused test and confirm it passes.

### Task 2: Load and validate coach cache in Go

**Files:**
- Create: `tools/cypress-servers/internal/cfb27assets/coaches.go`
- Create: `tools/cypress-servers/internal/cfb27assets/coaches_test.go`
- Modify: `tools/cypress-servers/internal/cfb27blaze/service.go`

**Interfaces:**
- Produces: `LoadCoachCatalog(path string) (*CoachCatalog, error)`, `HeadCoachByTeamName(string)`, and `CoachesByTeamName(string)`.

- [ ] Write failing table-driven tests for authoritative lookup, all-position coverage, duplicate-team rejection, and invalid cache version.
- [ ] Run `go test ./internal/cfb27assets -count=1` and confirm the package/API is missing.
- [ ] Implement strict JSON loading and normalized indexes without embedding game data.
- [ ] Re-run the focused tests and confirm they pass.

### Task 3: Replace captured coach mutations with authoritative fields

**Files:**
- Modify: `tools/cypress-servers/internal/cfb27blaze/dynasty_form_payloads.go`
- Modify: `tools/cypress-servers/internal/cfb27blaze/handlers.go`
- Modify: `tools/cypress-servers/internal/cfb27blaze/service_test.go`

**Interfaces:**
- Consumes: selected captured team key plus `CoachCatalog`.
- Produces: request-specific 533, 1111, and 534 replies using the same authoritative coach.

- [ ] Change the Ohio State and Oklahoma tests to expect portraits `618` and `898`, pipelines `Ohio` and `Kansas`, archetypes `CEO` and `Architect`, and to reject derived portrait `12`/`Local Coach`.
- [ ] Run the focused tests and confirm they fail against captured-key derivation.
- [ ] Resolve team key to team name, select the matching coach/position, and patch every represented coach field from the cache.
- [ ] Re-run the focused and full `cfb27blaze` tests.

### Task 4: Persist dynasty selection and advancement

**Files:**
- Modify: `tools/cypress-servers/internal/dynasty/service.go`
- Modify: `tools/cypress-servers/internal/dynasty/service_test.go`
- Modify: `tools/cypress-servers/internal/cfb27blaze/service.go`
- Modify: `tools/cypress-servers/internal/cfb27blaze/dynasty_progression_payloads.go`

**Interfaces:**
- Persists: selected team/coach, season, week, stage, ready users, idempotency key, and last completed transition.
- Produces: transactional `Advance(sessionID, userID, requestID)` and Blaze-backed progression responses.

- [ ] Write failing tests for restart persistence, commissioner/readiness validation, one-step advancement, duplicate request idempotency, and rollback.
- [ ] Run focused dynasty tests and confirm the missing-state failures.
- [ ] Add migrations and transactional transition logic, then bind captured advance commands to it.
- [ ] Re-run dynasty and Blaze package tests.

### Task 5: Startup integration and verification

**Files:**
- Modify: `tools/cypress-servers/deploy/start-cfb27-private-host.ps1`
- Modify: `tools/cypress-servers/deploy/cfb27-private.example.json`

**Interfaces:**
- Generates or refreshes the local coach cache when source hashes change and passes its path to the Blaze server.

- [ ] Write a failing script-level test for slot discovery and stale-cache refresh.
- [ ] Implement explicit asset-root/cache configuration and actionable startup diagnostics.
- [ ] Run Node tests, focused Go tests, `go test ./... -count=1`, launcher tests, and build the private host executable.
