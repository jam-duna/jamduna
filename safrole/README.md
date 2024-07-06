
Open cases:

# 1. publish-tickets-no-mark-1.json Expected error: Fail: Submit an extrinsic with more tickets than allowed.
```
Input Slot: 1 Y = 10
added Ticket ID 13fecb426e0a73b84b58b9a0832b11582dc971e79c5399e69f0baf1a244c7787 (1)
added Ticket ID 3a5d10abc80dda33fe3f40b3bb2e3eefd3e97dda3d617a860c9d94eb70b832ad (1)
added Ticket ID 5d91e5951d2b62c424b767220498e6d64280fc84ffb16951d93a00c009341498 (1)
Sorted -- result:
 0: 0x13fecb426e0a73b84b58b9a0832b11582dc971e79c5399e69f0baf1a244c7787
 1: 0x3a5d10abc80dda33fe3f40b3bb2e3eefd3e97dda3d617a860c9d94eb70b832ad
 2: 0x5d91e5951d2b62c424b767220498e6d64280fc84ffb16951d93a00c009341498
 -- 1 12 1 
FAIL: expected 'Fail: Submit an extrinsic with more tickets than allowed.', got 'ok'
FAIL Output mismatch: expected {
  "ok": null
}, got {
  "ok": {
    "epoch_mark": null,
    "tickets_mark": null
  }
} [ok]
FAIL PostState mismatch on publish-tickets-no-mark-1.json: timeslot mismatch expected 0, got 1
```

QUESTION: Since we cannot recover the ticket submitter, how are we supposed to identify if 3 tickets are from the same submitter?

# 2. publish-tickets-no-mark-3.json Expected error: Fail: Re-submit tickets from authority 0.

```
ticketID? 0 => 0x09a696b142112c0af1cd2b5f91726f2c050112078e3ef733198c5f43daa20d2b
ticketID? 1 => 0x3a5d10abc80dda33fe3f40b3bb2e3eefd3e97dda3d617a860c9d94eb70b832ad
ticketID? 2 => 0x5d91e5951d2b62c424b767220498e6d64280fc84ffb16951d93a00c009341498
ticketID? 3 => 0xf28c553c54ea497bb39e20344e7a687523d06390bb8a24a919183599e584e813
Input Slot: 2 Y = 10
added Ticket ID 0b07226e17e82a9e447dd914d3a072fff7015c9f58681f7f5f4213487259891f (1)
added Ticket ID 1ec38833e785b52adca8c791f8bf479c8cae805c93ce1ed245b32b273e3c9322 (1)
FAIL PostState mismatch on publish-tickets-no-mark-3.json: ticketsaccumulator mismatch on length s2 has 4, s1 has 6
```

QUESTION: Basically the same as the above -- if we have no way of recovering that tickets are from authority 0.  The ticket IDs do not appear to be resubmissions (0b07226e17e82a9e447dd914d3a072fff7015c9f58681f7f5f4213487259891f, 1ec38833e785b52adca8c791f8bf479c8cae805c93ce1ed245b32b273e3c9322) 



# 3. publish-tickets-no-mark-4.json Expected error: Fail: Submit tickets in bad order.

```
ticketID? 0 => 0x09a696b142112c0af1cd2b5f91726f2c050112078e3ef733198c5f43daa20d2b
ticketID? 1 => 0x3a5d10abc80dda33fe3f40b3bb2e3eefd3e97dda3d617a860c9d94eb70b832ad
ticketID? 2 => 0x5d91e5951d2b62c424b767220498e6d64280fc84ffb16951d93a00c009341498
ticketID? 3 => 0xf28c553c54ea497bb39e20344e7a687523d06390bb8a24a919183599e584e813
Input Slot: 2 Y = 10
added Ticket ID 1ec38833e785b52adca8c791f8bf479c8cae805c93ce1ed245b32b273e3c9322 (1)
added Ticket ID 0b07226e17e82a9e447dd914d3a072fff7015c9f58681f7f5f4213487259891f (1)
Sorted -- result:
 0: 0x09a696b142112c0af1cd2b5f91726f2c050112078e3ef733198c5f43daa20d2b
 1: 0x0b07226e17e82a9e447dd914d3a072fff7015c9f58681f7f5f4213487259891f
 2: 0x1ec38833e785b52adca8c791f8bf479c8cae805c93ce1ed245b32b273e3c9322
 3: 0x3a5d10abc80dda33fe3f40b3bb2e3eefd3e97dda3d617a860c9d94eb70b832ad
 4: 0x5d91e5951d2b62c424b767220498e6d64280fc84ffb16951d93a00c009341498
 5: 0xf28c553c54ea497bb39e20344e7a687523d06390bb8a24a919183599e584e813
 -- 2 12 2 
FAIL: expected 'Fail: Submit tickets in bad order.', got 'ok'
FAIL Output mismatch: expected {
  "ok": null
}, got {
  "ok": {
    "epoch_mark": null,
    "tickets_mark": null
  }
} [ok]
FAIL PostState mismatch on publish-tickets-no-mark-4.json: timeslot mismatch expected 1, got 2
```

QUESTION: How are we supposed to know that the tickets are in "Bad Order"?

# 4. enact-epoch-change-with-no-tickets-4.json Expected error: ok

Input Slot: 15 Y = 10
Sorted -- result:
 -- 15 12 3 
