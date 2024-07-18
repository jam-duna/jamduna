Background:

* https://github.com/paritytech/erasure-coding/blob/6c22e28a7402cfc75381e430483defe8189df486/tools/test_vectors/src/main.rs#L29-L54
* https://github.com/w3f/jamtestvectors/pull/4
* https://github.com/paritytech/erasure-coding/pull/15

As you can see, the test vectors were NOT generated with the "powers
of 2" but instead there is a form of padding involved.  In principle
we could use this Rust code as a "Foreign Function Interface" instead
of coding the raw stuff in Go, which MAY (or may not) be a better way
to solve this.  I did this for Safrole "bandersnatch", and its not as
hard as you think, but it will involve THREE languages (Rust, Go, and
a form of "C") [Fortunately, ChatGPT knows all 3 better than we do]

So to jump in:
```
root@polkaholic-infra:~# git clone git@github.com:paritytech/erasure-coding.git
root@polkaholic-infra:~/erasure-coding# git checkout cheme/test_vecs
root@polkaholic-infra:~/erasure-coding# cd tools
root@polkaholic-infra:~/erasure-coding/tools# cd test_vectors/
root@polkaholic-infra:~/erasure-coding/tools/test_vectors# cargo run
    Finished dev [optimized + debuginfo] target(s) in 0.12s
     Running `target/debug/test_vectors`
vectors/package_0

Skipping size 0, for package
vectors/package_1

Skipping size 1, for package
vectors/package_32

Skipping size 32, for package
vectors/package_684

Skipping size 684, for package
vectors/package_4096

Skipping size 4096, for package
vectors/package_4104

Skipping size 4104, for package
vectors/package_15000

Skipping size 15000, for package
vectors/package_21824

vectors/package_21888

vectors/package_100000

vectors/package_200000

Skipping check size 4096, for package
Skipping check size 15000, for package
Skipping check size 684, for package
Skipping check size 4104, for package
Skipping check size 0, for package
Skipping check size 32, for package
Skipping check size 1, for package
3:12
root@polkaholic-infra:~/erasure-coding/tools/test_vectors# git diff src/main.rs
diff --git a/tools/test_vectors/src/main.rs b/tools/test_vectors/src/main.rs
index 526913f..c8a768b 100644
--- a/tools/test_vectors/src/main.rs
+++ b/tools/test_vectors/src/main.rs
@@ -126,7 +126,8 @@ fn build_vector(size_index: usize) {
                std::println!("Skipping size {}, file {} exists already", package_size, file_name);
                return;
        }
-       let mut file = File::create(&file_path).unwrap();
+    std::println!("{}\n", file_path.display());
+        let mut file = File::create(&file_path).unwrap();
 
        let mut vector = Vector::default();
        vector.data = vec![0; package_size];
3:13
root@polkaholic-infra:~/erasure-coding/tools/test_vectors# ls -lt vectors
total 5512
-rw-r--r-- 1 root root 2838783 Jul  9 01:12 package_200000
-rw-r--r-- 1 root root 1527687 Jul  9 01:12 package_100000
-rw-r--r-- 1 root root  460923 Jul  9 01:12 package_21888
-rw-r--r-- 1 root root  374907 Jul  9 01:12 package_21824
-rw-r--r-- 1 root root  187467 Jul  9 01:12 package_15000
-rw-r--r-- 1 root root   78435 Jul  9 01:12 package_4104
-rw-r--r-- 1 root root   47597 Jul  9 01:12 package_4096
-rw-r--r-- 1 root root   37571 Jul  9 01:12 package_684
-rw-r--r-- 1 root root   36703 Jul  9 01:12 package_32
-rw-r--r-- 1 root root   31185 Jul  9 01:12 package_1
-rw-r--r-- 1 root root     347 Jul  9 01:12 package_0
```

Example:

