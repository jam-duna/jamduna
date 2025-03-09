use std::slice;
use tiny_keccak::{Hasher, Keccak};
use hex;

// Constants: 1024 u32 values per page (1024 * 4 = 4096 bytes)
const RESULTS_PER_PAGE: usize = 1024;
const NUM_PAGES: usize = 65_536;

/// Checks whether `candidate` appears in any previously computed pages or
/// in the current page buffer up to current_index.
fn already_seen(candidate: u32, pages: &Vec<[u32; RESULTS_PER_PAGE]>, current_page: &[u32], current_index: usize) -> bool {
    // Check all complete pages.
    for page in pages {
        for &value in page.iter() {
            if value == candidate {
                return true;
            }
        }
    }
    // Check the current page up to index current_index.
    for &value in current_page.iter().take(current_index) {
        if value == candidate {
            return true;
        }
    }
    false
}

pub fn main() {
    let mut recaman: u32 = 0; // Starting value: a(0) = 0.
    let mut n: u32 = 1;       // Step counter starting from 1.
    
    // We'll store complete pages here.
    let mut pages_vec: Vec<[u32; RESULTS_PER_PAGE]> = Vec::new();

    // For each page:
    for page_num in 0..NUM_PAGES {
        // Create a new page buffer.
        let mut page = [0u32; RESULTS_PER_PAGE];

        // Fill the page with 512 Recamán numbers.
        for i in 0..RESULTS_PER_PAGE {
            // For Recamán's sequence:
            // Candidate = recaman - n, if recaman >= n and candidate has not been seen.
            // Otherwise, candidate = recaman + n.
            let candidate = if recaman >= n && !already_seen(recaman - n, &pages_vec, &page, i) {
                recaman - n
            } else {
                recaman + n
            };

            page[i] = candidate;
            recaman = candidate;
            n += 1;
        }

        // Interpret the page (512 u32 values) as bytes (each u32 is 8 bytes).
        let page_bytes: &[u8] = unsafe {
            slice::from_raw_parts(page.as_ptr() as *const u8, RESULTS_PER_PAGE * 8)
        };

        // Compute the Keccak256 hash of the page.
        let mut keccak = Keccak::v256();
        keccak.update(page_bytes);
        let mut hash = [0u8; 32];
        keccak.finalize(&mut hash);

        println!("Page {}: Hash: {} | n = {} | recaman = {}", 
                 page_num, hex::encode(hash), n, recaman);

        // Store the completed page.
        pages_vec.push(page);
    }
}


