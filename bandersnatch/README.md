# Bandersnatch library

We use a Go-based FFI to davxy's IETF + Ring VRF Rust library. 

Rust:
```
# cargo test -- --nocapture 
warning: field `commitment` is never read
   --> src/lib.rs:120:9
    |
119 | struct Verifier {
    |        -------- field in this struct
120 |     pub commitment: RingCommitment,
    |         ^^^^^^^^^^
    |
    = note: `#[warn(dead_code)]` on by default

warning: `bandersnatch` (lib test) generated 1 warning
    Finished test [unoptimized + debuginfo] target(s) in 0.07s
     Running unittests src/lib.rs (/root/go/src/github.com/colorfulnotion/jam/bandersnatch/target/debug/deps/bandersnatch-1aeb8e5dba87745a)

running 4 tests
Public Key: Public((23442063538195846661742373395729734442242035977768966282914944910562780818753, 13156918817181251236527455909657275781885587278433599221714040374033674618462))
VRF Output0: "f88c394a21f612cf9dba8b8badfa74af4573c9ef6da84c183a9a4d548158acad"
Signature: "ab18386059ca4c99a50196ced60058a44e1339ff1c7e88a200b2ac43ed4b0447a722c54c4b03508db413deb674be9334afd686825b1488912ea95ace705af31282a653d26ad4ef2c7ee05feb4cc6811557fb7eb62e1c2a6e271e1608d6a85c13"
VRF Output1: "f88c394a21f612cf9dba8b8badfa74af4573c9ef6da84c183a9a4d548158acad" (Verification successful)
test tests::test_ietf_vrf_sign_and_verify ... ok
Verification result: 1
VRF output hash: 09a696b142112c0af1cd2b5f91726f2c050112078e3ef733198c5f43daa20d2b
test tests::test_ring_vrf_verify_external ... ok
Signature: "015a9370a4a89f94dd21da9cd804139e338e7d5ebb8ce728593803dc84b2042e4800bb22df4a02d6a5ac9f6b24ec2c9b760074f6ee9e02ffbce83ba6548984e6c9b4ddca5832bf99c1912e1eaaeb32f8c336a2126319d40a007edb2bb856913383ecc86e8351229381bf7e545645289cf8b235fb5a464aeefb085a7ba81463ec9d5f46f1dd44c9c7f9b0283e496a7bb3aa305cac20eaa005f68616c35e80e7042d612d1c6e36fc567fc2a228ec4a4f67afa4ae06cf05fa289343ef843897fe15ac4234f6cac9a2372f0591a37557fc0446e322435c01c26d42ea8b1b10fd1e4f541d7d75d62c3ff4a9bcb0e6231f458bad579ffb9a8dbde2f66625efb40f5773365435aec5cec95bbf873b03d361baa5fc3fb339f5a7e16360e7033e89887631b6e7fab00966988cf894399585e49ee3500b04f64f127277fd01a578407a412770f26a64496ff0b45f8921efbd965bb7b8cc759fa7c97751d578c211b10c9afa6db68a0d6c77ddfaa9197f135d3bbde4d9fd42ac18228f8733326f9d3ee8bebf9e74e1c39446d0b3e497c5e0ea0fd6d33d379a6caae7be2fd7602f2d2069070d3004b72730898b1639aaac48492fb7aab9296da3617020102f388100a8a7413e95f9f7934ccc122b8c2b12797dd43f6891f9195c564459548973390dffde0d58a3061177c51b01e14779b8053055fa956be6c0ce22e00ce45f699d128010ef4bfe02e6a18c8a15cc53479d446fdda42bed27e8d7cf8d32761970a7ec58714a099c8f116a47d218febcba3cc88a57b6f04ffab0f4e9bdab6cde80f115ce68673d79569341cd37206715956f8c2518ed667e24b0114f4f7230b4a25f1d4600135d92dfe1f9cd59492f33f4084904df8fb0d0b10cb18e5d112edf10ec6995cd1b415f05e253ee31465bcf4b48059c44a74d12e0b71d0de9b2ee9e1e6b3db2fb4856422304229334920f34756de777f0cb0594746da62e91e467737cccb7b5fa8503d0703b4fc1f7c837ca70ecf8d4f4eae6f4f2609b88f1c5c9a95dfa38a1b841bfa7566e4d611f55d89bc417bc7d4629746d556dc8d8d9f258a16533fba4fee54b7e8c885f8f9a50dae00498d70f0e7b78"
VRF Output: "57bfedcc9d2f61941af1c4920d51c9ffcde2560225279afa59f70e40e58d305a"
* Time taken by ring-vrf-sign: 4.629066444s
Verification successful
VRF Output: "57bfedcc9d2f61941af1c4920d51c9ffcde2560225279afa59f70e40e58d305a"
test tests::test_ring_vrf_sign_and_verify ... ok
* Time taken by ring-vrf-verify: 730.74108ms
* Time taken by ietf-vrf-sign: 6.022439ms
* Time taken by ietf-vrf-verify: 8.264699ms
test tests::test_sign_and_verify_internal ... ok

