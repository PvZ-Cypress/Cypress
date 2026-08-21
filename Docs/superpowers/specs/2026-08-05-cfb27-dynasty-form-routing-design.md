# CFB27 Dynasty Form Routing Design

## Problem

The private Blaze service now reaches Dynasty setup, settings, custom conferences, and team selection. The latest run still returns the game to the main menu because captured Dynasty commands are unanswered. Component 2098 command 533 is request-dependent: its response must match the request's `FORM` identifier. Commands 303, 305, 306, 307, and 309 are settings mutations with the same captured success response; command 302 is a captured empty success. Command 1269 is Team Builder entry, but the supplied successful capture did not exercise it.

## Design

Add a request-aware raw-response dispatch layer alongside the existing route-only raw fixtures. Register command 533 there and decode only enough of the request to select one of the three exact captured payloads for FORM 133300354, 133300355, or 133300356. Unknown or malformed FORM requests remain errors so the server never replays an incorrect schema.

Register commands 303, 305, 306, 307, and 309 as structured capture-backed success handlers returning `SMSG=""` and `SUCC=1`. Register command 302 as an empty success. Keep Team Builder command 1269 unsupported until its response contract is derived from executable metadata or a successful capture.

## Verification

Tests assert exact byte length and SHA-256 for every 533 fixture, rejection of an unknown FORM, the exact semantic fields for mutation commands, and an empty response for command 302. Build and run the focused CFB27 Blaze package before replacing the executable used by the launcher.
