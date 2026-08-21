# CFB27 Dynasty Week-One Coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Advance a newly created private Dynasty through contract signing into week one while covering every safe, capture-backed Dynasty route present in the supplied successful dump.

**Architecture:** Keep the Blaze route table explicit. Use constructed replies for small known schemas and empty acknowledgements; use exact captured binary fixtures for state-bearing DAL responses. The extracted payloads were verified not to contain the captured league-ID varint, so no unsafe identifier substitution is required. Do not turn unknown commands into blanket success.

**Tech Stack:** Go, Cypress Blaze/TDF codec, CFB27 ProtoSSL capture parser, embedded binary fixtures.

## Global Constraints

- Preserve the existing dirty worktree and do not commit or discard unrelated user changes.
- Write a failing route-contract test before each production behavior change.
- Only add routes observed in `CFB27-Dynasty-protossl_dump_1785904242.acp`.
- Never replay captured authentication/session secrets; only embed server-to-client Dynasty component payloads.
- Build the live server as `cfb27blaze.next.exe` when `cfb27blaze.exe` is running so the launcher can promote it safely.

---

### Task 1: Fix the immediate post-contract mutation

**Files:**
- Modify: `tools/cypress-servers/internal/cfb27blaze/handlers.go`
- Test: `tools/cypress-servers/internal/cfb27blaze/service_test.go`

**Interfaces:**
- Consumes: Blaze request route `(2098, 304)`.
- Produces: `SMSG=""`, `SUCC=1`, matching the 11-byte captured reply.

- [x] **Step 1: Write a failing test** that sends command `304` and asserts a successful reply containing empty `SMSG` and `SUCC=1`.
- [x] **Step 2: Run** `go test ./internal/cfb27blaze -run TestDynastyContractSettingsMutationMatchesCapture -count=1` and confirm `COMMAND_NOT_FOUND`.
- [x] **Step 3: Register command `304`** with `handleDynastyMutationSuccess`.
- [x] **Step 4: Re-run the focused test** and confirm it passes.

### Task 2: Cover capture-backed acknowledgement routes

**Files:**
- Modify: `tools/cypress-servers/internal/cfb27blaze/handlers.go`
- Test: `tools/cypress-servers/internal/cfb27blaze/service_test.go`

**Interfaces:**
- Consumes: captured mutation commands `107`, `163`, `164`, `414` and empty acknowledgement commands `176`, `192`, `222`, `272`, `276`, `312`, `322`, `362`, `392`, `412`, `502`, `532`, `542`, `562`, `801`, `1132`, `1152`, `1252`, `1272`, `1411`.
- Produces: the exact captured small response shape for each class.

- [x] **Step 1: Write table-driven failing tests** for every route and response class.
- [x] **Step 2: Run the focused tests** and confirm the currently unregistered commands fail.
- [x] **Step 3: Register only the listed capture-backed routes** with `handleDynastyMutationSuccess` or `handleEmptySuccess`.
- [x] **Step 4: Re-run the focused tests** and confirm all listed routes pass.

### Task 3: Preserve raw captured Dynasty response payloads for fixture extraction

**Files:**
- Modify: `tools/cypress-servers/internal/cfb27capture/pcap.go`
- Test: `tools/cypress-servers/internal/cfb27capture/pcap_test.go`

**Interfaces:**
- Produces: `FrameRecord.RawPayload []byte` with `json:"-"`, populated from the parsed Blaze frame without exposing it in ordinary JSON output.

- [x] **Step 1: Write a failing parser test** asserting the known synthetic payload bytes are retained in `RawPayload`.
- [x] **Step 2: Run** `go test ./internal/cfb27capture -run TestParseRetainsRawBlazePayload -count=1` and confirm the missing-field failure.
- [x] **Step 3: Add and populate `RawPayload`** using a defensive byte copy.
- [x] **Step 4: Re-run the parser package tests** and confirm they pass.

### Task 4: Extract and serve state-bearing week-one fixtures

**Files:**
- Create: `tools/cypress-servers/cmd/cfb27fixtureextract/main.go`
- Create: `tools/cypress-servers/internal/cfb27blaze/fixtures/dynasty-<command>-reply.bin`
- Create: `tools/cypress-servers/internal/cfb27blaze/dynasty_progression_payloads.go`
- Modify: `tools/cypress-servers/internal/cfb27blaze/service_test.go`

**Interfaces:**
- Consumes: successful server replies for commands `161`, `541`, `1251`, and later unique state-bearing Dynasty commands from the supplied dump.
- Produces: embedded raw handlers keyed by explicit `(2098, command)` routes; fixture validation proves the captured league ID `2561563` is absent from the selected response payloads.

- [x] **Step 1: Add failing fixture integrity and route tests** for all 29 state-bearing routes, using captured byte lengths and SHA-256 hashes as independent literals.
- [x] **Step 2: Run the focused tests** and confirm missing routes/fixtures fail.
- [x] **Step 3: Extract only server-to-client component `2098` replies** from the dump with the extractor and embed the selected fixtures.
- [x] **Step 4: Verify the captured league-ID varint is absent** from every selected fixture.
- [x] **Step 5: Register each state-bearing route explicitly** and re-run focused tests.

### Task 5: Verify and deploy

**Files:**
- Build output: `tools/cypress-servers/build/cfb27blaze.next.exe` or `cfb27blaze.exe`

**Interfaces:**
- Produces: the server binary promoted by the existing Cypress launcher.

- [x] **Step 1: Run** `go test ./internal/cfb27capture ./internal/cfb27blaze -count=1`.
- [x] **Step 2: Run** `go test ./... -count=1` and document only pre-existing environmental failures, if any.
- [x] **Step 3: Build the appropriate live or pending executable** based on the running process check.
- [x] **Step 4: Record the output path, timestamp, and SHA-256** for handoff.