test result: ok. 4 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 7.51s
```

Go:
```
# go test
TestVRFOperations IetfVrfSign -- VRFOutput: d838d9787d8af3ddb93053b398ba693fb2e0e553bfb6d98791fa5a2039819bfa Signature: 1d16576a25d669c7797b0740e772dc809c3be2be4124c0695ce5023132b1ba263649413acbd09c8bca26b1e83df0c6e54980e1e88b02c789d4d9ac9460bb5704635699759fa527d862f28aaa45672400d5f969dd016add727e0a59892ba0b90a (96 bytes)
TestVRFOperations Ietf Recovered VRFOutput: d838d9787d8af3ddb93053b398ba693fb2e0e553bfb6d98791fa5a2039819bfa
TestVRFOperations IETF VRF Output: d838d9787d8af3ddb93053b398ba693fb2e0e553bfb6d98791fa5a2039819bfa (32 bytes) [VERIFIED sigature]
IETF VRF Outputs MATCH!
TestVRFOperations RingVrfSign -- VRFOutput: d838d9787d8af3ddb93053b398ba693fb2e0e553bfb6d98791fa5a2039819bfa Signature: 1d16576a25d669c7797b0740e772dc809c3be2be4124c0695ce5023132b1ba2631002db5fad9f50b21b6c4fe40ad73defa9820e221dbd469b59211e6eb6a48b0e6cd6091f01329f5810e4a9a3e989846c9eaabf239156b550854ae409656cc13ca7b1d67a30f33c87484718f8667fadde1b607d07f2f8b3af6d4f7ac14bf3ac675d6981c37dc16c2f9ba9b7393bb59757c77abfd5a6f55c33b1821dfdc83cf00f0d1917e391346ac6dc15063240ae1bd181f42e2a93873a652eb1c3980c589119154212d04dd9ebb08e15463d4d6d876af97681823892be73982984d6422c0a1bd8dadae473634be0d2e8186fd8de134ad579ffb9a8dbde2f66625efb40f5773365435aec5cec95bbf873b03d361baa5fc3fb339f5a7e16360e7033e898876318b5b7b63f8f804ed7670a4143ca4c334cfdab327c8d086a2a80c48330c2a28cf671694ad5b54cd04c63ce6e696174dfa80b8f5ee16dfef5eacc8a20928f0ff3b0ad9fdb0f307337c3cf53b0487a918e9677b91be7ec28d91e72bfcef05decaa927933db1b30211236afb2223ac37ba1ce5a6fbcfb3e6a70fe9d3e98b87784908c519e11609baa9d9534b905bd46bb534eba7c3f9b7a481ed5aa2733509cb7031aa65ffa89cc87016f4190583c25af969740773d413581545d4d0326b4cecd740d7add6255bd61cca3b31dd396b905a11709b421a22574d88dfc6d76cf2cc770cbc5056995bae77cce8097a3bd71800bdf92390c1dd3729f187a8de7e563ebe0d94f4c2ef0e69ea2694d72aa9e1e8d9c45355d06e1c302251900d97afb15a11491f8945635ca4bdf6139036946b29a4395f32eea54701fc3eb79367c194e07038a614e8ba917e27885de47071b3532243ca85ebb2f81c4ac57bdbfc84a10c8e6ffaf977ba171cbc1833303cbbbcbd53b390b8d852909e9cb078285dca1258e9fb5b7f732b53264eb6aeb995af386ded1cad16dca4cc4c4541a7843c791e96314dfe1eea2f6803dc85369455648ba21cc3e2d7ad56f96c0fcdcfc664210707340c80bdb2c004a38bac155aa4fd78b8b8c8426da442dbee0b740913782cab9687730192814c4297a44829d1ae4f074f8206 (784 bytes)
TestVRFOperations Ring Recovered VRFOutput: d838d9787d8af3ddb93053b398ba693fb2e0e553bfb6d98791fa5a2039819bfa
TestVRFOperations Ring VRF Output: d838d9787d8af3ddb93053b398ba693fb2e0e553bfb6d98791fa5a2039819bfa
Ring VRF Outputs MATCH!
IETF + Ring VRF Outputs MATCH!
TestBandersnatch RingVRFOutput: 09a696b142112c0af1cd2b5f91726f2c050112078e3ef733198c5f43daa20d2b (32 bytes)
PASS
ok  	github.com/colorfulnotion/jam/bandersnatch	0.343s
```

Next steps:
* situate the above inside `safrole`
* pass tests cases when ready

### How it works:
1. Cargo.toml 
```
[lib]
crate-type = ["cdylib"]
```

where building the library generates `libbandersnatch_vrfs.so` in `target/release` (This can also be done with "make bandersnatch")

# cargo build --release
Finished release [optimized] target(s) in 0.32s

# ls -l target/release/*.so
-rwxr-xr-x 2 root root 4536904 Jun 19 16:19 target/release/libbandersnatch_vrfs.so
```

2. To use the above library in `target/release`:
```
export LD_LIBRARY_PATH=$(pwd)/target/release:$LD_LIBRARY_PATH
```

3. Then this works:

```
# go test
(see above)
```



