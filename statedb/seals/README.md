This document describes community-generated test vectors concerning Block sealing for primary (T=1) and fallback (T=0), how to interpret their fields, and how to test/verify them.

[JAM Graypaper 6.4](https://graypaper.fluffylabs.dev/#/579bd12/0eae000eae00) is the primary section being tested.

These files are contributed by [Colorful Notion (JAM DUNA)](https://github.com/jam-duna/jamtestnet) and are not official.  If you find issues, let us know in the PR and we'll try to correct them within a day or two.

If you think they are reasonable and can pass them, say so!  

## Overview

When sealing a block, as per GP Section 6, two VRF Signatures:

* H_s – the seal proof
* H_v – the entropy source proof

These proofs are produced by signing (with a VRF) various inputs (c, m) derived from the block header, ticket IDs, entropy.



## JSON Structure

Each JSON file generated has the following fields:

```
Field	JSON Key	Description
BlockAuthorPub	bandersnatch_pub	Hex-encoded Bandersnatch public key of the block author. In production, you typically store just the public portion.
BlockAuthorPriv	bandersnatch_priv	Hex-encoded Bandersnatch secret key (private key). Storing private keys in production JSON is not recommended; it is done here for debugging purposes.
TicketID	ticket_id	Hex-encoded ticket identifier used for verifying or associating with a block’s VRF entropies.
CForHs	c_for_H_s	VRF input c used to generate H_s.
MForHs	m_for_H_s	VRF message m used to generate H_s.
Hs	H_s	Hex-encoded proof output for H_s (the VRF signature/proof itself).
CForHv	c_for_H_v	VRF input c used to generate H_v.
MForHv	m_for_H_v	VRF message m used to generate H_v. (Often empty in primary epoch flows.)
Hv	H_v	Hex-encoded proof output for H_v (the VRF signature/proof itself).
T	T	Integer indicator for epoch type (1 = primary epoch, 0 = fallback epoch).
HeaderBytes	header_bytes	Hex-encoded serialized block header bytes at the time of sealing.
```

## Example JSON


```json
{
  "bandersnatch_pub": "1a2b3c4d5e6f...",
  "bandersnatch_priv": "0a1b2c3d4e5f...",
  "ticket_id": "7469636b65742d69642d68657265...",
  "c_for_H_s": "abcdef123456",
  "m_for_H_s": "0203040506...",
  "H_s": "deadbeef...",
  "c_for_H_v": "1234abcd...",
  "m_for_H_v": "",
  "H_v": "baadf00d...",
  "T": 1,
  "header_bytes": "fffe0102..."
}
```

### Verifying the Seal and Testing Block Sealing

* Read the JSON file (e.g., seals/1-0.json).
* Parse the hex-encoded fields into byte slices.

Verify Seal:
* Use the public key (bandersnatch_pub) along with the stored VRF proofs (H_s, H_v) and block header to test your block seal verifier.  Note that TWO IETF Verify checks are required, one for H_s and one for H_v.

Check your Block Sealer:
* use the supplied `bandersnatch_priv`, `ticket_id` field (if `T`=1) and `header_bytes` to compute `H_s` and `H_v`








