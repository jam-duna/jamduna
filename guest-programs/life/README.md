# Conway's Game of Life – 9 Page Variant with Keccak256 Hash Chain Initialization

This project is an implementation of Conway's Game of Life that runs
in a web browser and within a Rust program. See
[playgameoflife.com](https://playgameoflife.com/) for a visual
demonstration and [Conway's Game of Life on
Wikipedia](https://en.wikipedia.org/wiki/Conway%27s_Game_of_Life) for
background.  Remarkably, with just a few rules, you can use this world
to build gates, memory cells and basically compute everything.  I
(Sourabh) learned about this when I was 17 in 1989 in my first
computer science class, Structure and Interpretation of Computer
Programs from Gerald Sussman.  It is an inspiring reminder of how
something simple rules can be used build complex things, not unlike
JAM itself.


The grid is 192×192 cells, divided into 9 pages (each page is 4096
bytes). Instead of a random or preset initialization, the grid is
seeded with a Keccak256 hash chain. Each hash (32 bytes) is converted
so that each byte becomes either 0 or 1 based on whether its
most-significant bit is set.


## Features

- **Keccak256 Hash Chain Initialization:**  
  Generates 1,152 Keccak256 hashes (starting from the empty string) to fill a total of 36,864 bytes. Each byte is then mapped to 0 or 1 (if the MSB is set, the cell is 1; otherwise, 0). This chain is evenly split into 9 pages.

- **Game of Life Simulation:**  
  Uses Conway's rules on a 192×192 grid (with ghost borders to simplify neighbor calculations).

- **Debug Output:**  
  - Every **100 steps:** The computed Keccak256 hash of each page is logged to the console.
  - Every **10,000 steps:** The full hexadecimal contents of each page are logged.

## Files

- **`index.html`**  
  Contains the complete HTML and JavaScript code, including the modified `initHashChain` function that mirrors the behavior of the Rust version.

## Browser code

1. **Open index.html**  
   Simply open [index.html](./index.html) in your preferred web browser. 

2. **Watch the Simulation!**  
   The simulation starts automatically. The grid updates continuously and debug output is printed to the browser console.

3. **Check page hashes**  
   Open the browser console to see:
   - The Keccak256 hash of each page printed every 100 steps.
   - The full contents of each page printed every 10,000 steps.

Ideally we'd see segment page hashes instead

## Rust code

Setup:
```
export LD_LIBRARY_PATH=$(rustc --print sysroot)/lib
```

Build:
```
cargo build --release
```

Run:
```
./target/debug/life
```




