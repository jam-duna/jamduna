## Import

### Error case:

#### `ImportOK`
- **Description:** Successfully executed.
- **Action:** Write the value `OK` into `omega_7` and append x to e.

#### `ImportOOB`
- **Description:** Access permissions error.
- **Action:** Write `OOB` into `omega_7`.

#### `ImportNONE`
- **Description:** import segment index is wrong.
- **Action:** Write `NONE` into `omega_7`.

---

### Key Parameters
#### Note 
`W_G` is `24` in our tiny set

#### Initial Import segment
  ```json
  "initial-import-segment": [
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
      15
    ]
  ],
  ```