# Erasure coding


Compile Rust lib:
```
cargo build --release
mv target/release/liberasurecoding.so .
```

Test:

```
go test -run TestEncodeDecode
```