```
root@polkaholic-infra:~/erasure-coding/tools/test_vectors# more vectors/package_684
{
  "data": "NpN8bWQhzEqcrkKl96PeoLYPXcoIgCdynrTZoYDIfOR9J4Z6CN/yKDZQi2+HQ2fKU6C0Zw4HL+A7HKl9DECQMYKrVmxZKt0ZFED2n7To4HoN26Vuq4MhX9sa6nG+78d8PMUZWv1qsC3bS8FnVtVn+k8TPEJflxNYe
NyRAG04YulQEm3eJtHLaKtNtg6w0CEJMq4ydcdq3B2Qukh8V7r8Ujp488nqEhjy3Hb4QoM17psLPMuXqeHSiL87QnWvKhs3Xe62bD9b3G2Pce5fejJnA1YQAu69d4ZJcrmwcgfUk70kof+E2Fj2q726JKHqCe/6k/mLpl0quFxcX
L8bh3va36Je0GULqD9L4IsNIEUyEx/HnUI1Ao9mXTDtJZUUbb7dHBQwJaVUMSxZgIQAdRJakogSCAAOb3UAorxpbsRiiBAh04sZPIbey5X/od6Qblsh3/0/loVowy5nFXaeBL5SnJdcSNkAmfUjXbZxt4owCT/N7BWVkADIVnEKP
Rm5gDcAVRfFmBzpwT61vZNIUeOr97BzEIGGigewZWBJkU/opIpjDuTpEMdAqsxep97MeC6mHpH2b/om1K7qbNnxN4owWHz1c/+0Qm1l1B6xFUQCbYSESozaYmz1GKuLRfh1mZQnMJU/+wl3DwKDrAj6l5xb87Sf3TWgWomudxB0R
y5sUTvGf4tD70M10fn2s++AW6DBgBallR0DsMFTExQ2UkqT4DGILe7ZBkNsNpLwHLgJyO8BkW/0SZjovSBfZ1jovHFW5H8t6vDjk+YdoWnFKTsRoDK67nWExUum3VLvqAVVrHooYVT/xSYJsUIvGCnPybgU4tqfnuB5lBkEIOypQ
KYq5xzqNBXKokX+0OlqqU1Xv/C+Krt26UnUif8wCp055bjK5unOBO/H1rRfroTo",
  "work_package": {
    "chunks": [],
    "chunks_root": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
  },
  "segment": {
    "segments": [
      {
        "segment_ec": [
          "NpMAAAAAAAAAAAAA",
          "fG0AAAAAAAAAAAAA",
          "ZCEAAAAAAAAAAAAA",
...
          "NnAAAAAAAAAAAAAA",
          "IoMAAAAAAAAAAAAA",
          "asoAAAAAAAAAAAAA",
          "Q2AAAAAAAAAAAAAA"
        ]
      }
    ],
    "segments_root": "1+CFC+IMLPvr6la5SAZIyShjYB6v6Lvp4zRMl8Jbu6c="
  },
  "page_proof": {
    "page_proofs": [
      "NpN8bWQhzEqcrkKl96PeoLYPXcoIgCdynrTZoYDIfOR9J4Z6CN/yKDZQi2+HQ2fKU6C0Zw4HL+A7HKl9DECQMYKrVmxZKt0ZFED2n7To4HoN26Vuq4MhX9sa6nG+78d8PMUZWv1qsC3bS8FnVtVn+k8TPEJflxNYeNyRA
G04YulQEm3eJtHLaKtNtg6w0CEJMq4ydcdq3B2Qukh8V7r8Ujp488nqEhjy3Hb4QoM17psLPMuXqeHSiL87QnWvKhs3Xe62bD9b3G2Pce5fejJnA1YQAu69d4ZJcrmwcgfUk70kof+E2Fj2q726JKHqCe/6k/mLpl0quFxcXL8bh
3va36Je0GULqD9L4IsNIEUyEx/HnUI1Ao9mXTDtJZUUbb7dHBQwJaVUMSxZgIQAdRJakogSCAAOb3UAorxpbsRiiBAh04sZPIbey5X/od6Qblsh3/0/loVowy5nFXaeBL5SnJdcSNkAmfUjXbZxt4owCT/N7BWVkADIVnEKPRm5g
DcAVRfFmBzpwT61vZNIUeOr97BzEIGGigewZWBJkU/opIpjDuTpEMdAqsxep97MeC6mHpH2b/om1K7qbNnxN4owWHz1c/+0Qm1l1B6xFUQCbYSESozaYmz1GKuLRfh1mZQnMJU/+wl3DwKDrAj6l5xb87Sf3TWgWomudxB0Ry5sU
TvGf4tD70M10fn2s++AW6DBgBallR0DsMFTExQ2UkqT4DGILe7ZBkNsNpLwHLgJyO8BkW/0SZjovSBfZ1jovHFW5H8t6vDjk+YdoWnFKTsRoDK67nWExUum3VLvqAVVrHooYVT/xSYJsUIvGCnPybgU4tqfnuB5lBkEIOypQKYq5
xzqNBXKokX+0OlqqU1Xv/C+Krt26UnUif8wCp055bjKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="
    ],
    "segments_root": "g7vhXhL03e+on6ancSF1wIHs+9YGcSnXCaNe+9pu9o8="
  }
}
```