FAIL Output mismatch: expected {
  "ok": {
    "epoch_mark": {
      "entropy": "0xf164ce89b10488598cb295e4eef273fb8977722f4d6b2754b970ac77b45fa29b",
      "validators": [
        "0xaa2b95f7572875b0d0f186552ae745ba8222fc0b5bd456554bfe51c68938f8bc",
        "0xf16e5352840afb47e206b5c89f560f2611835855cf2e6ebad1acc9520a72591d",
        "0x5e465beb01dbafe160ce8216047f2155dd0569f058afd52dcea601025a8d161d",
        "0x48e5fcdce10e0b64ec4eebd0d9211c7bac2f27ce54bca6f7776ff6fee86ab3e3",
        "0x3d5e5a51aab2b048f8686ecd79712a80e3265a114cc73f14bdb2a59233fb66d0",
        "0x7f6190116d118d643a98878e294ccf62b509e214299931aad8ff9764181a4e33"
      ]
    },
    "tickets_mark": null
  }
}, got {
  "ok": {
    "epoch_mark": null,
    "tickets_mark": null
  }
} [ok]
FAIL PostState mismatch on enact-epoch-change-with-no-tickets-4.json: entropy mismatch on value 1: expected 0xf164ce89b10488598cb295e4eef273fb8977722f4d6b2754b970ac77b45fa29b, got 0x



# 5. skip-epochs-1.json Expected error: ok

```
ticketID? 0 => 0x11da6d1f761ddf9bdb4c9d6e5303ebd41f61858d0a5647a1a7bfe089bf921be9
ticketID? 1 => 0x26a08e4d0c5190f01871e0569b6290b86760085d99f17eb4e7e6b58feb8d6249
ticketID? 2 => 0x2c088bf3b4e7853c99e49636d9e7c9a351918d70bd6cdf6148b81e68f5706f68
ticketID? 3 => 0x2f016b7a5db930dabdea03aa68d2734d2fa47a0557e20d130cc1e044f8dc5796
ticketID? 4 => 0x5b8f29db76cf4e676e4fc9b17040312debedafcd5637fb3c7badd2cddce6a445
ticketID? 5 => 0x7b0aa1735e5ba58d3236316c671fe4f00ed366ee72417c9ed02a53a8019e85b8
ticketID? 6 => 0x8c039ff7caa17ccebfcadc44bd9fce6a4b6699c4d03de2e3349aa1dc11193cd7
ticketID? 7 => 0x8c35d22f459d77ca4c0b0b5035869766d60d182b9716ab3e8879e066478899a8
ticketID? 8 => 0x9dff876a4b942d0a9711d18221898f11ca39751589ebf4d49d749f6b3e493292
ticketID? 9 => 0xd86b397901605eef0229e0598759a8984f13c8d62b040e194fc5da975fd7d26e
ticketID? 10 => 0xe12c22d4f162d9a012c9319233da5d3e923cc5e1029b8f90e47249c9ab256b35
ticketID? 11 => 0xf4aac2fbe33f03554bfeb559ea2690ed8521caa4be961e61c91ac9a1530dce7a
Input Slot: 46 Y = 10
Sorted -- result:
 0: 0x11da6d1f761ddf9bdb4c9d6e5303ebd41f61858d0a5647a1a7bfe089bf921be9
 1: 0x26a08e4d0c5190f01871e0569b6290b86760085d99f17eb4e7e6b58feb8d6249
 2: 0x2c088bf3b4e7853c99e49636d9e7c9a351918d70bd6cdf6148b81e68f5706f68
 3: 0x2f016b7a5db930dabdea03aa68d2734d2fa47a0557e20d130cc1e044f8dc5796
 4: 0x5b8f29db76cf4e676e4fc9b17040312debedafcd5637fb3c7badd2cddce6a445
 5: 0x7b0aa1735e5ba58d3236316c671fe4f00ed366ee72417c9ed02a53a8019e85b8
 6: 0x8c039ff7caa17ccebfcadc44bd9fce6a4b6699c4d03de2e3349aa1dc11193cd7
 7: 0x8c35d22f459d77ca4c0b0b5035869766d60d182b9716ab3e8879e066478899a8
 8: 0x9dff876a4b942d0a9711d18221898f11ca39751589ebf4d49d749f6b3e493292
 9: 0xd86b397901605eef0229e0598759a8984f13c8d62b040e194fc5da975fd7d26e
 10: 0xe12c22d4f162d9a012c9319233da5d3e923cc5e1029b8f90e47249c9ab256b35
 11: 0xf4aac2fbe33f03554bfeb559ea2690ed8521caa4be961e61c91ac9a1530dce7a
 -- 46 12 10 
FAIL Output mismatch: expected {
  "ok": {
    "epoch_mark": {
      "entropy": "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
      "validators": [
        "0xaa2b95f7572875b0d0f186552ae745ba8222fc0b5bd456554bfe51c68938f8bc",
        "0xf16e5352840afb47e206b5c89f560f2611835855cf2e6ebad1acc9520a72591d",
        "0x5e465beb01dbafe160ce8216047f2155dd0569f058afd52dcea601025a8d161d",
        "0x48e5fcdce10e0b64ec4eebd0d9211c7bac2f27ce54bca6f7776ff6fee86ab3e3",
        "0x3d5e5a51aab2b048f8686ecd79712a80e3265a114cc73f14bdb2a59233fb66d0",
        "0x7f6190116d118d643a98878e294ccf62b509e214299931aad8ff9764181a4e33"
      ]
    },
    "tickets_mark": null
  }
}, got {
  "ok": {
    "epoch_mark": null,
    "tickets_mark": null
  }
} [ok]
FAIL PostState mismatch on skip-epochs-1.json: entropy mismatch on value 1: expected 0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314, got 0xee155ace9c40292074cb6aff8c9ccdd273c81648ff1149ef36bcea6ebb8a3e25
```

QUESTION: What is the meaning of jumping all the way from slot 10 to slot 46, skipping from Epoch N to Epoch N+3, ie 
```
Epoch N (0..11)
Epoch N+1 (12..23)
Epoch N+2 (24..35)
Epoch N+3 (36..48) 
```
