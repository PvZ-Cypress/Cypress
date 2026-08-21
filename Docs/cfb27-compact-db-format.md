# CFB27 Dynasty Compact-Database (FranTk form) format

Reverse-engineered 2026-08-08 from real EA captures (`form481.bin` team-filter,
`form541.bin` member registry, `form483.bin` team-change). These are the
`FORM`/`DATA` blobs the Blaze Dynasty commands exchange (533, 541, 481, 483,
432, 436, 581, 591, …). The private server currently replays captured Akron
copies of these; generating them from real state (correct team, sticky settings,
real member list) requires a codec for this format.

## Outer layer — standard Blaze TDF

The blob is a normal TDF struct (our `internal/blaze` decoder handles this part):

```
FORM (or DATA) : struct {
  DICT : map<int typeId, TypeDef>     // the embedded schema
  RIBC : int                          // seen 17 (record-id bit/byte count?)
  ROOT : int                          // the root object id (== the form id, e.g. 133300253)
  SIBC : int                          // seen 14 (string-id bit/byte count?)
  TABL : map<int objectId, Object>    // the actual objects/rows
}
```

Some replies wrap this as `{DATA: FORM...}` or carry sibling scalars
(`CAYE`, `CNFW`, `FORM`, `PFIL`, `RDON`, `SFIL`, `SUCC`, `SMSG`).

## DICT — the schema (fully decoded)

`DICT` is `map<int typeId, struct>`; each value defines one type:

```
TypeDef : struct {
  BASE : int                       // base typeId for inheritance (0 = none)
  DICT : map<str fieldName, int>   // field name -> field id (may be absent for scalar/array types)
  NAME : str                       // type name, e.g. "UIListSelectForm", "Command"
  TYPE : int                       // 0 = object type, 1 = array/collection type (name ends "[]")
}
```

Example (form481): types 65 ResponseForm, 214 Command, 227 FlowCommand
(BASE 214), 229 UIForm (BASE 65), 241 UIDataForm (BASE 229), 243
UISpreadsheetForm (BASE 241), 244 UIListSelectForm (BASE 243). These match the
`core-schemas/*.FTX` type names in `Dynasty_Assets`. Inheritance means a type's
full field set is its own `DICT` plus its base chain.

## TABL — the objects (partially decoded; the remaining work)

`TABL` is `map<int objectId, struct>`. Each object is **schema-driven**: fields
do NOT carry inline TDF type bytes (this is why the generic TDF decoder fails
with "unsupported type 141/0x8d"). Instead an object references a type and
encodes field values by field id, with each value's type taken from the schema.

Observed inside objects: nested `map<int fieldId, var>` collections, and packed
arrays delimited by a repeating 2-byte marker (e.g. `d0 7d`) — these hold the
list rows (the team list "Air Force", "Akron", … lives here in form481).

Still to pin down for a full codec:
- How an object declares its type id (header field vs. implied by referrer).
- The per-field value encoding for each schema type (int/str/enum/ref/array),
  driven by the DICT field-id → type mapping and the base chain.
- Object references vs. inline values (large varints like `a7 ac cc 8d 0b`
  look like ids/hashes into TABL or a string-intern table).
- The meaning of RIBC/SIBC (likely id widths used by the record encoding).

## Plan

1. **Decoder** (in progress): build the schema model from DICT (with inheritance),
   then decode TABL objects field-by-field using it. Validate by round-tripping
   the captured forms and extracting known content (team list from 481, the
   20-user registry from 541). No live game needed to verify.
2. **Encoder**: generate DICT+TABL from server state (session teams, settings,
   members) to replace the Akron fixtures. Validate byte-compat against captures,
   then in-game.

RE scratch work: `scratchpad/frantk/re2.mjs` (recursive TDF walker) + extracted
`form*.bin`. See [[cfb27-dynasty-list-load-wire]] and
[[cfb27-protossl-hwbp-capture]] in memory.
