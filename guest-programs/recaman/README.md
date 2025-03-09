# Recamán Sequence in 4096-Byte Pages

This Rust program computes Recamán’s sequence and stores the results
in fixed-size pages of 4096 bytes (each page holds 512 u64
values). Instead of using a HashSet to check for duplicates, the
program scans through the previously computed pages plus the current
page buffer to ensure that each new value is unique.

This is intended to be used to be an invokable CoreVM program, where
pages are poked at the start prior to invocation, peeked afterwards,
with continuous refinement possible with an increasing number of
programs.


## How It Works

Recamán’s Sequence Starts with 0. Then, for each step n, the candidate
is calculated as a(n-1) - n if the result is positive and not already
present; otherwise, it uses a(n-1) + n.

## Page Storage:

The sequence is stored in pages, with each page holding 512 u64
numbers (4096 bytes total). When a page fills up, its Keccak-256 hash
is computed (using the tiny-keccak crate) and printed along with the
page number, the current step number, and the last Recamán value.

## Duplicate Checking:

The program scans all previously stored pages and the current page buffer to check if a candidate has already appeared.


### Build and Run

```
cargo run --release
```

The program will compute pages of Recamán numbers and print out the
page number, the current step, the last Recamán value, and the
Keccak-256 hash of each 4096-byte page.

