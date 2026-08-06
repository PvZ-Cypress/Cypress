# CFB27 Portable VPN Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build verified, self-contained Windows server and player ZIP archives for the current CFB27 private-server stack over a private VPN.

**Architecture:** A PowerShell release tool builds the Go services, CFB27 bridge, and self-contained launcher, then assembles server and client staging trees from explicit allow-lists. Package-local lifecycle scripts validate configuration and compatibility before starting services or modifying a game directory; release verification extracts both ZIPs and runs offline smoke tests.

**Tech Stack:** PowerShell 5.1+, Go, C++/CMake/MSVC, .NET 8 win-x64 publish, Node.js portable runtime, JSON, ZIP/SHA-256.

## Global Constraints

- Dedicated host operating system is Windows.
- Players run legally owned CFB27 installations on their own Windows PCs.
- Player-to-host connectivity uses a private VPN such as Tailscale or ZeroTier.
- The host does not require the game, Go, Node.js, npm, Visual Studio, or the .NET SDK after packaging.
- The client rejects unknown CFB27 executable hashes before installing the bridge.
- The release never includes personal saves, captures, databases, credentials, or runtime logs.
- Extracted `Dynasty_Assets` are bundled because no automated packed-archive extractor exists.

---

### Task 1: Package configuration and shared validation

**Files:**
- Create: `tools/cypress-servers/release/Release.Common.psm1`
- Create: `tools/cypress-servers/release/server.example.json`
- Create: `tools/cypress-servers/release/compatibility.json`
- Create: `tools/cypress-servers/release/tests/Test-Release.Common.ps1`

**Interfaces:**
- Produces: `Read-CypressJson -Path <file>`, `Assert-CypressFile -Path <file> -Label <text>`, `Test-CFBChunksFile -Path <file>`, `Get-CFB27ExecutableInfo -GameDirectory <dir>`, `Test-CFB27Compatibility -ExecutableInfo <object> -Manifest <object>`.
- Produces: server config keys `vpnBindAddress`, `vpnRemoteAddress`, `blazePort`, `diagnosticsPort`, `dynastyPort`, `profile`, `assetSlot`, and `dynastySeed`.

- [ ] **Step 1: Write common-module tests**

Create a temporary fixture with a valid eight-byte `FBCHUNKS` header, an invalid save, and a fake `CollegeFB27.exe`. Assert header validation, SHA-256/size collection, known-hash acceptance, unknown-hash rejection, and malformed JSON errors.

- [ ] **Step 2: Run the tests and verify failure**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File tools/cypress-servers/release/tests/Test-Release.Common.ps1`

Expected: FAIL because `Release.Common.psm1` does not exist.

- [ ] **Step 3: Implement the shared module and schemas**

Implement strict JSON parsing, literal-path validation, non-mutating executable inspection, exact hash matching, and the documented server configuration. Seed `compatibility.json` from the locally installed supported executable when available; each entry includes `fileName`, `sha256`, `size`, `fileVersion`, and `bridgeProfile`.

- [ ] **Step 4: Run the tests**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File tools/cypress-servers/release/tests/Test-Release.Common.ps1`

Expected: PASS.

### Task 2: Dedicated server lifecycle

**Files:**
- Create: `tools/cypress-servers/release/server/Setup-Server.ps1`
- Create: `tools/cypress-servers/release/server/Start-Server.ps1`
- Create: `tools/cypress-servers/release/server/Stop-Server.ps1`
- Create: `tools/cypress-servers/release/server/Test-Server.ps1`
- Create: `tools/cypress-servers/release/tests/Test-ServerScripts.ps1`

**Interfaces:**
- Consumes: package layout `bin`, `runtime`, `tools`, `node_modules`, `assets/Dynasty_Assets`, and `config/server.json`.
- Produces: `data/cfb27_dynasty.db`, `data/dynasties`, `data/cfb27-assets-slot<N>.json`, `runs/<timestamp>`, and `run/server-pids.json`.

- [ ] **Step 1: Write server lifecycle tests**

Build a temporary package fixture with stub Dynasty/Blaze executables and assert that setup rejects invalid seeds and missing slots, preserves an existing `data` directory, writes only package-local config, start records exact PIDs after both health endpoints pass, and stop targets only recorded PIDs. Assert firewall creation is skipped unless explicitly requested from an elevated shell.

- [ ] **Step 2: Run the tests and verify failure**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File tools/cypress-servers/release/tests/Test-ServerScripts.ps1`

Expected: FAIL because the lifecycle scripts do not exist.

- [ ] **Step 3: Implement setup, start, stop, and health scripts**

Setup accepts `-VpnBindAddress`, `-VpnRemoteAddress`, `-DynastySeed`, `-Profile`, `-AssetSlot`, and `-InstallFirewallRule`. Start uses package-relative paths, exports the catalog with bundled Node, starts Dynasty on loopback, starts Blaze on the VPN address, waits for both health endpoints, and writes an atomic PID file. Stop verifies process executable paths before stopping them. Test validates static files, seed headers, ports, bind address, and live health when running.

- [ ] **Step 4: Run server tests**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File tools/cypress-servers/release/tests/Test-ServerScripts.ps1`

Expected: PASS.

### Task 3: Player setup and safe launch

