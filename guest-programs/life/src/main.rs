use tiny_keccak::{Hasher, Keccak};

const PAGE_SIZE: usize = 4096; // bytes per page (each cell is 1 byte)
const PAGE_DIM: usize = 64;    // each page is 64x64 cells
const NUM_PAGES: usize = 9;
const PAGES_PER_ROW: usize = 3; // ghost layout is 3x3 pages
const TOTAL_ROWS: usize = PAGE_DIM * 3; // 64 * 3 = 192
const TOTAL_COLS: usize = PAGE_DIM * 3; // 64 * 3 = 192

/// Compute the Keccak-256 hash of `data` and return the 32-byte array.
fn keccak256_hash(data: &[u8]) -> [u8; 32] {
    let mut hasher = Keccak::v256();
    hasher.update(data);
    let mut res = [0u8; 32];
    hasher.finalize(&mut res);
    res
}

/// Convert a byte slice to a lowercase hex string.
fn bytes_to_hex(data: &[u8]) -> String {
    data.iter().map(|b| format!("{:02x}", b)).collect()
}

/// The grid holds 9 pages (each 4096 bytes) in a 3x3 layout.
struct Grid {
    // Each page is stored as [u8; PAGE_SIZE].
    pages: [[u8; PAGE_SIZE]; NUM_PAGES],
}

impl Grid {
    /// Create a new (empty) grid.
    fn new() -> Self {
        Self {
            pages: [[0; PAGE_SIZE]; NUM_PAGES],
        }
    }

    /// Initialize the grid using a Keccak256 hash chain.
    ///
    /// We need 9 pages * 4096 bytes = 36,864 bytes.
    /// Each Keccak256 hash produces 32 bytes; so we require 36,864 / 32 = 1,152 hashes.
    /// We start with the empty string, then repeatedly hash the previous result’s hex string.
    /// Then, each byte of the concatenated hash chain is converted to 0 or 1 based on whether
    /// its MSB (most-significant bit) is set.
    fn init_hash_chain(&mut self) {
        let total_bytes = PAGE_SIZE * NUM_PAGES; // 36,864 bytes.
        let num_hashes = total_bytes / 32; // 1,152 hashes.
        let mut hash_chain = vec![0u8; total_bytes];
        let mut current = String::new(); // start with empty string

        for i in 0..num_hashes {
            // Compute hash of current.
            let hash_bytes = keccak256_hash(current.as_bytes());
            // Place these 32 bytes into the hash chain buffer.
            hash_chain[i * 32..(i + 1) * 32].copy_from_slice(&hash_bytes);
            // Update current to be the hex string representation.
            current = bytes_to_hex(&hash_bytes);
        }

        // Convert each byte in the hash chain into 0 or 1 based on its MSB.
        // If the most significant bit is set (i.e. byte & 0x80 != 0), then set to 1; otherwise 0.
        for byte in hash_chain.iter_mut() {
            *byte = if (*byte & 0x80) != 0 { 1 } else { 0 };
        }

        // Distribute the hash chain bytes into the 9 pages.
        for i in 0..NUM_PAGES {
            let start = i * PAGE_SIZE;
            self.pages[i].copy_from_slice(&hash_chain[start..start + PAGE_SIZE]);
            // Debug print for each page (show full contents as hex, which will be only 0s and 1s).
            //println!("Page {} contents: {}", i, bytes_to_hex(&self.pages[i]));
        }
    }
    

