## Export

### Error case:

#### `ExportOK`
- **Description:** Successfully executed.
- **Action:** Write the value `ς + |e|` into `omega_7` and append x to e.

#### `ExportOOB`
- **Description:** Access permissions error.
- **Action:** Write `OOB` into `omega_7` and e remain the same.

#### `ExportFULL`
- **Description:** Preimage could not be retrieved (x > t).
- **Action:** Write `FULL` into `omega_7`and e remain the same.

---

### Key Parameters
#### Note 
`W_G` is `24` in our tiny set

#### Initial Export segment
  ```json
  [[]]
  ```

#### Expected Export segment
  ```json
[
    [],
    [
      10,
      10,
      11,
      11,
      12,
      12,
      13,
      13,
      14,
      14,
      15,
      15,
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
    ]
  ]
  ```

#### Initial SegmentIdx

  ```json
  "initial-export-segment-index": 0,
  ```