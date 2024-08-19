## Host function Test Vectors

This "hostfunctions" directory has starting points to implement 23 host functions described in Appendix B using the `ecalli` opcode (0x4e = 78)

```
GAS = 0.       host_gas.txt 
LOOKUP = 1.    host_lookup.txt 
READ = 2.      host_read_write.txt
WRITE = 3.     host_read_write.txt
INFO = 4       host_info.txt
EMPOWER = 5.   host_empower.txt
ASSIGN = 6     host_assign.txt
DESIGNATE = 7  host_designate.txt
CHECKPOINT = 8 host_checkpoint.txt
NEW = 9.       host_new.txt
UPGRADE = 10.  host_upgrade.txt
TRANSFER = 11. host_transfer.txt
QUIT = 12.     host_quit.txt
SOLICIT = 13.  host_solicit.txt
FORGET = 14.   host_forget.txt
HISTORICAL_LOOKUP = 15 host_historical_lookup.txt
IMPORT = 16    host_import_export.txt
EXPORT = 17.   host_import_export.txt
MACHINE = 18.  host_machine.txt
PEEK = 19.     host_peek.txt
POKE = 20.     host_poke.txt
INVOKE = 21.   host_invoke.txt
EXPUNGE = 22.  host_expunge.txt
```

### Assembly

`make assemble` will generate the compiled .polkavm code for the above assembly, and dump 

