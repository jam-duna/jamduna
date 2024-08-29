## Work Package Test Vectors

This "workpackages" directory has working packages

### fib Service

Runs through the Fibonacci sequence Fib[n+1] = Fib[n] + Fib[n-1]

| n   | Fib[n] | Fib[n-1] | Hex Value (12 bytes)         |
|-----|--------|----------|------------------------------|
| 1   | 1      | 0        | 0x00000001 00000001 00000000 |
| 2   | 1      | 1        | 0x00000002 00000001 00000001 |
| 3   | 2      | 1        | 0x00000003 00000002 00000001 |
| 4   | 3      | 2        | 0x00000004 00000003 00000002 |
| 5   | 5      | 3        | 0x00000005 00000005 00000003 |
| 6   | 8      | 5        | 0x00000006 00000008 00000005 |
| 7   | 13     | 8        | 0x00000007 0000000D 00000008 |
| 8   | 21     | 13       | 0x00000008 00000015 0000000D |
| 9   | 34     | 21       | 0x00000009 00000022 00000015 |
| 10  | 55     | 34       | 0x0000000A 00000037 00000022 |
| 11  | 89     | 55       | 0x0000000B 00000059 00000037 |
| 12  | 144    | 89       | 0x0000000C 00000090 00000059 |
| 13  | 233    | 144      | 0x0000000D 000000E9 00000090 |
| 14  | 377    | 233      | 0x0000000E 00000179 000000E9 |
| 15  | 610    | 377      | 0x0000000F 00000262 00000179 |
| 16  | 987    | 610      | 0x00000010 000003DB 00000262 |
| 17  | 1597   | 987      | 0x00000011 0000063D 000003DB |
| 18  | 2584   | 1597     | 0x00000012 00000A18 0000063D |
| 19  | 4181   | 2584     | 0x00000013 00001055 00000A18 |
| 20  | 6765   | 4181     | 0x00000014 00001A6D 00001055 |


This has 4 entry points:
* `refine`:
  (1) `import`s a 12 byte record (n, Fib[n] + Fib[n-1]) into 0x6000; if there is no importable data, initializes (1, 1, 0) into 0x6000
  (2) computes n | Fib[n] | Fib[n-1] in 0x6000
  (3) `export`s 1 12-byte item from 0x6000

* `accumulate` -- reads the 12-byte exported item and writes to service storage for s

* `authorize` -- assuming the work package hash has been signed with a private key `0x4c0883a69102937d6231471b5dbb6204fe5129617082790e20c3a52a9e7efed2` with a public Address of `0x627306090abaB3A6e1400e9345bC60c78a8BEf57`, this r

* `on_transfer` - investigating


```
pub @refine:
    @import_12_byte_record:    // import an item (n, Fib[n], Fib[n-1]) into 0x6000
    a0 = 1
    a1 = 0x6000
    a2 = 12  
    ecalli 16 
    a1 = 0
    jump @init if a0 != a1     // if we didn't get an importable item jump to init and then come back
    
    @fibsum:
    a1 = u32[0x6004]
    a2 = u32[0x6008]
    u32[0x6004] = a1               // compute Fib[n] as Fib[n-1]  at 0x6004
    a1 = a1 + a2
    u32[0x6008] = a1               // compute Fib[n-1] as Fib[n-2] at 0x6008
    
    a3 = 1                         // increment n at 0x6000
    a0 = u32[0x6000]
    a0 = a0 + a3
    u32[0x6000] = a0

    @export_12_byte_record:    // export 12 bytes at 0x6000
    a0 = 0x6000
    a1 = 12
    ecalli 17
    trap

    @init:                    // initialize first exported item (n, Fib[n], Fib[n-1]) with (1, 1, 0)
    u32 [0x6000] = 0x00000001
    u32 [0x6004] = 0x00000001
    u32 [0x6008] = 0x00000000
    jump @fibsum
    
pub @accumulate:
    // TODO: copy _wrangled_ 12 byte work result to 0x6000
    @TODO_wrangled_refine_results_to_0x4000:
        
    @write:
    a0 = 0x4000
    a1 = 4
    a2 = 0x5000
    a3 = 12
    u32 [0x4000] = 0x00000000
    ecalli 3
    trap

pub @authorization:
    @readsig:
    a0 = 0x1000
    a1 = 0x2000 
    a2 = 0x3000
    @setup_public_address:
    u32 [0x4000] = 0x62730609
    u32 [0x4004] = 0x0abaB3A6
    u32 [0x4008] = 0xe1400e93
    u32 [0x400c] = 0x45bC60c7
    u32 [0x4010] = 0x8a8BEf57

    // TODO: copy 65 signature (signing the workpackage hash) into 0x1000, copy 32-byte workpackage hash into 0x2000
    @TODO_copy_sig_wphash:
    
    // ecrecover will return 20byte address into 0x3000 if OK
    @ecrecover:
    ecalli 25
    a3 = 0
    
    @check_match_to_public_address:
    a0 = u32 [0x3000]
    a1 = u32 [0x4000]
    jump @notauth if a0 != a1
    
    a0 = u32 [0x3004]
    a1 = u32 [0x4004]
    jump @notauth if a0 != a1

    a0 = u32 [0x3008]
    a1 = u32 [0x4008]
    jump @notauth if a0 != a1

    a0 = u32 [0x300c]
    a1 = u32 [0x400c]
    jump @notauth if a0 != a1

    a0 = u32 [0x3010]
    a1 = u32 [0x4010]
    jump @notauth if a0 != a1
    trap
    
    @notauth:
    trap

pub @on_transfer:
    @TODO_investigating:
    trap
```

