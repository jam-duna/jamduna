# Host Funtion test cases
## host_read_vlen.json:

- The value of v has been successfully read.
- Write the length of v into r0.
- Write the value of v to the VM's RAM, address range 0x20000-0x20003.

**Initial non-zero registers:**

    * r0 = 7 (s)
    * r1 = 131072 (ko)
    * r2 = 4 (kz)
    * r3 = 131072 (bo)
    * r4 = 10 (bz)

**Initial page map:**

   * RW: 0x20000-0x21000 (0x1000 bytes)

**Initial non-zero memory chunks:**

   * 0x20000-0x20004 (0x4 bytes) = [0x12, 0x34, 0x56, 0x78]

**Registers after execution (only changed registers):**

   * r0 = 3

**Final non-zero memory chunks:**

   * 0x20000-0x20003 (0x3 bytes) = [0xC, 0x22, 0x38]

## host_read_NONE.json:

- k does not exist in the key of service_account_storage, so v is ∅.
- Write NONE(2^32 - 1) into r0.
- RAM remains unchanged.

**Initial non-zero registers:**

    * r0 = 100 (s)
    * r1 = 131072 (ko)
    * r2 = 4 (kz)
    * r3 = 131072 (bo)
    * r4 = 10 (bz)

**Initial page map:**

   * RW: 0x20000-0x21000 (0x1000 bytes)

**Initial non-zero memory chunks:**

   * 0x20000-0x20004 (0x4 bytes) = [0x12, 0x34, 0x56, 0x78]

**Registers after execution (only changed registers):**

   * r0 = 4294967295

**Final non-zero memory chunks:**

   * 0x20000-0x20004 (0x4 bytes) = [0x12, 0x34, 0x56, 0x78]

## host_read_OOB.json:

- The RAM cannot be read from positions ko to kz, so k is ∇ and v is ∅.
- Write OOB(2^32 - 2) into r0.
- RAM remains unchanged.

**Initial non-zero registers:**

    * r0 = 7 (s)
    * r1 = 131072 (ko)
    * r2 = 4 (kz)
    * r3 = 131072 (bo)
    * r4 = 10 (bz)

**Initial page map:**

   * RW: 0x20000-0x2002 (0x2 bytes)

**Initial non-zero memory chunks:**

   * 0x20000-0x20004 (0x4 bytes) = [0x12, 0x34, 0x56, 0x78]

**Registers after execution (only changed registers):**

   * r0 = 4294967294

**Final non-zero memory chunks:**

   * 0x20000-0x20004 (0x4 bytes) = [0x12, 0x34, 0x56, 0x78]