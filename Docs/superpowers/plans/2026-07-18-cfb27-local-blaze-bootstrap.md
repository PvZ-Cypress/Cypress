# CFB27 Local Blaze Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the supported CFB27 build send its first plaintext local Blaze request and record the online bootstrap sequence.

**Architecture:** Reassemble Fire2 frames from the supplied ACP capture, add a SHA-gated local-only `ProtoSSLConnect` downgrade, and permit plaintext Fire2 only on Cypress's loopback listener.

**Tech Stack:** Go 1.22, C++20, Win32, MinHook, CMake.

## Global Constraints

- Hook only supported SHA-256 `A048578530F7ED5967DF38803B63AD9B9F04FC71287F1E151C901A94AB240BFD` at `ProtoSSLConnect` RVA `0x16D0DD0`.
- Change `secure` only for a connection already recorded as a Cypress loopback redirect.
- Do not commit captures, dumps, ZIPs, account data, or game files.
- Disable the transport with `CYPRESS_CFB27_ENABLE_LOCAL_PLAINTEXT=0`.

### Task 1: Reassemble ACP TCP streams

**Files:**

- Modify: `tools/cypress-servers/internal/cfb27capture/pcap.go`
- Modify: `tools/cypress-servers/internal/cfb27capture/pcap_test.go`
- Modify: `tools/cypress-servers/cmd/cfb27capture/main.go`

**Interfaces:** Produces a `Report` containing Fire2 frames reassembled across contiguous payload segments, keyed by directional TCP four-tuple.

- [ ] **Step 1: Write the failing split-frame test**

```go
func TestParseReassemblesFrameAcrossTCPPackets(t *testing.T) {
    frame := blaze.Frame{Header: blaze.Header{Component: 1, Command: 10, MessageType: blaze.MessageRequest, MessageID: 7}, Payload: []byte{1, 2}}
    wire := encodeFrame(t, frame)
    report, err := Parse(bytes.NewReader(pcap(tcpPacket(100, wire[:9]), tcpPacket(109, wire[9:]))))
    if err != nil { t.Fatal(err) }
    if len(report.Frames) != 1 || report.Frames[0].Command != 10 { t.Fatalf("frames=%+v", report.Frames) }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/cfb27capture -run TestParseReassemblesFrameAcrossTCPPackets -v`

Expected: FAIL because the parser discards the first partial packet.

- [ ] **Step 3: Implement bounded stream state**

```go
type streamKey struct { source, destination string; sourcePort, destinationPort uint16 }
type streamBuffer struct { nextSequence uint32; data []byte; firstTimestamp time.Time }

func (report *Report) appendTCP(key streamKey, sequence uint32, payload []byte, timestamp time.Time) {
    // Reject gaps; append contiguous bytes; repeatedly parse complete blaze.HeaderSize frames.
}
```

- [ ] **Step 4: Verify GREEN and derive the actual capture routes**

Run: `go test ./internal/cfb27capture -v && go run ./cmd/cfb27capture -format json C:\Users\Shadow\Downloads\protossl_dump_1783976660.acp`

Expected: tests pass and the output contains `component=1 command=10`.

### Task 2: Add a guarded ProtoSSL local-plaintext hook

**Files:**

- Create: `Server/include/CFB27/ProtoSslHook.h`
- Create: `Server/Source/CFB27/ProtoSslHook.cpp`
- Modify: `Server/include/CFB27/BridgeConfig.h`
- Modify: `Server/Source/CFB27/BridgeConfig.cpp`
- Modify: `Server/Source/CFB27/BridgeBootstrap.cpp`
- Modify: `Server/CMakeLists.txt`
- Modify: `Server/Tests/CFB27BridgeTests.cpp`

**Interfaces:** `bool ShouldUseLocalPlaintext(bool enabled, bool redirected, bool loopback)` and `bool InstallProtoSslLocalPlaintextHook(const BridgeConfig&, BridgeLog&)`.

- [ ] **Step 1: Write the failing native policy tests**

```cpp
Check(Cypress::CFB27::ShouldUseLocalPlaintext(true, true, true), "redirected loopback must downgrade");
Check(!Cypress::CFB27::ShouldUseLocalPlaintext(true, false, true), "untracked loopback must not downgrade");
Check(!Cypress::CFB27::ShouldUseLocalPlaintext(true, true, false), "production route must not downgrade");
Check(!Cypress::CFB27::ShouldUseLocalPlaintext(false, true, true), "configuration must disable downgrade");
```

- [ ] **Step 2: Verify RED**

Run: `cmake --build Server/build --config Release --target CFB27BridgeTests`

Expected: build fails because the policy API is absent.

- [ ] **Step 3: Implement config and the pure policy**

```cpp
// BridgeConfig
bool enableLocalPlaintext = true;
// Environment key: CYPRESS_CFB27_ENABLE_LOCAL_PLAINTEXT
bool ShouldUseLocalPlaintext(bool enabled, bool redirected, bool loopback) {
    return enabled && redirected && loopback;
}
```