### Assembly
```
# cargo run -p polkatool -- assemble workpackages/fib.txt  -o fib.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool assemble workpackages/fib.txt -o fib.polkavm`
```

### Disassembly
```
# cargo run -p polkatool disassemble --show-raw-bytes fib.polkavm 
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.05s
     Running `target/debug/polkatool disassemble --show-raw-bytes fib.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 62
// Code size = 215 bytes

      :                          @0 [export #8: 'import_12_byte_record'] [export #13: 'refine']
     0: 04 07 01                 a0 = 0x1
     3: 04 08 00 60              a1 = 0x6000
     7: 04 09 0c                 a2 = 0xc
    10: 4e 10                    ecalli 16 // INVALID
    12: 04 08                    a1 = 0x0
    14: 1e 87 2f                 jump 61 if a0 != a1
      :                          @1 [export #7: 'fibsum']
    17: 0a 08 04 60              a1 = u32 [0x6004]
    21: 0a 09 08 60              a2 = u32 [0x6008]
    25: 16 08 04 60              u32 [0x6004] = a1
    29: 08 98 08                 a1 = a1 + a2
    32: 16 08 08 60              u32 [0x6008] = a1
    36: 04 0a 01                 a3 = 0x1
    39: 0a 07 00 60              a0 = u32 [0x6000]
    43: 08 a7 07                 a0 = a0 + a3
    46: 16 07 00 60              u32 [0x6000] = a0
    50: 11                       fallthrough
      :                          @2 [export #6: 'export_12_byte_record']
    51: 04 07 00 60              a0 = 0x6000
    55: 04 08 0c                 a1 = 0xc
    58: 4e 11                    ecalli 17 // INVALID
    60: 00                       trap
      :                          @3 [export #9: 'init']
    61: 26 02 00 60 01           u32 [0x6000] = 1
    66: 26 02 04 60 01           u32 [0x6004] = 1
    71: 26 02 08 60              u32 [0x6008] = 0
    75: 05 c6                    jump 17
      :                          @4 [export #2: 'TODO_wrangled_refine_results_to_0x4000'] [export #3: 'accumulate'] [export #15: 'write']
    77: 04 07 00 40              a0 = 0x4000
    81: 04 08 04                 a1 = 0x4
    84: 04 09 00 50              a2 = 0x5000
    88: 04 0a 0c                 a3 = 0xc
    91: 26 02 00 40              u32 [0x4000] = 0
    95: 4e 03                    ecalli 3 // INVALID
    97: 00                       trap
      :                          @5 [export #4: 'authorization'] [export #12: 'readsig']
    98: 04 07 00 10              a0 = 0x1000
   102: 04 08 00 20              a1 = 0x2000
   106: 04 09 00 30              a2 = 0x3000
   110: 11                       fallthrough
      :                          @6 [export #14: 'setup_public_address']
   111: 26 02 00 40 09 06 73 62  u32 [0x4000] = 1651705353
   119: 26 02 04 40 a6 b3 ba 0a  u32 [0x4004] = 180007846
   127: 26 02 08 40 93 0e 40 e1  u32 [0x4008] = 3779071635
   135: 26 02 0c 40 c7 60 bc 45  u32 [0x400c] = 1169973447
   143: 26 02 10 40 57 ef 8b 8a  u32 [0x4010] = 2324426583
   151: 11                       fallthrough
      :                          @7 [export #1: 'TODO_copy_sig_wphash']
   152: 4e 19                    ecalli 25 // INVALID
   154: 04 0a                    a3 = 0x0
   156: 11                       fallthrough
      :                          @8 [export #5: 'check_match_to_public_address']
   157: 0a 07 00 30              a0 = u32 [0x3000]
   161: 0a 08 00 40              a1 = u32 [0x4000]
   165: 1e 87 30                 jump 213 if a0 != a1
      :                          @9
   168: 0a 07 04 30              a0 = u32 [0x3004]
   172: 0a 08 04 40              a1 = u32 [0x4004]
   176: 1e 87 25                 jump 213 if a0 != a1
      :                          @10
   179: 0a 07 08 30              a0 = u32 [0x3008]
   183: 0a 08 08 40              a1 = u32 [0x4008]
   187: 1e 87 1a                 jump 213 if a0 != a1
      :                          @11
   190: 0a 07 0c 30              a0 = u32 [0x300c]
   194: 0a 08 0c 40              a1 = u32 [0x400c]
   198: 1e 87 0f                 jump 213 if a0 != a1
      :                          @12
   201: 0a 07 10 30              a0 = u32 [0x3010]
   205: 0a 08 10 40              a1 = u32 [0x4010]
   209: 1e 87 04                 jump 213 if a0 != a1
      :                          @13
   212: 00                       trap
      :                          @14 [export #10: 'notauth']
   213: 00                       trap
      :                          @15 [export #0: 'TODO'] [export #11: 'on_transfer']
   214: 00                       trap