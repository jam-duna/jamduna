## Solicit

### Error cases
#### `SolicitOK`
- **Description:** Successfully executed and append a timeslot into lookup.
- **Action:** Write the value `OK` into `omega_7`.

#### `SolicitOOB`
- **Description:** Access permissions error.
- **Action:** Write `OOB` into `omega_7`.

#### `SolicitWHUH`
- **Description:** Lookup cannot be found or does not meet the conditions.
- **Action:** Write `HUH` into `omega_7`.

#### `SolicitFULL`
- **Description:** Balance Insufficient.
- **Action:** Write `FULL` into `omega_7`.

---

### Key Parameters
#### Note
**Storage, Preimage, Lookup keys in the structure are serialized keys.**

#### Timeslot
Initial timeslot  is as follows, will vary depending on the case
```json
"initial-timeslot": 100
```

#### XContent
- There are two main x contents: **initial xcontent x** and **expected xcontent x**. The **expected xcontent x** will only change upon successful execution; otherwise, it will remain the same as **initial xcontent x**.

- Within **initial xcontent x.U.D**, there is a **service account**, and the service indices is **0**.

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
              1,
              2
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
  },
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
              1,
              2,
              100
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