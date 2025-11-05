# GATEWAY.md

## Overview

Everything in JAM is an **object** identified by a 32-byte `object_id`.  
There are **no separate endpoints** for transactions, contracts, or storage — everything is addressed the same way.

You interact with objects directly via standard HTTP verbs:

| Verb | Meaning |
|------|----------|
| **GET /{object_id}** | Read an object (optionally at a specific version or state root). |
| **PUT /{object_id}** | Create a new version of an object (e.g., submit a transaction, update storage, publish code). |
| **PATCH /{object_id}** | Amend a pending object (e.g., bump fee, attach data availability refs). |
| **DELETE /{object_id}** | Cancel a pending object (best-effort). |

---

## Headers

### Version / State Resolution (for GET)
Use one of the following headers to specify how to resolve the version:

- `Resolve: version=<u32>`  
  Get that exact version.
- `Resolve: state-root=0x…`  
  Resolve to the version canonical under a given state root.
- `Resolve: timeslot=<u32>`  
  Get the latest version ≤ that timeslot.
- *(omit)* → latest version in current JAM State.

### Concurrency / Lineage (for PUT, PATCH, DELETE)
- `If-Match: "v:<u32>"`  
  Apply change only if the current version matches.

### Response Metadata
Returned on all verbs:

- `ETag: "v:<u32>"` — version resolved or created  
- `Hash: 0x…` — Work package hash containing this version  
- `Index: <u64>` — Index within that work package  
- `Status: pending | included | finalized | dropped` — lifecycle state

---

## Body Format

The body of a PUT or GET response is always a **JAM Object**, consisting of:

- 160-byte fixed metadata header  
- variable-length payload

MIME types:
- `application/jam-object` — raw binary (canonical format)  
- `application/jam-object+json` — JSON envelope (for easier authoring)

Example (JSON form):

```json
{
  "header": {
    "object_id": "0xA1...",
    "version": 13,
    "service_id": 3001,
    "type_ref": "0x12...56",
    "prev_version": "0x00...00",
    "owner": "0xFE...",
    "timeslot": 12345678,
    "payload_length": 96,
    "reserved": "0000"
  },
  "payload": "0x…"
}
```

---

## Verb Semantics

### GET /{object_id}
Retrieve the object.  
Version resolution controlled by `Resolve:` header.

**Responses**
- `200 OK` — returns the resolved object  
- `404` — not found for the requested resolution  

---

### PUT /{object_id}
Create **a new version** of the object.

**Headers**
- `If-Match: "v:<current_version>"` (or `If-None-Match: "*"`)  

**Responses**
- `201 Created` — accepted and staged (`Status: pending`)  
- `200 OK` — re-PUT of identical bytes (idempotent)  
- `409 Conflict` — predecessor mismatch or version race  
- `422 Unprocessable Entity` — invalid header linkage  

**Example: submit a transaction**
```bash
curl -X PUT https://jam.example/0xTXOBJECT   -H 'Content-Type: application/jam-object+json'   -H 'If-Match: "v:12"'   -d '{
    "header": {
      "object_id":"0xTXOBJECT",
      "version":13,
      "service_id":3001,
      "type_ref":"0x12...56",
      "prev_version":"0x00...00",
      "owner":"0xALICE...",
      "timeslot":12345678,
      "payload_length":96,
      "reserved":"0000"
    },
    "payload":"0x…"
  }'
```

---

### PATCH /{object_id}
Amend a **pending** object.  
Typical uses: bump fee, attach DA refs, adjust gas.

**Example**
```bash
curl -X PATCH https://jam.example/0xTXOBJECT   -H 'If-Match: "v:13"'   -H 'Content-Type: application/json'   -d '{ "add_da": [{"hash":"0x..","index":7}], "replace": {"maxFeePerGas":"0x..."} }'
```

**Responses**
- `200 OK` — updated pending object  
- `409 Conflict` — already finalized  

---

### DELETE /{object_id}
Cancel a **pending** object.

**Responses**
- `204 No Content` — successfully withdrawn  
- `409 Conflict` — already included/finalized  

---

## Reading by State

You can read historical versions or state roots via headers.

```bash
# Read as of a state root
curl https://jam.example/0xOBJECT   -H 'Resolve: state-root=0xSTATE'
```

---

## Minimal Status Model

`Status` header in responses reflects lifecycle:

| Status | Meaning |
|---------|----------|
| **pending** | Object accepted, awaiting inclusion |
| **included** | Present in an ordered work package |
| **finalized** | Pointer in JAM State advanced |
| **dropped** | Rejected or replaced |

---

## Intuition

- **One noun:** every entity is just `{object_id}`.  
- **No special tx endpoint:** submitting a transaction = PUT-ing a new object version.  
- **Immutability:** existing versions never change; new versions append.  
- **HTTP semantics:** version control via `If-Match`/`ETag`, history via `Resolve`.  
- **Discoverability:** clients can fetch any version, state, or header chain through the same URL form.

This makes the gateway **intuitive**, **idempotent**, and **self-documenting** — everything flows through the standard verbs without extra RPC conventions.
