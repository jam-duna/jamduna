## Transfer

### Error cases
#### `TransferOK`
- **Description:** Successfully executed and append a defered transfer into xt.
- **Action:** Write the value `OK` into `omega_7`.

#### `TransferOOB`
- **Description:** Access permissions error.
- **Action:** Write `OOB` into `omega_7`.

#### `TransferWHO`
- **Description:** Recipient does not exist.
- **Action:** Write `WHO` into `omega_7`.

#### `TransferLOW`
- **Description:** g < d[d]m.
- **Action:** Write `LOW` into `omega_7`.

#### `TransferHIGH`
- **Description:** ϱ < g.
- **Action:** Write `HIGH` into `omega_7`.

#### `TransferCASH`
- **Description:** b < (xs)t.
- **Action:** Write `CASH` into `omega_7`.

---

### Key Parameters
#### Note
**Storage, Preimage, Lookup keys in the structure are serialized keys.**

#### Gas
Initial gas is as follows, will vary depending on the case
```json
"initial-gas": 10000000000000,
```

```json
"expected-gas": 9570503270290,
```

#### XContent
- There are two main x contents: **initial xcontent x** and **expected xcontent x**. The **expected xcontent x** will only change upon successful execution; otherwise, it will remain the same as **initial xcontent x**.

- Within **initial xcontent x.U.D**, there are two **service account**, and the service indices are **0** and **1**.

- After the transfer is successfully executed, a new deferred transfer will be inserted, such as the expected `xcontent-x.T`.

##### initial-xcontent-x
  ```json
  "initial-xcontent-x": {
    "D": {},
    "I": 1,
    "S": 0,
    "U": {
      "D": {
        "0": {
          "s_map": {
            "0x008100e4007a0019e6b29b0a65b9591762ce5143ed30d0261e5d24a320175250": [
              0,
              0,
              0,
              0
            ]
          },
          "l_map": {
            "0x000c0000000000003ee347e16fb6594af50ce1cc49996bb7a5e39c7d60d5371e": [
              100000
            ]
          },
          "p_map": {
            "0x00fe00ff00ff00ffe802c135a8c671aa22d990fbb26116d9844a458d89d05aa0": [
              15,
              15,
              14,
              14,
              13,
              13,
              12,
              12,
              11,
              11,
              10,
              10
            ]
          },
          "code_hash": "0x0be802c135a8c671aa22d990fbb26116d9844a458d89d05aa013272a64160337",
          "balance": 1000,
          "min_item_gas": 100,
          "min_memo_gas": 200
        },
        "1": {
          "s_map": {
            "0x011d00bd007d000b561a41d23c2a469ad42fbd70d5438bae826f6fd607413190": [
              0,
              0,
              0,
              0
            ]
          },
          "l_map": {
            "0x010c0000000000003ee347e16fb6594af50ce1cc49996bb7a5e39c7d60d5371e": [
              100000
            ]
          },
          "p_map": {
            "0x01fe00ff00ff00ffe802c135a8c671aa22d990fbb26116d9844a458d89d05aa0": [
              15,
              15,
              14,
              14,
              13,
              13,
              12,
              12,
              11,
              11,
              10,
              10
            ]
          },
          "code_hash": "0x0be802c135a8c671aa22d990fbb26116d9844a458d89d05aa013272a64160337",
          "balance": 1000,
          "min_item_gas": 100,
          "min_memo_gas": 100
        }
      },
      "I": [],
      "Q": [
        [
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000"
        ],
        [
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000"
        ]
      ],
      "X": {
        "chi_m": 0,
        "chi_a": 0,
        "chi_v": 0,
        "chi_g": {}
      }
    },
    "T": []
  ```

##### expected-xcontent-x

  ```json
"expected-xcontent-x": {
    "D": {},
    "I": 1,
    "S": 0,
    "U": {
      "D": {
        "0": {
          "s_map": {
            "0x008100e4007a0019e6b29b0a65b9591762ce5143ed30d0261e5d24a320175250": [
              0,
              0,
              0,
              0
            ]
          },
          "l_map": {
            "0x000c0000000000003ee347e16fb6594af50ce1cc49996bb7a5e39c7d60d5371e": [
              100000
            ]
          },
          "p_map": {
            "0x00fe00ff00ff00ffe802c135a8c671aa22d990fbb26116d9844a458d89d05aa0": [
              15,
              15,
              14,
              14,
              13,
              13,
              12,
              12,
              11,
              11,
              10,
              10
            ]
          },
          "code_hash": "0x0be802c135a8c671aa22d990fbb26116d9844a458d89d05aa013272a64160337",
          "balance": 900,
          "min_item_gas": 100,
          "min_memo_gas": 200
        },
        "1": {
          "s_map": {
            "0x011d00bd007d000b561a41d23c2a469ad42fbd70d5438bae826f6fd607413190": [
              0,
              0,
              0,
              0
            ]
          },
          "l_map": {
            "0x010c0000000000003ee347e16fb6594af50ce1cc49996bb7a5e39c7d60d5371e": [
              100000
            ]
          },
          "p_map": {
            "0x01fe00ff00ff00ffe802c135a8c671aa22d990fbb26116d9844a458d89d05aa0": [
              15,
              15,
              14,
              14,
              13,
              13,
              12,
              12,
              11,
              11,
              10,
              10
            ]
          },
          "code_hash": "0x0be802c135a8c671aa22d990fbb26116d9844a458d89d05aa013272a64160337",
          "balance": 1000,
          "min_item_gas": 100,
          "min_memo_gas": 100
        }
      },
      "I": [],
      "Q": [
        [
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000"
        ],
        [
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000",
          "0x0000000000000000000000000000000000000000000000000000000000000000"
        ]
      ],
      "X": {
        "chi_m": 0,
        "chi_a": 0,
        "chi_v": 0,
        "chi_g": {}
      }
    },
    "T": [
      {
        "sender_index": 0,
        "receiver_index": 1,
        "amount": 100,
        "memo": [
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0,
          0
        ],
        "gas_limit": 100
      }
    ]
  ```