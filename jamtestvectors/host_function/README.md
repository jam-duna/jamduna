# Refine Function
### `hostImportOK`
- Successfully read the import segment into memory.

### `hostImportOOB`
- Failed to write the import segment into memory.

### `hostImportNONE`
- Invalid segment index specified for import segments.

### `hostExportOK`
- Successfully read `x` from memory and appended it to the export segment.

### `hostExportOOB`
- Failed to read the `x` from memory.

### `hostExportFULL`
- Export segment index > `W_x`

---

# Accumulate Function
### `hostNewOK`
- Successfully read the code hash from memory and initialized a new service.

### `hostNewOOB`
- Failed to read the code hash from RAM.

### `hostNewCASH`
- Operation failed due to insufficient balance.

---