- [ ] **Step 4: Implement the validated MinHook detour**

```cpp
using ProtoSSLConnectFunction = int(__cdecl*)(void*, int32_t, const char*, uint32_t, int32_t);
constexpr std::uintptr_t kProtoSSLConnectRva = 0x16D0DD0;
int __cdecl HookProtoSSLConnect(void* state, int32_t secure, const char* host, uint32_t address, int32_t port) {
    const bool loopback = address == 0x7F000001 && port == s_bridgePort;
    const bool local = ShouldUseLocalPlaintext(s_enabled, IsRedirectedSocketForDiagnostics(reinterpret_cast<std::uintptr_t>(state)), loopback);
    return s_original(state, local ? 0 : secure, host, address, port);
}
```

Validate that the RVA maps to executable nonzero bytes before creating the hook; log every downgraded call. Install it after redirector hooks.

- [ ] **Step 5: Verify GREEN**

Run: `cmake --build Server/build --config Release --target CFB27BridgeTests Cypress && Server\build\Release\CFB27BridgeTests.exe`

Expected: build succeeds and output is `CFB27 bridge tests passed`.

### Task 3: Accept loopback plaintext Fire2

**Files:**

- Modify: `tools/cypress-servers/internal/cfb27blaze/service.go`
- Modify: `tools/cypress-servers/internal/cfb27blaze/service_test.go`
- Modify: `tools/cypress-servers/deploy/start-cfb27-private-host.ps1`

**Interfaces:** listener event `transport:"plaintext"`; existing handler loop receives a plaintext `blaze.Frame`.

- [ ] **Step 1: Write the failing plaintext login test**

```go
func TestLoopbackPlaintextConnectionDispatchesLogin(t *testing.T) {
    service := startService(t, Config{Bind: "127.0.0.1", AllowLoopbackPlaintext: true})
    conn, err := net.Dial("tcp", service.Address())
    if err != nil { t.Fatal(err) }
    defer conn.Close()
    request := blaze.Frame{Header: blaze.Header{Component: 1, Command: 10, MessageType: blaze.MessageRequest, MessageID: 42}}
    if err := blaze.WriteFrame(conn, request); err != nil { t.Fatal(err) }
    response, err := blaze.ReadFrame(conn)
    if err != nil { t.Fatal(err) }
    if response.Header.MessageID != 42 { t.Fatalf("message id=%d", response.Header.MessageID) }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/cfb27blaze -run TestLoopbackPlaintextConnectionDispatchesLogin -v`

Expected: FAIL because the listener begins TLS unconditionally.

- [ ] **Step 3: Sniff and restrict the connection**

```go
prefix, err := buffered.Peek(1)
if prefix[0] == 0x16 { return tls.Server(bufferedConn, config), "tls", nil }
if !allowLoopbackPlaintext || !isLoopback(conn.RemoteAddr()) { return nil, "", ErrPlaintextNotAllowed }
return bufferedConn, "plaintext", nil
```

Run the existing Fire2 session loop directly for plaintext. Add `-allow-loopback-plaintext` to the host script and keep TLS support enabled.

- [ ] **Step 4: Verify GREEN and live boundary**

Run: `go test ./internal/cfb27blaze ./internal/blaze -v; tools\cypress-servers\build.ps1; tools\cypress-servers\deploy\start-cfb27-private-host.ps1`

Expected: tests pass. A real game run must produce `accepted` → `transport: plaintext` → decoded `frame` in `cfb27-blaze.jsonl`; a ClientHello then close is a failure.

### Task 4: Handle the first observed bootstrap route

**Files:**

- Modify: `tools/cypress-servers/internal/cfb27blaze/handlers.go`
- Modify: `tools/cypress-servers/internal/cfb27blaze/service_test.go`
- Modify: `Docs/cfb27-protocol-discovery.md`

- [ ] **Step 1: Write the failing message-ID test**

```go
func TestLocalLoginPreservesMessageID(t *testing.T) {
    response := handle(t, blaze.Frame{Header: blaze.Header{Component: 1, Command: 10, MessageType: blaze.MessageRequest, MessageID: 42}})
    if response.Header.MessageID != 42 { t.Fatalf("message id=%d", response.Header.MessageID) }
}
```

- [ ] **Step 2: Verify RED then implement the exact observed response**

Run: `go test ./internal/cfb27blaze -run TestLocalLoginPreservesMessageID -v`

Use existing deterministic identity/TDF helpers, preserve request message ID, and log `handled-local-bootstrap`. Do not guess later component responses.

- [ ] **Step 3: Verify and document the next live route**

Run: `go test ./... && cmake --build Server/build --config Release --target CFB27BridgeTests Cypress && Server\build\Release\CFB27BridgeTests.exe`

Append only the resulting next component/command to `Docs/cfb27-protocol-discovery.md`.

## Plan Self-Review

- Tasks cover capture recovery, guarded downgrade, local plaintext handling, and the first login response.
- Matchmaking, gameplay replication, and later mode-specific feature gates are deliberately outside this bootstrap milestone.
