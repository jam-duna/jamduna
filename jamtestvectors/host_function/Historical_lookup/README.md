## Historical Lookup

### Error cases
#### `historical_lookupOK`
- **Description:** Successfully executed.
- **Action:** Write the value `|v|` into `omega_7`.

#### `historical_lookupOOB`
- **Description:** Access permissions error.
- **Action:** Write `OOB` into `omega_7`.

#### `historical_lookupNONE`
- **Description:** Preimage could not be retrieved (x > t).
- **Action:** Write `NONE` into `omega_7`.

---

### Key Parameters
#### Note
**Storage, Preimage, Lookup keys in the structure are serialized keys.**

#### Service Account in d
The service account is stored in the variable `d` and `service index` is `0`, the structure is defined as follows:

#### Storage dict (`s_map`)
  ```json
  "s_map": {
      "0x008100e4007a0019e6b29b0a65b9591762ce5143ed30d0261e5d24a320175250": [
          0,
          0,
          0,
          0
      ]
  }
  ```

#### Lookup dict (`l_map`)
  ```json
  "l_map": {
      "0x000c0000000000003ee347e16fb6594af50ce1cc49996bb7a5e39c7d60d5371e": [
          0
      ]
  }
  ```

#### Preimage dict (`p_map`)
  ```json
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
  }
  ```

#### Code Hash (c)
  ```json
  "code_hash": "0x00fe00ff00ff00ffe802c135a8c671aa22d990fbb26116d9844a458d89d05aa0"
  ```

#### Balance (b)

  ```json
  "balance": 100
  ```

#### Minimum Item Gas (g)
  ```json
  "min_item_gas": 100
  ```

#### Minimum Memo Gas (m)

  ```json
  "min_memo_gas": 100
  ```

