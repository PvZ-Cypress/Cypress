# CFB27 Offline Online Dynasty Recovery Design

**Date:** 2026-08-04
**Status:** Approved for implementation planning

## Goal

Make CFB27 Online Dynasty usable through the local Cypress private stack while
guaranteeing that the game cannot communicate with EA services during a
private-server run.

The first deliverable is a repeatable diagnostic launch that restores bridge
loading for the currently installed executable, records every request needed to
enter Online Dynasty, and fails closed on every non-loopback connection. Later
iterations implement the captured requests until a local Dynasty reaches its
hub without crashing.

## Current Evidence

- The Cypress launcher can start the local Dynasty REST service and Blaze
  bridge.
- The July 22 private run loaded Mascot mode and made Online Dynasty selectable.
- The successful July 22 proxy run used the TU2 executable and patched the
  ProtoSSL certificate decision at runtime RVA `0x016F5163` after validating a
  dump-derived instruction signature.
- The executable currently launched by Cypress has SHA-256
  `9E654AD49C4702D8F9FA4E38FD1110ABE657DD38926D4124B30C70E7D29ADFE8`, PE
  timestamp `0x6A4ACB31`, and image size `0x10D8A000`.
- The standalone bridge loads into that executable, but its TU2-only header
  gate disables both traffic handling and the ProtoSSL patch.
- The existing July 22 capture contains HTTP/TLS activity but no decoded Blaze
  frames that establish the complete Online Dynasty entry sequence. It is not
  sufficient to implement Dynasty by inference.

## Safety Boundary

Private mode is loopback-only:

- Connections to `127.0.0.0/8` and `::1` are allowed.
- Every attempted non-loopback TCP connection is intercepted before the network
  call, recorded with its original destination, and redirected to the local
  Cypress gateway only when the gateway can safely identify and handle it.
- An unidentified or unsupported connection is rejected locally. It is never
  passed through to the original destination.
- The run summary reports the attempted external destinations and proves that
  the number of permitted non-loopback connections is zero.
- The launcher stops before starting the game if the proxy DLL, local gateway,
  or offline enforcement cannot be verified.

This boundary applies only to an explicit CFB27 private-server launch. It must
not alter ordinary EA-launched sessions.

## Architecture

### 1. Runtime bridge discovery

Replace the standalone bridge's TU2 timestamp and fixed-RVA requirement with a
runtime image scanner. After the game has initialized its executable code, the
scanner searches committed executable memory for the verified ProtoSSL
certificate-decision signature.

The scanner patches only when exactly one full signature is found. Zero or
multiple matches are a hard failure and leave code unchanged. It logs the PE
metadata, match count, discovered RVA, expected bytes, patch bytes, and
post-write verification. Network enforcement does not depend on the TLS patch
and remains active if discovery fails.

Known fixed import slots may be used only after validating their expected
values. Module import-table interception remains the portable fallback for
`connect`, `WSAConnect`, `ConnectEx`, and the required WinHTTP calls.

### 2. Offline capture gateway

The local gateway records enough information to identify each access point:

- original IP address and port;
- TLS SNI, ALPN, and handshake result;
- HTTP version, host, method, path, headers needed for routing, and bounded body
  metadata;
- Blaze transport, component, command, message ID, message type, error code,
  and decoded TDF fields;
- connection lifecycle and the last successful request before a game exit or
  crash.

Sensitive identity tokens and unbounded payloads are redacted from summaries.
Raw local diagnostic files stay untracked.

### 3. One-click diagnostic launch

A repository PowerShell entry point performs these steps:

1. Build the proxy bridge and private services.
2. Create a timestamped run directory under Cypress application data.
3. Write an explicit offline bridge configuration.
4. Verify the gateway health and offline enforcement.
5. Install the proxy as `dinput8.dll` without replacing the game executable.
6. Launch CFB27 through the existing supported launch flow.
7. Continuously write bridge, gateway, Dynasty, process-exit, and run-summary
   diagnostics.

It must preserve an existing non-Cypress `dinput8.dll` through a recoverable
backup and must not delete or overwrite unrelated files.

### 4. Dynasty protocol implementation loop

Unknown requests do not receive blanket success. Each observed request is added
as a named route with one of three explicit behaviors:

- deterministic local response backed by the Dynasty REST/SQLite service;
- capture-backed empty success when the real client contract permits it;
- structured local error when the feature is intentionally unsupported.

The implementation sequence follows the client's actual request order:

1. authentication and local persona/session bootstrap;
2. online feature/presence gates;
3. Dynasty catalogue and local session selection;
4. create-or-join and game/session identifiers;
5. roster/setup state;
6. Dynasty hub state and notifications;
7. local persistence and reload.

Every newly supported route gets a synthetic protocol test. Captures, game
executables, account tokens, and personal Dynasty data are not committed.

### 5. Crash correlation

The launcher records the game PID chain, exit codes, and timestamps. If Windows
produces a crash dump, the run summary links it without copying it into the
repository. The gateway marks the last complete request, outstanding request,
and last response for the same interval. This separates a missing protocol
response from a client-side injection fault.

## Failure Handling

- Unsupported executable or ambiguous signature: keep outbound traffic blocked,
  write a precise diagnostic, and abort private launch.
- Proxy cannot be installed: abort before game launch.
- Gateway or Dynasty service unhealthy: abort before game launch.
- Unknown protocol request: log the complete bounded request and return a
  deliberate command-not-found/system error; never invent success.
- Game crash: preserve the run directory and generate a correlation summary.
- Cleanup: stop only processes started by the diagnostic run and restore only
  files whose backup ownership was recorded by that run.

## Testing

### Native bridge tests

- Finds one signature in a synthetic executable region and returns its RVA.
- Rejects zero and multiple signature matches.
- Rejects non-executable or unreadable regions.
- Verifies original bytes before applying the patch.
- Allows loopback and intercepts non-loopback IPv4, IPv6, and IPv4-mapped IPv6.

### Gateway and service tests

- Records original destination and TLS SNI.
- Decodes fragmented Blaze frames and preserves message IDs.
- Produces an explicit diagnostic for unknown routes.
- Tests every implemented Dynasty request/response pair.

### Launcher tests

- Refuses to launch when enforcement or services are unavailable.
- Creates a unique run directory and complete config.
- Preserves/restores an existing proxy DLL safely.
- Reports the game process exit and relevant diagnostics.

### Real-run acceptance gates

1. Bridge logs a unique runtime ProtoSSL signature and verified patch.
2. CFB27 reaches its main menu using only loopback connections.
3. Selecting Online Dynasty yields a decoded, ordered request transcript.
4. The transcript identifies the request or response immediately preceding the
   current crash.
5. After protocol implementation, a seeded local Dynasty reaches its hub,
   persists a local state change, and reloads after restarting the stack.
6. The final audit reports zero permitted non-loopback connections for the full
   run.

## Scope Boundaries

This work targets local Online Dynasty only. It does not connect to EA,
impersonate EA services on the public network, support public matchmaking, or
make unrelated online modes fully playable. Mascot behavior is retained only
as a regression check for the shared authentication/bootstrap path.

