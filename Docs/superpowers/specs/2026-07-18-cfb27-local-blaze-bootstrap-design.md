# CFB27 Local Blaze Bootstrap Design

**Goal:** Establish a local authenticated CFB27 Blaze session and use the resulting request sequence to enable the online front-end before implementing individual online modes.

## Evidence

- The installed July 16 CFB27 executable has SHA-256 `A048578530F7ED5967DF38803B63AD9B9F04FC71287F1E151C901A94AB240BFD` and is supported by the current bridge.
- The bridge redirects the Blaze redirector connection to `127.0.0.1:27920`, where the local service receives a TLS ClientHello but the client closes during certificate processing. No Blaze frame reaches the service.
- `protossl_dump_1783976660.acp` is a plaintext capture made with the supplied EA-MITM project. Its verified CFB27 ProtoSSL RVAs are `Connect=0x16D0DD0`, `Send=0x16D15F0`, and `Recv=0x16D14E0`.
- The capture contains the live authentication request `component 0x0001 / command 0x000A`, then the CFB-specific component families needed to determine the online bootstrap order.
- `CollegeFB27-current-20260716-212536.dump` is a sparse raw memory image with the game executable mapped at `0x140000000`; it can validate the above RVAs and their code bytes without depending on a different build.

## Scope

This milestone ends only when a local redirected game run completes transport setup and the server records at least one valid Blaze request. The next request sequence is captured and decoded deterministically. It does not claim that Online Dynasty, Mascot Mashup, or Play a Friend is playable yet.

## Architecture

### 1. Capture recovery

The ACP reader will reassemble TCP streams by directional four-tuple before parsing Fire2 frames. It will preserve sequence order and emit a transcript containing frame headers and bounded payload bytes. This replaces its current packet-local parsing, which discards nearly all split frames.

The real ACP becomes a fixture only through compact, checked-in derived transcript data. The 42 MB personal capture remains outside the repository.

### 2. Local-only ProtoSSL transport downgrade

The bridge will hook `ProtoSSLConnect` at a validated RVA, but only when the target connection was already marked as a Cypress Blaze redirect. The hook will call the original function with `secure=0`; all production and non-redirected calls pass through unchanged.

The choice is deliberately transport-local: it avoids bypassing certificate validation, does not change validation behavior for EA services, and does not modify executable code. The hook is installed only after code-byte validation against the known executable build. Failed validation leaves the game untouched and writes a diagnostic event.

### 3. Dual-mode local Blaze listener

The local service will accept either its existing TLS flow or a plaintext Fire2 connection. Plaintext is permitted only on the loopback listener. Connection logging records selected transport, complete frame headers, and a bounded payload preview. The client does not need a certificate in the plaintext path.

### 4. Bootstrap response catalog

After the first local Blaze request is recorded, the server will implement only the observed bootstrap calls in request order. Responses are generated from named TDF builders and a deterministic `LocalPlayer` session/entitlement profile. Unsupported calls retain explicit diagnostics rather than returning a plausible empty success.

The supplied Blaze reference source is used to check component names and authentication response fields; it is not incorporated or built as part of Cypress.

## Safety and Compatibility

- Hooks are gated by the supported executable SHA-256 and validated RVAs.
- No production endpoint is downgraded, redirected, or certificate-bypassed.
- The game executable, game data, and the supplied artifacts are read-only inputs.
- All test fixtures are synthetic or derived metadata; no account token, capture payload, or executable dump is committed.
- The user can disable the local plaintext path with `CYPRESS_CFB27_ENABLE_LOCAL_PLAINTEXT=0`.

## Testing and Verification

1. Unit tests cover TCP reassembly across packet boundaries, direction separation, and malformed stream limits.
2. Native bridge tests cover supported-RVA validation and reject non-local/non-redirected downgrade attempts.
3. Service tests cover plaintext frame read/write and ensure remote listeners reject plaintext mode.
4. An integration test connects a plaintext client, sends the captured login header shape, and asserts the service records a decoded request and replies with the same message ID.
5. A real game run is successful only when the event log contains `frame` after a local redirect, not merely a ClientHello or a connection acceptance.

## Deferred Work

- Exact online-mode feature gates after authenticated bootstrap.
- GameManager/matchmaking, peer relay, and gameplay session replication for Play a Friend and Mascot Mashup.
- Online Dynasty persistence semantics beyond the existing local REST catalogue.