By seeing the encoding process (and decoding process) work in Rust perfectly, you can set up the decoding process in Go.


```
root@polkaholic-infra:~/erasure-coding/tools/test_vectors# git diff src/main.rs
diff --git a/tools/test_vectors/src/main.rs b/tools/test_vectors/src/main.rs
index 526913f..5e57010 100644
--- a/tools/test_vectors/src/main.rs
+++ b/tools/test_vectors/src/main.rs
@@ -27,10 +27,7 @@ use std::{
 // Some size may not make sense but this should
 // not be an issue regarding EC
 const PACKAGE_SIZES: [usize; 11] = [
-       0,
-       1,
-       32,
-       684,     // one subshard point only
+       0, 1, 32, 684,     // one subshard point only
        4096,    // one page only for subshard
        4104,    // one page padded
        15000,   // unaligne padded 4 pages
@@ -123,8 +120,8 @@ fn build_vector(size_index: usize) {
        let file_name: String = format!("{}_{}", PREFIX_PACKAGE, package_size);
        file_path.push(&file_name);
        if file_path.exists() {
-               std::println!("Skipping size {}, file {} exists already", package_size, file_name);
-               return;
+               //      std::println!("Skipping size {}, file {} exists already", package_size, file_name);
+               //      return;
        }
        let mut file = File::create(&file_path).unwrap();
 
@@ -134,12 +131,21 @@ fn build_vector(size_index: usize) {
        let mut rng = rand::thread_rng();
        rng.fill_bytes(&mut vector.data);
 
+       std::println!(
+               "File: {} package_size: {} N_CHUNKS: {} 64 * N_CHUNKS: {} N_CHUNKS*3: {}\n",
+               file_path.display(),
+               package_size,
+               N_CHUNKS,
+               64 * N_CHUNKS,
+               N_CHUNKS * 3
+       );
        // consider data as work package then chunks
        if package_size >= (64 * N_CHUNKS as usize) {
                for chunk in construct_chunks(N_CHUNKS * 3, &vector.data).unwrap() {
                        vector.work_package.chunks.push(Bytes(chunk));
                }
                let chunk_len = vector.work_package.chunks[0].0.len();
+               std::println!("chunk_len: {}", chunk_len);
                let merlized = root_build(vector.data.as_slice(), chunk_len);
                vector.work_package.chunks_root = merlized.root().into();
        } else {
@@ -151,6 +157,7 @@ fn build_vector(size_index: usize) {
        let mut encoder = erasure_coding::SubShardEncoder::new().unwrap();
        for segment_chunks in encoder.construct_chunks(&segments_chunks).unwrap().into_iter() {
                let mut segment = Segment { segment_ec: Vec::with_capacity(segment_chunks.len()) };
+               std::println!("segment_chunks.len(): {}", segment_chunks.len());
                for chunk in segment_chunks.iter() {
                        segment.segment_ec.push(SubChunk(*chunk));
                }
@@ -158,7 +165,7 @@ fn build_vector(size_index: usize) {
        }
        assert_eq!(vector.segment.segments.len(), segments_chunks.len());
        vector.segment.segments_root = root_from_segments(segments_chunks.as_slice());
-
+       std::println!("vector.segment.segments.len(): {}", vector.segment.segments.len());
        // consider data as containing only hashes of every exported segments up to 2^11 segments.
        build_segment_root(vector.data.as_slice(), &mut vector.page_proof);
```

You can solve this problem with the FFI round, in which case the key operations of DECODING from

