# CFB27 Private Online Dynasty Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the private bridge on the currently installed CFB27 build and produce a loopback-only capture that identifies the first missing Online Dynasty service request.

**Architecture:** The standalone `dinput8.dll` proxy will discover the verified ProtoSSL certificate-decision sequence in the live executable image instead of requiring the TU2 PE timestamp and fixed RVA. Its Winsock/WinHTTP hooks will remain active for the explicit private launch and route all non-loopback attempts into the local Cypress gateway, where HTTP, TLS, and Blaze requests are recorded. The existing launcher remains the authority for starting services, staging the proxy, setting the run directory, and launching the game.

**Tech Stack:** C++20, Win32, Winsock, PowerShell, Go, .NET 8

## Global Constraints

- Target the game's Online Dynasty mode, not its Offline Dynasty mode.
- A private run must permit zero non-loopback connections to EA or any other public service.
- Never replace or modify a CFB27 executable.
- Apply the ProtoSSL patch only after exactly one complete runtime signature match and post-write verification.
- Unknown Blaze commands receive explicit diagnostics, not blanket success.
- Do not commit captures, dumps, credentials, account tokens, game files, or personal Dynasty data.
- Preserve all unrelated uncommitted work already present in the repository.

---

### Task 1: Runtime signature scanner

**Files:**
- Create: `tools/cypress-servers/bridge/runtime_scan.h`
- Create: `tools/cypress-servers/bridge/tests/runtime_scan_tests.cpp`
- Create: `tools/cypress-servers/bridge/test-bridge.ps1`

**Interfaces:**
- Produces: `Cypress::CFB27::RuntimeScan::FindUniquePattern(std::span<const std::uint8_t>, std::span<const std::uint8_t>) -> PatternResult`
- Produces: `PatternResult { std::size_t offset; std::size_t count; }`, with `offset == npos` unless `count == 1`.

- [ ] **Step 1: Write failing scanner tests**

  Add literal fixtures for one match, no match, two matches, a pattern longer than the region, and a match at the final legal byte. The one-match assertion must expect the hand-derived offset `2`; the multiple-match assertion must expect `count == 2` and `offset == npos`.

- [ ] **Step 2: Verify the tests fail for the missing header**

  Run: `powershell -ExecutionPolicy Bypass -File tools/cypress-servers/bridge/test-bridge.ps1`

  Expected: compilation fails because `runtime_scan.h` does not exist.

- [ ] **Step 3: Implement the minimal pure scanner**

  Implement a bounds-checked byte comparison over the supplied spans. Count every match, retain the candidate offset only while the count is one, and return `npos` as soon as a second match is observed.

- [ ] **Step 4: Verify scanner tests pass**

  Run the same test command. Expected: `runtime_scan_tests: PASS` and exit code zero.

### Task 2: Live executable discovery and guarded patch

**Files:**
- Modify: `tools/cypress-servers/bridge/bridge.cpp`
- Modify: `tools/cypress-servers/bridge/build-bridge.ps1`
- Test: `tools/cypress-servers/bridge/tests/runtime_scan_tests.cpp`

**Interfaces:**
- Consumes: `RuntimeScan::FindUniquePattern` from Task 1.
- Produces: bridge log events `runtime scan started`, `runtime signature matches=<n>`, `runtime signature discovered rva=0x...`, and verified patch success/failure.

- [ ] **Step 1: Add a failing test for patch-offset derivation**

  Extend the fixture with the 36-byte verified signature and assert that the three-byte patch begins at literal signature offset `28`.

- [ ] **Step 2: Verify the new test fails**

  Run the bridge tests. Expected: failure because the production signature/patch offset is not exported by the scanner header.

- [ ] **Step 3: Move signature metadata into the tested header**

  Define the verified signature, replacement bytes, and `kPatchOffsetInSignature` in `runtime_scan.h`. Keep the bridge log version current.

- [ ] **Step 4: Replace the TU2-only patch gate**

  In `PatchWorker`, validate only the PE structure and image bounds first. Poll committed executable image regions with `VirtualQuery`, use the pure scanner on readable executable spans, aggregate the total match count, and patch only a single match. Log current PE timestamp/image size for correlation. Zero matches continue polling until timeout; multiple matches abort immediately without modifying memory.