    /// Perform one simulation step (Game of Life) and return a new grid.
    ///
    /// This uses a ghost border to avoid modulo arithmetic.
    fn step(&self) -> Grid {
        // Create a 2D vector (rows x cols) for the full grid with a ghost border.
        let rows_with_ghost = TOTAL_ROWS + 2;
        let cols_with_ghost = TOTAL_COLS + 2;
        let mut in_buf = vec![vec![0u8; cols_with_ghost]; rows_with_ghost];

        // Copy the 9 pages into the inner region of in_buf.
        // The pages are arranged in a 3x3 grid.
        for page in 0..NUM_PAGES {
            let page_row = page / PAGES_PER_ROW;
            let page_col = page % PAGES_PER_ROW;
            for i in 0..PAGE_DIM {
                for j in 0..PAGE_DIM {
                    let global_row = page_row * PAGE_DIM + i;
                    let global_col = page_col * PAGE_DIM + j;
                    in_buf[global_row + 1][global_col + 1] =
                        self.pages[page][i * PAGE_DIM + j];
                }
            }
        }

        // Fill in the ghost borders by wrapping.
        for j in 1..=TOTAL_COLS {
            in_buf[0][j] = in_buf[TOTAL_ROWS][j];             // top border from bottom row.
            in_buf[TOTAL_ROWS + 1][j] = in_buf[1][j];           // bottom border from top row.
        }
        for i in 1..=TOTAL_ROWS {
            in_buf[i][0] = in_buf[i][TOTAL_COLS];             // left border from rightmost col.
            in_buf[i][TOTAL_COLS + 1] = in_buf[i][1];           // right border from leftmost col.
        }
        // Corners.
        in_buf[0][0] = in_buf[TOTAL_ROWS][TOTAL_COLS];
        in_buf[0][TOTAL_COLS + 1] = in_buf[TOTAL_ROWS][1];
        in_buf[TOTAL_ROWS + 1][0] = in_buf[1][TOTAL_COLS];
        in_buf[TOTAL_ROWS + 1][TOTAL_COLS + 1] = in_buf[1][1];

        // Create output buffer (active area only).
        let mut out_buf = vec![vec![0u8; TOTAL_COLS]; TOTAL_ROWS];
        for i in 1..=TOTAL_ROWS {
            for j in 1..=TOTAL_COLS {
                let live_neighbors = 
                    in_buf[i - 1][j - 1] as u32 +
                    in_buf[i - 1][j] as u32 +
                    in_buf[i - 1][j + 1] as u32 +
                    in_buf[i][j - 1] as u32 +
                    in_buf[i][j + 1] as u32 +
                    in_buf[i + 1][j - 1] as u32 +
                    in_buf[i + 1][j] as u32 +
                    in_buf[i + 1][j + 1] as u32;
                let current = in_buf[i][j];
                out_buf[i - 1][j - 1] = if (current == 1 && (live_neighbors == 2 || live_neighbors == 3))
                    || (current == 0 && live_neighbors == 3)
                {
                    1
                } else {
                    0
                };
            }
        }

        // Pack out_buf back into 9 pages.
        let mut new_pages = [[0u8; PAGE_SIZE]; NUM_PAGES];
        for page in 0..NUM_PAGES {
            let page_row = page / PAGES_PER_ROW;
            let page_col = page % PAGES_PER_ROW;
            for i in 0..PAGE_DIM {
                for j in 0..PAGE_DIM {
                    let global_row = page_row * PAGE_DIM + i;
                    let global_col = page_col * PAGE_DIM + j;
                    new_pages[page][i * PAGE_DIM + j] = out_buf[global_row][global_col];
                }
            }
        }

        Grid { pages: new_pages }
    }
}

fn main() {
    // Create and initialize the grid of 9 pages using the hash chain.
    let mut grid = Grid::new();
    grid.init_hash_chain();

    // Simulation step counter.
    let mut steps: u64 = 0;

    // Main simulation loop.
    loop {
        // Compute one simulation step.
        grid = grid.step();
        steps += 1;

        // Poke one cell (set it to 1) based on step number modulo total cells.
        let total_cells = TOTAL_ROWS * TOTAL_COLS;
        let poke_index = (steps as usize) % total_cells;
        let poke_row = poke_index / TOTAL_COLS;
        let poke_col = poke_index % TOTAL_COLS;
        // Map the global coordinate to a page.
        let page_row = poke_row / PAGE_DIM;
        let page_col = poke_col / PAGE_DIM;
        let page_index = page_row * PAGES_PER_ROW + page_col;
        let local_row = poke_row % PAGE_DIM;
        let local_col = poke_col % PAGE_DIM;
        grid.pages[page_index][local_row * PAGE_DIM + local_col] = 1;

        // Every 100 steps: print computed hash of each page.
        if steps % 100 == 0 {
            println!("Step {}: (hashes)", steps);
            for (index, page) in grid.pages.iter().enumerate() {
                let hash = keccak256_hash(page);
                println!("  Page {} hash: {}", index, bytes_to_hex(&hash));
            }
        }

        // Every 10,000 steps: print full contents of each page.
        if steps % 1_000 == 0 {
            println!("Step {}: (full page contents)", steps);
            for (index, page) in grid.pages.iter().enumerate() {
                println!("Page {} contents: {}", index, bytes_to_hex(page));
            }
        }
        
    }
}