```
if vector.work_package.chunks.len() > 0 {
                let r = erasure_coding::reconstruct(
                        N_CHUNKS * 3,
                        vector
                                .work_package
                                .chunks
                                .iter()
                                .enumerate()
                                .filter(|(i, _)| in_range(*i, false))
                                .map(|(i, c)| (ChunkIndex(i as u16), &c.0)),
                        package_size,
                )
                .unwrap();
                assert_eq!(r, vector.data);
        }
        let mut decoder = erasure_coding::SubShardDecoder::new().unwrap();
        // not running segments in parallel (could be but simpler code here)                                                                                                
        for (seg_index, segment) in vector.segment.segments.iter().enumerate() {
                let r = decoder
                        .reconstruct(
                                &mut segment
                                        .segment_ec
                                        .iter()
                                        .enumerate()
                                        .filter(|(i, _)| in_range(*i, true))
                                        .map(|(i, c)| (seg_index as u8, ChunkIndex(i as u16), c.0)),
                        )
                        .unwrap();
                assert_eq!(r.1, 1);
                assert_eq!(r.0.len(), 1);
                assert_eq!(r.0[0].0, seg_index as u8);
                assert_eq!(r.0[0].1, segments_chunks[seg_index]);
        }

```

need to be put in a Rust function with extern "C" like I did here:

* https://github.com/colorfulnotion/jam/blob/main/bandersnatch/bandersnatch.go#L43-L53
* https://github.com/colorfulnotion/jam/blob/main/bandersnatch/bandersnatch.h

which eliminated the need to reimplement all the Rust crypto machinery.  It seems extremely likely, given the above Rust code test vector generation that we should do the 
I got my first Rust =FFI=> Go program working in mid-June.    Here are the basics.  It may be valuable for you to go through these steps to make you confident that you can call erasure_coding::reconstruct for any byte length along with `encoder.construct_chunks`.  Here is the basics of a Go program (main.go) calling a Rust  library rust_add with a function add_two_integers where the end point is basically:

```
root@polkaholic-infra:~/ffi/rust_add# go run main.go
The sum of 5 and 7 is 12
root@polkaholic-infra:~/ffi/rust_add# ls -lt
total 24
drwxr-xr-x 2 root root 4096 Jun 18 22:02 src
-rw-r--r-- 1 root root   95 Jun 18 22:01 Cargo.toml
drwxr-xr-x 3 root root 4096 Jun 18 21:58 target
-rw-r--r-- 1 root root  152 Jun 18 21:58 Cargo.lock
-rw-r--r-- 1 root root  268 Jun 18 21:57 main.go
-rw-r--r-- 1 root root  166 Jun 18 21:57 rust_add.h

root@polkaholic-infra:~/ffi/rust_add# ls -lt src
total 4
-rw-r--r-- 1 root root 235 Jun 18 22:02 lib.rs
main.go
package main

/*
#cgo LDFLAGS: -L./target/release -lrust_add
#include "rust_add.h"
*/
import "C"
import "fmt"

func main() {
    a := 5
    b := 7
    result := C.add_two_integers(C.int(a), C.int(b))
    fmt.Printf("The sum of %d and %d is %d\n", a, b, int(result))
}
rust_add.h is referenced in the above go file
#ifndef RUST_ADD_H
#define RUST_ADD_H

#ifdef __cplusplus
extern "C" {
#endif

int add_two_integers(int x, int y);

#ifdef __cplusplus
}
#endif

#endif // RUST_ADD_H
Cargo.toml is how the Rust library is compiled:
 
[package]
name = "rust_add"
version = "0.1.0"
edition = "2021"


[lib]
crate-type = ["cdylib"]
here in "target/release":
# ls -lt target/release/
total 3328
drwxr-xr-x 2 root root    4096 Jun 18 22:02 deps
-rwxr-xr-x 2 root root 3379792 Jun 18 22:02 librust_add.so
-rw-r--r-- 1 root root      82 Jun 18 21:58 librust_add.d
-rw-r--r-- 2 root root    3978 Jun 18 21:58 librust_add.rlib
drwxr-xr-x 2 root root    4096 Jun 18 21:58 build
drwxr-xr-x 2 root root    4096 Jun 18 21:58 examples
drwxr-xr-x 2 root root    4096 Jun 18 21:58 incremental
based on
root@polkaholic-infra:~/ffi/rust_add# cat src/lib.rs 


#[no_mangle]
pub extern "C" fn add_two_integers(x: i32, y: i32) -> i32 {
    x + y
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn it_works() {
        let result = add(2, 2);
        assert_eq!(result, 4);
    }
}
```

Once you have a Go program using the parity erasure_coding library, you can proceed to use the Constant Depth Merkle Trie package in Go to compute segment root and "paged proofs".
