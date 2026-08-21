# CFB27 Dynasty Session Teardown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let CFB27 close an active private Dynasty cleanly before entering Road to Glory so RTG does not inherit stale Dynasty state.

**Architecture:** Handle Blaze BootStatus command `162` inside the existing CFB27 Blaze service. The handler resets only in-memory Dynasty lifecycle state and returns the protocol's normal empty success reply; it does not delete or mutate the persisted Dynasty session.

**Tech Stack:** Go, Cypress Blaze/TDF protocol implementation, Go `testing` package.

## Global Constraints

- Preserve persisted Dynasty data; teardown resets only active client-session state.
- Do not broaden unknown-command handling.
- Do not alter Team Builder data or save files in this change.

---

### Task 1: Add capture-scoped Dynasty teardown behavior

**Files:**
- Modify: `tools/cypress-servers/internal/cfb27blaze/service_test.go`
- Modify: `tools/cypress-servers/internal/cfb27blaze/handlers.go`

**Interfaces:**
- Consumes: `Service.HandleFrame(context.Context, string, blaze.Frame) blaze.Frame`
- Produces: `(*Service).handleDynastyClose(context.Context, blaze.Frame) ([]blaze.Field, uint16)` registered for `{ComponentBootStatus, 162}`.

- [ ] **Step 1: Write the failing regression test**

Create a service with nonzero `activeDynastySession`, `selectedTeam`, `dynastyContract`, `dynastyHub`, and `dynastyAdvance`; send component `2098`, command `162`; assert a success reply and that all five state values are zero.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/cfb27blaze -run TestDynastyCloseClearsActiveClientState -count=1`

Expected: FAIL because command `162` returns `ERR_COMMAND_NOT_FOUND` and leaves state unchanged.

- [ ] **Step 3: Implement the minimal handler**

Register command `162` and implement a handler that stores zero in the five in-memory lifecycle fields, then returns an empty success response.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run: `go test ./internal/cfb27blaze -run TestDynastyCloseClearsActiveClientState -count=1`

Expected: PASS.

- [ ] **Step 5: Run package and repository verification**

Run: `go test ./internal/cfb27blaze -count=1`

Run: `go test ./... -count=1`

Expected: PASS with no new failures.
