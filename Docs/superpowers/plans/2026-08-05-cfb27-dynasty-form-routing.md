# CFB27 Dynasty Form Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve the capture-backed Dynasty form schemas and settings mutation acknowledgements that currently return the game to the main menu.

**Architecture:** Extend the Blaze service with request-aware raw handlers for responses that vary by request data. Keep exact captured binary form payloads separate from ordinary structured handlers, and keep uncaptured Team Builder behavior outside this change.

**Tech Stack:** Go, embedded binary fixtures, Cypress Blaze TDF codec, Go tests

## Global Constraints

- Only replay responses proven by `CFB27-Dynasty-protossl_dump_1785904242.acp`.
- Preserve command-not-found behavior for unknown routes and unknown FORM values.
- Do not implement command 1269 without a proven Team Builder response contract.
- Do not commit from the dirty shared working tree.

---

### Task 1: Request-aware Dynasty form responses

**Files:**
- Create: `tools/cypress-servers/internal/cfb27blaze/dynasty_form_payloads.go`
- Create: `tools/cypress-servers/internal/cfb27blaze/fixtures/dynasty-533-form-133300354-reply.bin`
- Create: `tools/cypress-servers/internal/cfb27blaze/fixtures/dynasty-533-form-133300355-reply.bin`
- Create: `tools/cypress-servers/internal/cfb27blaze/fixtures/dynasty-533-form-133300356-reply.bin`
- Modify: `tools/cypress-servers/internal/cfb27blaze/service.go`
- Test: `tools/cypress-servers/internal/cfb27blaze/service_test.go`

**Interfaces:**
- Consumes: `blaze.Frame`, `blaze.Decode`, and the existing `route` key.
- Produces: `type rawRequestHandler func(context.Context, blaze.Frame) ([]byte, uint16)` and `capturedDynastyFormHandlers() map[route]rawRequestHandler`.

- [ ] **Step 1: Write the failing tests**

Encode a request containing an integer `FORM` field for each captured ID. Assert reply status, zero error, and these exact contracts: 133300354 = 11646 bytes / SHA-256 `3757e325ef46d950d8353b82b079aff6d441ebe2f320a158e796581832a78e7f`; 133300355 = 61152 / `63fdc08f4d8f9f9973b026934ee399fe2b4d23815c56b8a93a19ecdbbf8c29d9`; 133300356 = 423124 / `755ae814ac016c1244a382b4bd1fbf9cad18a61890eaf1da3bb2f184fc68a320`. Assert an unknown FORM returns command-not-found.

- [ ] **Step 2: Run the focused tests and confirm RED**

Run `go test ./internal/cfb27blaze -run DynastyForm -count=1` from `tools/cypress-servers`. Expected: command 533 returns command-not-found.

- [ ] **Step 3: Add request-aware dispatch and embedded fixtures**

Add a service map keyed by route. In `HandleFrame`, call its handler after route-only raw fixtures and before structured handlers, copying its bytes into a reply on error code zero or producing an error reply otherwise. The command 533 handler decodes `FORM`, selects only the matching embedded payload, and returns `ErrorCommandNotFound` for unsupported IDs.

- [ ] **Step 4: Run the focused tests and confirm GREEN**

Run `go test ./internal/cfb27blaze -run DynastyForm -count=1` from `tools/cypress-servers`. Expected: all form variants and rejection case pass.

### Task 2: Dynasty settings mutation acknowledgements

**Files:**
- Modify: `tools/cypress-servers/internal/cfb27blaze/handlers.go`
- Test: `tools/cypress-servers/internal/cfb27blaze/service_test.go`

**Interfaces:**
- Consumes: the existing structured `handler` interface.
- Produces: `handleDynastyMutationSuccess(context.Context, blaze.Frame) ([]blaze.Field, uint16)` returning `SMSG=""`, `SUCC=1`.

- [ ] **Step 1: Write the failing tests**

For commands 303, 305, 306, 307, and 309, assert a successful reply decodes to empty `SMSG` and integer `SUCC=1`. For command 302, assert a successful empty reply.

- [ ] **Step 2: Run the focused tests and confirm RED**

Run `go test ./internal/cfb27blaze -run DynastySettingsMutation -count=1` from `tools/cypress-servers`. Expected: each route returns command-not-found.

- [ ] **Step 3: Register the captured handlers**

Register 303, 305, 306, 307, and 309 to `handleDynastyMutationSuccess`; register 302 to `handleEmptySuccess`. Do not add a blanket success rule.

- [ ] **Step 4: Run focused and package verification**

Run `go test ./internal/cfb27blaze -count=1` and build `go build -o build/cfb27blaze.exe ./cmd/cfb27blaze` from `tools/cypress-servers`. Expected: tests and build pass.

### Task 3: Team Builder contract investigation

**Files:**
- Inspect: the installed CFB27 executable and the latest Blaze event log
- No server mutation unless an exact response schema is established

**Interfaces:**
- Consumes: command 1269 request tags `CKEY`, `LGID`, and `SKEY`, plus executable reflection/type metadata for `TeamBuilderDynasty::EnterResponse`.
- Produces: either a proven minimal response field schema with evidence or a precise requirement for a successful Team Builder capture.

- [ ] **Step 1: Search executable metadata references efficiently**

Locate pointers to the known `TeamBuilderDynasty::EnterResponse` type-name string and inspect adjacent reflection descriptors rather than scanning the full executable instruction stream.

- [ ] **Step 2: Validate any discovered tags**

Require field names, types, and collection structure to be supported by metadata or captured bytes. If that proof is absent, leave 1269 unsupported and report the capture gap explicitly.