**Files:**
- Create: `tools/cypress-servers/release/client/Setup-Client.ps1`
- Create: `tools/cypress-servers/release/client/Start-Client.ps1`
- Create: `tools/cypress-servers/release/client/Uninstall-Client.ps1`
- Create: `tools/cypress-servers/release/tests/Test-ClientScripts.ps1`

**Interfaces:**
- Consumes: package-local `compatibility.json`, `payload/cypress_CFB27.dll`, and `payload/cfb27-endpoints.json`.
- Produces: `%APPDATA%/Cypress/CFB27/Remote/client.json`, `cfb27-bridge.ini`, compatibility reports, and a reversible bridge installation in the selected game directory.

- [ ] **Step 1: Write client safety tests**

Use temporary fake game directories. Assert unknown hashes create a report and make no game-directory changes; known hashes back up an existing `dinput8.dll`, install the bridge and endpoint file, persist the VPN host/port, and uninstall restores only the recorded backup. Assert launch uses `UseShellExecute=false` and the required `CYPRESS_CFB27_*` environment variables without starting local backend services.

- [ ] **Step 2: Run the tests and verify failure**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File tools/cypress-servers/release/tests/Test-ClientScripts.ps1`

Expected: FAIL because the client scripts do not exist.

- [ ] **Step 3: Implement client setup, launch, and uninstall**

Setup accepts `-GameDirectory`, `-ServerAddress`, `-BlazePort`, and `-Profile`; validates the game hash first; writes an update diagnostic on rejection; creates a timestamped backup before copying files; and records installed hashes. Start revalidates compatibility and installed payloads, writes bridge configuration, sets remote Blaze environment variables, and launches the game. Uninstall compares hashes before removal and restores the recorded prior DLL.

- [ ] **Step 4: Run client tests**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File tools/cypress-servers/release/tests/Test-ClientScripts.ps1`

Expected: PASS.

### Task 4: Reproducible release builder

**Files:**
- Create: `tools/cypress-servers/release/Build-Release.ps1`
- Create: `tools/cypress-servers/release/Verify-Release.ps1`
- Create: `tools/cypress-servers/release/tests/Test-ReleaseLayout.ps1`

**Interfaces:**
- Consumes: repository source, `C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets`, installed Node runtime, CMake/MSVC, Go, and .NET SDK.
- Produces: `dist/cfb27-private/<version>/Cypress-CFB27-Server-<version>-win-x64.zip`, `Cypress-CFB27-Client-<version>-win-x64.zip`, `manifest.json`, and `SHA256SUMS.txt`.

- [ ] **Step 1: Write release-layout tests**

Assert both staging trees contain every documented executable/script/config and exclude patterns for `captures`, `*.db`, `DYNASTY*`, `*.log`, `.git`, source objects, and secrets. Assert all manifest hashes match and both ZIPs contain the same `VERSION.txt`.

- [ ] **Step 2: Run the test and verify failure**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File tools/cypress-servers/release/tests/Test-ReleaseLayout.ps1 -ReleaseRoot dist/cfb27-private/test`

Expected: FAIL because no staged release exists.

- [ ] **Step 3: Implement the builder**

Run Go/Node/C++/.NET tests, build fresh Go services and bridge, publish the launcher with `--self-contained true -r win-x64`, run `npm ci --omit=dev --ignore-scripts`, copy the portable Node runtime and licenses, copy Dynasty assets, assemble from explicit paths, generate per-package manifests, calculate checksums, and compress with deterministic package names. Never copy the repository root recursively.

- [ ] **Step 4: Implement extracted-package verification**

Verify ZIP hashes, extract into fresh temporary directories, parse every PowerShell script, validate static layouts and manifests, run server health startup/shutdown on loopback test ports, and run client compatibility tests without launching the game.

- [ ] **Step 5: Build and verify the release**

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File tools/cypress-servers/release/Build-Release.ps1 -Configuration Release`

Then: `powershell -NoProfile -ExecutionPolicy Bypass -File tools/cypress-servers/release/Verify-Release.ps1 -ReleaseRoot <printed-release-directory>`

Expected: both commands exit 0 and print the two ZIP paths and SHA-256 values.

### Task 5: Host and player documentation

**Files:**
- Create: `tools/cypress-servers/release/server/README-SERVER.md`
- Create: `tools/cypress-servers/release/client/README-PLAYER.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: final script parameters and package layout.
- Produces: exact install, VPN, firewall, update, backup, start/stop, and troubleshooting instructions.

- [ ] **Step 1: Write both focused guides**

Document Tailscale/ZeroTier prerequisites, server setup command, required Dynasty seed, client setup command, connection address, safe uninstall, data backup, unknown-update report collection, and the explicit limitation that new game executables may require a compatibility update.

- [ ] **Step 2: Add the repository release entry point**

Add a CFB27 portable-release section linking the builder and design without changing existing PvZ hosting instructions.

- [ ] **Step 3: Verify documentation commands**

Parse every documented PowerShell command and compare parameter names with `Get-Help` output from the packaged scripts.

- [ ] **Step 4: Run final verification**

Run all release PowerShell tests, `go test ./...`, Node tests, bridge tests, launcher tests, and extracted-package verification. Record any environment-specific skipped test with its exact reason in the final release manifest.
