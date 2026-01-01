# Deferred Transfers

This document specifies the deferred transfer format used between services.
A deferred transfer always moves value from one service to another, and the
destination type determines how recipients identify the transfer.

## Destination Types

There are two destination types:

```rust
pub enum Destination {
    /// EVM service + 20-byte address (account-based)
    Evm {
        service_id: u32,
        address: [u8; 20],
    },

    /// Orchard service + 32-byte note id (note-based)
    Orchard {
        service_id: u32,
        note_id: [u8; 32],
    },
}

pub struct DeferredTransfer {
    pub from_service_id: u32,
    pub to: Destination,
    pub amount: u64,
}
```

## Note ID Rules (Orchard)

The `note_id` is a 32-byte identifier that may refer to an unspent note in
another service or an external system. It is not required to be an Orchard
commitment (`cmx`) or a nullifier. It is an explicit cross-service identifier
chosen by the sender.

Incoming Orchard transfers treat the memo `note_id` as the primary identifier.
To make outgoing transfers symmetric:

- The sender MUST supply a 32-byte `note_id` in the destination.
- The Orchard output memo MUST embed the same `note_id` (verbatim).
- Wallets index both incoming and outgoing transfers by the `note_id`.

This keeps note identification consistent across directions while staying
explicit in the transfer object.

## Memo Format for Deferred Transfers

Orchard memos are 512 bytes. This format uses a 1-byte memo type followed by
memo data:

```
[1 byte: 0xFE memo type]
[32 bytes: note_id]
[4 bytes: from_service_id (little endian)]
[475 bytes: zero padding]
```

The `note_id` appears at offset 1 in the memo payload.

## Encoding

Minimal wire format suggestion:

```
destination_tag (1 byte)
to_service_id  (4 bytes, little endian)
destination    (20 bytes if EVM, 32 bytes if Orchard)
amount         (8 bytes, little endian)
```

`destination_tag` is `0x01` for EVM, `0x02` for Orchard.

## Cross-Service Flow

EVM → Orchard:
1. EVM service emits `DeferredTransfer` with a chosen `note_id`.
2. Orchard service creates a note whose memo embeds `note_id`.
3. Recipient wallet scans memos and indexes the transfer by `note_id`.

Orchard → EVM:
1. Orchard bundle spends notes and emits `DeferredTransfer` to an EVM address.
2. EVM service credits the target account upon processing the transfer.

## Amount Limits

The `amount` field is `u64`. Destination services MUST reject transfers that
cannot be represented safely in their local value types. EVM services that use
`u256` still accept only the `u64` range defined here.

## Validation Rules

- `from_service_id` and `to.service_id` MUST be valid registered services.
- EVM destinations MUST supply a 20-byte address.
- Orchard destinations MUST supply a 32-byte note_id and embed it in memo.
- Amounts MUST be non-zero and within the destination service's limits.

## Security and Privacy Notes

- Destination services MUST prevent replay by recording consumed transfers
  (e.g., by hashing the transfer payload or tracking `note_id`).
- Orchard nullifiers still prevent double-spends within Orchard; cross-service
  replay protection is handled at the deferred-transfer layer.
- Deferred transfers are public. Embedding `note_id` in memos makes linking
  between services possible, which is a privacy trade-off for interoperability.