- [ ] **Step 5: Decouple network enforcement from the stale build header**

  Remove the `SupportedExecutable` early return from `RedirectWorker`. Patch known fixed slots only when their current pointer equals the expected Winsock function; always scan loaded import tables. This keeps the fixed slots fail-closed while allowing portable import interception on a new build.

- [ ] **Step 6: Build and run native tests**

  Run:

  ```powershell
  powershell -ExecutionPolicy Bypass -File tools/cypress-servers/bridge/test-bridge.ps1
  powershell -ExecutionPolicy Bypass -File tools/cypress-servers/bridge/build-bridge.ps1 -Output Launcher/cypress_CFB27.dll
  ```

  Expected: tests pass and the x64 proxy DLL builds without warnings or errors.

### Task 3: Private Online Dynasty launch configuration

**Files:**
- Modify: `Launcher/CypressLauncher/MessageHandler.Diagnostics.cs`
- Modify: `Launcher/CypressLauncher/MessageHandler.Launch.cs`
- Create: `tools/cypress-servers/deploy/prepare-cfb27-online-dynasty-capture.ps1`

**Interfaces:**
- Consumes: `Launcher/cypress_CFB27.dll` and the existing `EnsureCFB27PrivateStackAsync` launch path.
- Produces: a timestamped private run with bridge, Blaze, Dynasty, and launcher logs.

- [ ] **Step 1: Write a failing script preflight test mode**

  Add `-PreflightOnly` to the new script contract and run it before the script exists. Expected: PowerShell reports the file is missing.

- [ ] **Step 2: Implement the preparation script**

  Resolve the repository and configured game directory, fail if the game executable or build tools are missing, build the scanner tests, proxy, Go services, and Windows launcher, verify their outputs, and print the exact launcher/run locations. `-PreflightOnly` performs validation and builds but never starts the game.

- [ ] **Step 3: Make the launcher config unambiguous**

  Name the generated mode `privateOnlineDynasty=true`, keep the Dynasty and Blaze loopback URLs, and log that external pass-through is disabled. Do not enable the obsolete guard-page verifier probe or the old fixed-RVA server DLL path when the standalone proxy is packaged.

- [ ] **Step 4: Run build-only verification**

  Run:

  ```powershell
  powershell -ExecutionPolicy Bypass -File tools/cypress-servers/deploy/prepare-cfb27-online-dynasty-capture.ps1 -PreflightOnly
  ```

  Expected: bridge tests, DLL build, Go tests/build, and launcher build succeed; no CFB27 process starts.

### Task 4: Controlled Online Dynasty capture

**Files:**
- Reference: `%APPDATA%/Cypress/CFB27/Private/runs/<timestamp>/cfb27-bridge.log`
- Reference: `%APPDATA%/Cypress/CFB27/Private/runs/<timestamp>/cfb27-blaze.jsonl`
- Modify after evidence: `tools/cypress-servers/internal/cfb27blaze/handlers.go`
- Modify after evidence: `tools/cypress-servers/internal/cfb27blaze/service_test.go`

**Interfaces:**
- Consumes: the verified bridge and private launch path from Tasks 1-3.
- Produces: an ordered Online Dynasty transcript and the first unsupported request/crash boundary.

- [ ] **Step 1: Start the prepared private launcher**

  Run the preparation script without `-PreflightOnly`, then use the existing CFB27 Host action to launch the Online Dynasty private stack.

- [ ] **Step 2: Verify the runtime bridge gate before navigating**

  Require a unique signature discovery, verified ProtoSSL patch, and installed socket hooks in `cfb27-bridge.log`. If any is absent, stop the run and preserve diagnostics.

- [ ] **Step 3: Navigate to Online Dynasty and reproduce the crash**

  Select Online Dynasty and the seeded local Dynasty once. Do not launch any EA/normal-online flow.

- [ ] **Step 4: Identify the first missing contract**

  Correlate the last Blaze request/response, HTTP request, bridge event, and game exit timestamp. Record the concrete component/command or HTTP route and bounded payload shape.

- [ ] **Step 5: Add one capture-backed handler using TDD**

  Write a synthetic request test that fails with `ErrorCommandNotFound`, implement only the observed response fields, then rerun `go test ./internal/cfb27blaze -v` and the full Go suite.

- [ ] **Step 6: Repeat until the local Dynasty hub loads**

  Repeat Steps 3-5 one request at a time. Completion requires a seeded private Online Dynasty to load, persist a local state change, reload after restart, and record zero permitted non-loopback connections.