```

###  Disassemble

`make disassemble` dumps the disassembled code.  Although it shows "// INVALID", its fine.

```
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/read_write.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/read_write.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 33
// Code size = 117 bytes

      :                          @0 [export #0: 'main']
     0: 26 02 00 40 2a           u32 [0x4000] = 42
     5: 26 03 00 00 02 78 56 34 12 u32 [0x20000] = 305419896
    14: 04 07 00 00 02           a0 = 0x20000
    19: 04 08 04                 a1 = 0x4
    22: 04 09 00 40              a2 = 0x4000
    26: 04 0a 04                 a3 = 0x4
    29: 4e 03                    ecalli 3 // INVALID
    31: 04 07 31                 a0 = 0x31
    34: 04 08 00 00 02           a1 = 0x20000
    39: 04 09 04                 a2 = 0x4
    42: 04 0a 00 00 01           a3 = 0x10000
    47: 04 0b 04                 a4 = 0x4
    50: 4e 02                    ecalli 2 // INVALID
    52: 0a 0a 00 00 01           a3 = u32 [0x10000]
    57: 04 07 01                 a0 = 0x1
    60: 22 77 09                 a2 = a0 * a0
    63: 08 9a 0a                 a3 = a3 + a2
    66: 04 07 03                 a0 = 0x3
    69: 22 77 09                 a2 = a0 * a0
    72: 08 9a 0a                 a3 = a3 + a2
    75: 04 07 05                 a0 = 0x5
    78: 22 77 09                 a2 = a0 * a0
    81: 08 9a 0a                 a3 = a3 + a2
    84: 04 07 07                 a0 = 0x7
    87: 22 77 09                 a2 = a0 * a0
    90: 08 9a 0a                 a3 = a3 + a2
    93: 16 0a 00 00 02           u32 [0x20000] = a3
    98: 04 07 00 00 03           a0 = 0x30000
   103: 04 08 04                 a1 = 0x4
   106: 04 09 00 00 02           a2 = 0x20000
   111: 04 0a 04                 a3 = 0x4
   114: 4e 03                    ecalli 3 // INVALID
   116: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/import_export.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.05s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/import_export.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 10
// Code size = 28 bytes

      :                          @0 [export #0: 'main']
     0: 04 07                    a0 = 0x0
     2: 04 08 00 30              a1 = 0x3000
     6: 04 09 04                 a2 = 0x4
     9: 4e 10                    ecalli 16 // INVALID
    11: 0a 0a 00 30              a3 = u32 [0x3000]
    15: 22 aa 0b                 a4 = a3 * a3
    18: 04 07 00 70              a0 = 0x7000
    22: 04 08 04                 a1 = 0x4
    25: 4e 11                    ecalli 17 // INVALID
    27: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/solicit.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.05s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/solicit.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 12
// Code size = 74 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 00 40              a0 = 0x4000
     4: 04 08 04                 a1 = 0x4
     7: 26 02 00 40 43 44 7e 26  u32 [0x4000] = 645809219
    15: 26 02 04 40 87 38 1a fc  u32 [0x4004] = 4229576839
    23: 26 02 08 40 90 10 eb 9f  u32 [0x4008] = 2682982544
    31: 26 02 0c 40 89 78 1e af  u32 [0x400c] = 2938009737
    39: 26 02 10 40 32 d9 df 56  u32 [0x4010] = 1457510706
    47: 26 02 14 40 ba dc cd 04  u32 [0x4014] = 80600250
    55: 26 02 18 40 32 6e 8d 81  u32 [0x4018] = 2173529650
    63: 26 02 1c 40 35 f3 57 ee  u32 [0x401c] = 3998741301
    71: 4e 0d                    ecalli 13 // INVALID
    73: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/forget.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/forget.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 12
// Code size = 74 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 00 40              a0 = 0x4000
     4: 04 08 04                 a1 = 0x4
     7: 26 02 00 40 43 44 7e 26  u32 [0x4000] = 645809219
    15: 26 02 04 40 87 38 1a fc  u32 [0x4004] = 4229576839
    23: 26 02 08 40 90 10 eb 9f  u32 [0x4008] = 2682982544
    31: 26 02 0c 40 89 78 1e af  u32 [0x400c] = 2938009737
    39: 26 02 10 40 32 d9 df 56  u32 [0x4010] = 1457510706
    47: 26 02 14 40 ba dc cd 04  u32 [0x4014] = 80600250
    55: 26 02 18 40 32 6e 8d 81  u32 [0x4018] = 2173529650
    63: 26 02 1c 40 35 f3 57 ee  u32 [0x401c] = 3998741301
    71: 4e 0e                    ecalli 14 // INVALID
    73: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/historical_lookup.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.05s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/historical_lookup.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 14
// Code size = 81 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 31                 a0 = 0x31
     3: 26 02 00 40 78 56 34 12  u32 [0x4000] = 305419896
    11: 26 02 04 40 78 56 34 12  u32 [0x4004] = 305419896
    19: 26 02 08 40 78 56 34 12  u32 [0x4008] = 305419896
    27: 26 02 0c 40 78 56 34 12  u32 [0x400c] = 305419896
    35: 26 02 10 40 78 56 34 12  u32 [0x4010] = 305419896
    43: 26 02 14 40 78 56 34 12  u32 [0x4014] = 305419896
    51: 26 02 18 40 78 56 34 12  u32 [0x4018] = 305419896
    59: 26 02 1c 40 78 56 34 12  u32 [0x401c] = 305419896
    67: 04 08 00 40              a1 = 0x4000
    71: 04 09 00 50              a2 = 0x5000
    75: 04 0a 04                 a3 = 0x4
    78: 4e 0f                    ecalli 15 // INVALID
    80: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/lookup.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/lookup.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 14
// Code size = 81 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 31                 a0 = 0x31
     3: 26 02 00 40 78 56 34 12  u32 [0x4000] = 305419896
    11: 26 02 04 40 78 56 34 12  u32 [0x4004] = 305419896
    19: 26 02 08 40 78 56 34 12  u32 [0x4008] = 305419896
    27: 26 02 0c 40 78 56 34 12  u32 [0x400c] = 305419896
    35: 26 02 10 40 78 56 34 12  u32 [0x4010] = 305419896
    43: 26 02 14 40 78 56 34 12  u32 [0x4014] = 305419896
    51: 26 02 18 40 78 56 34 12  u32 [0x4018] = 305419896
    59: 26 02 1c 40 78 56 34 12  u32 [0x401c] = 305419896
    67: 04 08 00 40              a1 = 0x4000
    71: 04 09 00 50              a2 = 0x5000
    75: 04 0a 04                 a3 = 0x4
    78: 4e 01                    ecalli 1 // INVALID
    80: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/assign.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/assign.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 4
// Code size = 10 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 01                 a0 = 0x1
     3: 04 08 00 40              a1 = 0x4000
     7: 4e 06                    ecalli 6 // INVALID
     9: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/checkpoint.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/checkpoint.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 2
// Code size = 3 bytes

      :                          @0 [export #0: 'main']
     0: 4e 08                    ecalli 8 // INVALID
     2: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/designate.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/designate.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 3
// Code size = 8 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 00 00 01           a0 = 0x10000
     5: 4e 07                    ecalli 7 // INVALID
     7: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/egas.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.05s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/egas.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 2
// Code size = 2 bytes

      :                          @0 [export #0: 'main']
     0: 4e                       ecalli 0 // INVALID
     1: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/empower.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.05s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/empower.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 5
// Code size = 19 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 78 56 34 12        a0 = 0x12345678
     6: 04 08 21 43 65 87        a1 = 0x87654321
    12: 04 09 00 01              a2 = 0x100
    16: 4e 05                    ecalli 5 // INVALID
    18: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/expunge.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.05s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/expunge.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 3
// Code size = 6 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 2b                 a0 = 0x2b
     3: 4e 16                    ecalli 22 // INVALID
     5: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/info.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/info.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 4
// Code size = 10 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 2a                 a0 = 0x2a
     3: 04 08 00 60              a1 = 0x6000
     7: 4e 04                    ecalli 4 // INVALID
     9: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/invoke.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/invoke.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 4
// Code size = 10 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 2b                 a0 = 0x2b
     3: 04 08 00 40              a1 = 0x4000
     7: 4e 15                    ecalli 21 // INVALID
     9: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/machine.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/machine.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 5
// Code size = 13 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 00 40              a0 = 0x4000
     4: 04 08 40                 a1 = 0x40
     7: 04 09 40                 a2 = 0x40
    10: 4e 12                    ecalli 18 // INVALID
    12: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/new.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/new.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 8
// Code size = 28 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 00 40              a0 = 0x4000
     4: 04 08 2a                 a1 = 0x2a
     7: 04 09 01                 a2 = 0x1
    10: 04 0a 78 56 34 12        a3 = 0x12345678
    16: 04 0b 02                 a4 = 0x2
    19: 04 0c 21 43 65 87        a5 = 0x87654321
    25: 4e 09                    ecalli 9 // INVALID
    27: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/peek.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/peek.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 6
// Code size = 17 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 2a                 a0 = 0x2a
     3: 04 08 00 30              a1 = 0x3000
     7: 04 09 00 20              a2 = 0x2000
    11: 04 0a 04                 a3 = 0x4
    14: 4e 13                    ecalli 19 // INVALID
    16: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/poke.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/poke.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 6
// Code size = 17 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 2a                 a0 = 0x2a
     3: 04 08 00 40              a1 = 0x4000
     7: 04 09 00 30              a2 = 0x3000
    11: 04 0a 04                 a3 = 0x4
    14: 4e 14                    ecalli 20 // INVALID
    16: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/quit.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/quit.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 4
// Code size = 10 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 2a                 a0 = 0x2a
     3: 04 08 00 40              a1 = 0x4000
     7: 4e 0c                    ecalli 12 // INVALID
     9: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/transfer.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/transfer.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 8
// Code size = 26 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 2a                 a0 = 0x2a
     3: 04 08 34 12              a1 = 0x1234
     7: 04 09 78 56              a2 = 0x5678
    11: 04 0a 23 01              a3 = 0x123
    15: 04 0b 67 45              a4 = 0x4567
    19: 04 0c 00 40              a5 = 0x4000
    23: 4e 0b                    ecalli 11 // INVALID
    25: 00                       trap
cargo run -p polkatool disassemble --show-raw-bytes hostfunctions/upgrade.polkavm
warning: /root/go/src/github.com/colorfulnotion/polkavm/Cargo.toml: unused manifest key: workspace.lints.rust.unexpected_cfgs.check-cfg
    Finished dev [unoptimized + debuginfo] target(s) in 0.04s
     Running `target/debug/polkatool disassemble --show-raw-bytes hostfunctions/upgrade.polkavm`
// RO data = 0/0 bytes
// RW data = 0/0 bytes
// Stack size = 0 bytes

// Instructions = 7
// Code size = 25 bytes

      :                          @0 [export #0: 'main']
     0: 04 07 00 40              a0 = 0x4000
     4: 04 08 78 56 34 12        a1 = 0x12345678
    10: 04 09 01                 a2 = 0x1
    13: 04 0a 02                 a3 = 0x2
    16: 04 0b 21 43 65 87        a4 = 0x87654321
    22: 4e 0a                    ecalli 10 // INVALID
    24: 00                       trap
```
