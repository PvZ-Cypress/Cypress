# CFB27 45-Second Endpoint Trace Design

## Goal

Replace the current 30-second CFB27 socket trace with a 45-second endpoint-discovery capture. The capture exists only to identify network endpoints used while the user performs an Online Dynasty action such as entering Dynasty or advancing a week.

## Scope

The trace records data available from Windows networking diagnostics without decrypting application traffic:

- CFB27 process ID and executable name.
- TCP and UDP local and remote addresses and ports.
- TCP state, first-seen time, last-seen time, and observation count.
- Connections that appeared, disappeared, or changed state during the capture.
- Reverse DNS and matching forward-DNS cache names when Windows exposes them.
- A baseline snapshot at capture start and a final snapshot at capture end.
- An action-candidate list that prioritizes endpoints first observed after the baseline.

The trace does not attempt to disable anti-cheat, inject code, decrypt TLS, extract credentials, or capture HTTP/Blaze payloads.

## User Experience

The existing `Trace 30s` button becomes `Trace 45s`. One click starts the trace, disables the button while it is running, and reports the resulting evidence directory and event count when complete.

Each run continues to write the raw observations and adds derived endpoint-lifecycle and candidate reports. The Markdown summary leads with newly observed endpoints so the result can be reviewed without manually comparing hundreds of repeated socket observations.

## Output

Each `live-traces/<run-id>` directory contains:

- `netstat-events.json`: raw process-owned socket observations.
- `endpoint-lifecycle.json`: one record per endpoint/local-socket combination with first seen, last seen, count, and states.
- `new-endpoint-candidates.json`: non-loopback remote endpoints absent from the first snapshot and first observed during the action window.
- `reverse-dns.json`: reverse-DNS results plus matching Windows DNS-cache names when available.
- `event-groups.json`: aggregate counts retained for compatibility.
- `summary.json` and `summary.md`: duration, event totals, process metadata, baseline endpoints, new candidates, and lifecycle highlights.

## Error Handling

If CFB27 is not running, the trace fails immediately with a clear message instead of producing an empty run. Individual DNS lookup failures do not fail the capture. Raw socket observations are retained if report generation partially fails.

## Testing

Endpoint lifecycle and candidate derivation will be isolated as deterministic functions. Tests will cover:

- An endpoint present in the initial snapshot is not a new candidate.
- An endpoint first observed later is a candidate.
- State transitions and first/last timestamps are retained.
- Loopback and wildcard endpoints are excluded from remote candidates.
- The requested UI duration is 45 seconds.

The Windows launcher will then be built and the existing CFB27 capture flow smoke-checked.
