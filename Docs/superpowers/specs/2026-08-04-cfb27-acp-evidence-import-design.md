# CFB27 ACP Evidence Import Design

## Goal

Add a Cypress-owned, read-only importer for plaintext `.acp`/classic-PCAP files produced by the user-operated EA-MITM ProtoSSL capture. The importer must produce a redacted, ordered HTTP and Blaze evidence report sufficient to identify endpoints and reproduce request/response flow in the private server.

## Scope

The importer accepts an existing ACP file. It does not launch the game, load a DLL, bypass anti-cheat, extract TLS keys, or modify a capture file.

It parses the synthetic Ethernet/IPv4/TCP records created by EA-MITM, keeps TCP payload streams separate by source/destination tuple, and reassembles complete Blaze frames across successive capture records for each stream.

It identifies plaintext HTTP/1.0 and HTTP/1.1 request lines and response status lines. HTTP reports retain method, host, path without query values, status, direction, endpoint, timestamp, and body byte count. Blaze reports retain endpoint, direction, component, command, message type, error code, sequence order, payload size, and decoded field shape.

## Privacy and Safety

Raw ACP files contain authentication/session data and remain local, outside version control. Default report output must never include raw payload hex, HTTP bodies, authorization headers, cookies, query values, JWTs, or decoded values for sensitive Blaze tags.

Sensitive Blaze tag names include `AUTH`, `TOKN`, `BNDL`, `SESS`, `TICK`, `PASS`, `KEY`, `TKN`, and names containing `TOKEN`, `SECRET`, `COOKIE`, or `AUTH`. Reports may show a sensitive field's tag and value type but replace its value with `REDACTED`.

## Output

The existing `cfb27capture` CLI gains a default safe JSON/text report containing:

- `http`: request/response records with redacted URL/query/header data.
- `blazeFrames`: reassembled Blaze record metadata and sanitized decoded fields.
- `routes`: aggregate Blaze component/command counts.
- `skipped`: parse and reassembly reasons.

The original raw `frames` payload disclosure and `payloadHex` output are removed. A failed parse never writes a partial report to the requested output location.

## Testing

Tests construct PCAP records in memory and prove:

- HTTP method, host, and path are reported while query values and authorization headers are absent.
- A Blaze frame split across TCP records is reassembled and decoded once.
- Streams with different endpoints are never combined.
- Sensitive Blaze field values are redacted.
- Default JSON output contains no raw payload hex or known secret value.

