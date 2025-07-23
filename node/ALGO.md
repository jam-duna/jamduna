```
root@polkavm:/tmp/root/jam/1753229146_404b26a28d26111d# ls -lt node?/data/bundle_snapshots/*guarantor.bin | wc
     17     153    2754
root@polkavm:/tmp/root/jam/1753229146_404b26a28d26111d# ls node?/data/bundle_snapshots/*guarantor.bin
node0/data/bundle_snapshots/00000024_0xaca193b21fe5cad5cbc6bb3a3ea34006cbf89016fa613eba61ad6f7de90bfda8_1_0_guarantor.bin
node0/data/bundle_snapshots/00000027_0x43285f347c84d8f0cf208cc3b94f222d63c020c026e33a04d4a2ecddaa922337_1_0_guarantor.bin
node0/data/bundle_snapshots/00000031_0x0a698c35d2c4ce8282c7384d1a937ab7dbf5f20af537309299873fedb7cb7c2d_0_0_guarantor.bin
node0/data/bundle_snapshots/00000033_0xe515b915ec1a7f05d870530973d3029602c3905c25acd7ddf91668e293949df4_1_0_guarantor.bin
node0/data/bundle_snapshots/00000037_0x405d6952331e6712a53ea3d3615e4aca7ee722de2a0ec6e8bbd05342a04bf8fb_1_0_guarantor.bin
node0/data/bundle_snapshots/00000039_0x8ba47f8dfcee01962f85b2fab4476097a711f725705beca0de522062f7ca1955_1_0_guarantor.bin
node0/data/bundle_snapshots/00000042_0xdf475c90abd45355f3e01185f73443d0d9d1bc1fad53a41b67b0f95333400a4d_0_0_guarantor.bin
node0/data/bundle_snapshots/00000045_0xbe22c3536b346cc1e3b02047244c9f53fe8de1d911e5bbd4963a25cd06f57786_1_0_guarantor.bin
node0/data/bundle_snapshots/00000047_0x6d68757bfda0e35ca577e355e42968411fbc66d040516cfb4459b73d2148edd6_1_0_guarantor.bin
node0/data/bundle_snapshots/00000060_0xbbec9ed659891ae8992b71ce29d58b79efca4a6164ea343c075dad08b76ebe5a_1_0_guarantor.bin
node0/data/bundle_snapshots/00000064_0x7fb021a9b3add81385a46abb058aa01c292ce1ec62e408fd7e7ad4d6144d6d01_0_0_guarantor.bin
node3/data/bundle_snapshots/00000017_0x22bbb94ebfc52027e737218240a714f6a3de828eee2d1fb6d1fd9a31c50eab3b_0_3_guarantor.bin
node3/data/bundle_snapshots/00000021_0xed03637641df7c2eaf1fe516b97f5a057eca412580ceb0e4888483420dbce250_1_3_guarantor.bin
node3/data/bundle_snapshots/00000049_0x5199ce3e8262067c4770afa0fc59b8a24995bd6358419b6afb0f0b1da978162b_1_3_guarantor.bin
node3/data/bundle_snapshots/00000052_0xfc77c78828e19dc2428045197ded5b0e5f0795dcb6a6272818b5c15175532bb2_0_3_guarantor.bin
node3/data/bundle_snapshots/00000055_0xf85ce5368a2382af5c35ee699f9d50933ef0d84e6a694a105caa85231dab799e_0_3_guarantor.bin
node3/data/bundle_snapshots/00000058_0x8d3647c49042a2849dd41c647727ce9d767157f9698a1ddac596883726658cdd_1_3_guarantor.bin
```



INFO [07-23|00:06:40.330] ALGO(0)                                  workPackageHash=0xcba992a66d97b79e227a84e11bf083c89fa00c12f125f1861952b2e7a5536a10 wr='{
  "package_spec": {
    "hash": "0x22bbb94ebfc52027e737218240a714f6a3de828eee2d1fb6d1fd9a31c50eab3b",
    "length": 310,
    "erasure_root": "0xb3db23bac2501d3121c0c9a22294ad0da061810136e1703fe854946ecc44add0",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x86b3fb1e17d06902c460755909de2d98dad6a4b50649230070c403c1bc6597b0",
    "state_root": "0x552b5afefd76914f113e4a8c8eb30d53c579732f847cddbae9960d20a6c6623e",
    "beefy_root": "0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
    "lookup_anchor": "0x86b3fb1e17d06902c460755909de2d98dad6a4b50649230070c403c1bc6597b0",
    "lookup_anchor_slot": 13,
    "prerequisites": []
  },
  "core_index": 0,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0xbe6878df54aad5658765327a7280a1618212ba05f2635f83347685de25bb44bd",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x00010203040506070809"
      },
      "refine_load": {
        "gas_used": 37795,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}


INFO [07-23|00:06:58.348] ALGO(1)                                  workPackageHash=0x71a7788de55dd7e87ca5b208eaea04f0e0ac869d1a63d00a8df3d765bdb5cece wr='{
  "package_spec": {
    "hash": "0xed03637641df7c2eaf1fe516b97f5a057eca412580ceb0e4888483420dbce250",
    "length": 310,
    "erasure_root": "0xa55073d3a90ea4aa6b026dcc3e6b0758bbd7cebed4ae756ad4eae66dcb11256c",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x7ec7ee27bf975c8e1a39cb942f66c544e86af5664a6d003062daa17cd7efbe7e",
    "state_root": "0x9333a7d15c19bd2fc717beb60f44e76dce18e6c033e625e7cd505c44f6888939",
    "beefy_root": "0xfee77aea940e38601cb828d2878308e32529f8382cf85f7a29b75f1c826985bc",
    "lookup_anchor": "0x7ec7ee27bf975c8e1a39cb942f66c544e86af5664a6d003062daa17cd7efbe7e",
    "lookup_anchor_slot": 17,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0x617ce002e75fe042290e845110a998659cdafc605f19cb368011d87a98dcd3ac",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x0a0b0c0d0e0f10111213"
      },
      "refine_load": {
        "gas_used": 113534,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}


# ALGO(2) 

{
  "package_spec": {
    "hash": "0xaca193b21fe5cad5cbc6bb3a3ea34006cbf89016fa613eba61ad6f7de90bfda8",
    "length": 310,
    "erasure_root": "0xe29aa85c5fdb18e63fdcc43cefe5727973c5d2a14128291838514b82435e4bec",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x16e52da1f690c735e2b8eca6b655764ca2e10dc18de909d7f51ee0d6d5f76146",
    "state_root": "0xdbad9f05f996303f38ba339b046b424fad168f33c21b83ee1ea01361df0afeb4",
    "beefy_root": "0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
    "lookup_anchor": "0x16e52da1f690c735e2b8eca6b655764ca2e10dc18de909d7f51ee0d6d5f76146",
    "lookup_anchor_slot": 19,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0x461bc668885fe22a6a6a218e98ed42fd5dc303f08e201651cd3e33142c98d186",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x1415161718191a1b1c1d"
      },
      "refine_load": {
        "gas_used": 73366,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}



# ALGO(3)   


```
{
  "package_spec": {
    "hash": "0x43285f347c84d8f0cf208cc3b94f222d63c020c026e33a04d4a2ecddaa922337",
    "length": 310,
    "erasure_root": "0x25f7aa70ebdcb3cad7be6e9dfd1be3b9d88933ec3ba52bfc2d3c0624e9092b3f",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x3364f9ba7ed8bb2ed8b3be1caac15e2f8d10ab3f83007803d2fbd6bc403414d7",
    "state_root": "0x5ebd959b8738c8ae6ca61a66efd81d06d69868afee68b6dda99630ca4937b293",
    "beefy_root": "0xa3dcfd2a75b714b29d6088aa094269d9777e1cd60cf128593d52067cff3c7f1f",
    "lookup_anchor": "0x3364f9ba7ed8bb2ed8b3be1caac15e2f8d10ab3f83007803d2fbd6bc403414d7",
    "lookup_anchor_slot": 23,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0xa59f1540b0b18b96ce25cb3c87f10b1be4929cef3933a78f8a13af724b2061f7",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x1e1f2021222324252627"
      },
      "refine_load": {
        "gas_used": 71706,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}
```


# ALGO(4)

```
{
  "package_spec": {
    "hash": "0x0a698c35d2c4ce8282c7384d1a937ab7dbf5f20af537309299873fedb7cb7c2d",
    "length": 310,
    "erasure_root": "0x66a73fb3d97e6ff9e4d565267155910f0c590f9053e013048cd6792dad555471",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x9724f76e62d172949173adc2aa45a8628f955c1ed720606a0b47e9e9eba25b9d",
    "state_root": "0xdd25ff2df5ec661f76bbacb6e39a7fd5e655f48cc57c8ea1407c757766d4e4d6",
    "beefy_root": "0x8e3c089409f743e71ccef29a8fa322fc11fa0ec372949749ecddc6fb5d23b4b6",
    "lookup_anchor": "0x9724f76e62d172949173adc2aa45a8628f955c1ed720606a0b47e9e9eba25b9d",
    "lookup_anchor_slot": 26,
    "prerequisites": []
  },
  "core_index": 0,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0x54de2d7e54b96d3ef79fd3e8cbc440ceb26f80e9605c6ad7d8639b3369409f8c",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x28292a2b2c2d2e2f3031"
      },
      "refine_load": {
        "gas_used": 31678,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}
```



### ALGO(5)

```
{
  "package_spec": {
    "hash": "0xe515b915ec1a7f05d870530973d3029602c3905c25acd7ddf91668e293949df4",
    "length": 310,
    "erasure_root": "0x380e96fc5fbe6489651179811cbf638b40b2117e998c8c9912a474f5642aa12a",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x8ff0033b9d436946399500e9c6dc1dd9481c5fe72148e48aa56bf0190c7765e7",
    "state_root": "0xe85ef3f0162d0bb3f5e91dc63bdf6b6f989e4824f473551894758661f7336074",
    "beefy_root": "0xd8b2e02c420d7ad6075e87e0f06b02ff88277f75a702d0f966d3c5d1d857c4fa",
    "lookup_anchor": "0x8ff0033b9d436946399500e9c6dc1dd9481c5fe72148e48aa56bf0190c7765e7",
    "lookup_anchor_slot": 29,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0x4328c504c12c3f9c731d7db8552b11ebf689abedb1ba68737a52abb81dfa0067",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x32333435363738393a3b"
      },
      "refine_load": {
        "gas_used": 66401,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}
```



### ALGO(6) 

{
  "package_spec": {
    "hash": "0x405d6952331e6712a53ea3d3615e4aca7ee722de2a0ec6e8bbd05342a04bf8fb",
    "length": 310,
    "erasure_root": "0x2f38fc9a00206ad94840040fe4132cef460bfabd49ccdd84186835cc60561e9d",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0xdee970744e9f4a02740d7add884c2695977069a61b7a2ad9ac3f04c9895a047b",
    "state_root": "0x73eeae8cd23e05204e63beef789512c371a4b99baf3c2bf8cfdf5b5d8da9d7f8",
    "beefy_root": "0x916b6b18935fd1a1487ed49dbaa026c4ca24400526e0fc1060b04a6b751737a0",
    "lookup_anchor": "0xdee970744e9f4a02740d7add884c2695977069a61b7a2ad9ac3f04c9895a047b",
    "lookup_anchor_slot": 32,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0xed7748d819cb8ea0f6f628007275bbd47a032662a927beb6fbe4c14665623d4d",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x3c3d3e3f404142434445"
      },
      "refine_load": {
        "gas_used": 44946,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}

## ALGO(7)  workPackageHash=0x553dec577f43a55f9dd51ae04f44388ba72b8530bead4f100aac726df0e671b1 
```
{
  "package_spec": {
    "hash": "0x8ba47f8dfcee01962f85b2fab4476097a711f725705beca0de522062f7ca1955",
    "length": 310,
    "erasure_root": "0x9330b085e6f15f1ae2614e0394fbf3a84775a3b185ba9dfeda31b5b175b33b45",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x469824cf1773906077286237530f5adf0e643bc3778e498bd37d79886307ebee",
    "state_root": "0x1c9f75003ace9babd01238ca427ea3a1f56984f4081249a9582bc76dc367670f",
    "beefy_root": "0x09bb3778383f50472a65de1557a1967172e77ea79e8ebd910f8577d73e217471",
    "lookup_anchor": "0x469824cf1773906077286237530f5adf0e643bc3778e498bd37d79886307ebee",
    "lookup_anchor_slot": 35,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0x58ea96694d0c8dcd5225fddd03971153d80e1ddb1aed8f99084732ff0396a52c",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x464748494a4b4c4d4e4f"
      },
      "refine_load": {
        "gas_used": 98669,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}
```



### ALGO(8) 0xc1b2076919a0c1b6a6a0af44c0384e273f080e312300c1d076fc6a81796274a9

```
{
  "package_spec": {
    "hash": "0xdf475c90abd45355f3e01185f73443d0d9d1bc1fad53a41b67b0f95333400a4d",
    "length": 310,
    "erasure_root": "0xb91f5f06144248a45b787591f1b15855c4c7ed77bca357315c6d8dfe76be19be",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x1ffdf335c9658a20d3db3d5d85a28795d416c7a6729108f47e94830a0f58bfe2",
    "state_root": "0xd4e4f3ebe99ea6f7da10345885909754b590768ad3b12df21ee4e8840c23c1bf",
    "beefy_root": "0x1e37ef4d5f58ead0b33923ac11b68c93f6a980de86dbef3144b5ffcb57e4cda0",
    "lookup_anchor": "0x1ffdf335c9658a20d3db3d5d85a28795d416c7a6729108f47e94830a0f58bfe2",
    "lookup_anchor_slot": 37,
    "prerequisites": []
  },
  "core_index": 0,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0x4f1ca147b7b434aa8ab29c4582141e81a2bc4c3bd4639f02fe8efadeffaa1244",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x50515253545556575859"
      },
      "refine_load": {
        "gas_used": 93753,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}
```


### ALGO(9) 0xd1f15a034cc0fec6579b6e8327d3170c97daa8e77011f9cc78cf0f8a9d60edc6 

```
{
  "package_spec": {
    "hash": "0xbe22c3536b346cc1e3b02047244c9f53fe8de1d911e5bbd4963a25cd06f57786",
    "length": 310,
    "erasure_root": "0x11d6ffe8bc51261049b61d6333e72cf9a7cfa0b26fbebbae18a71e0576e40148",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x0b1d4b6aac20c253f8015060894053cd25b76a4a5fb1e902979b3f466f9bd681",
    "state_root": "0xf3dd8b8d5ec486bd0ebbae3e4a97a4c7882c620bd9fb1d53bc53bf8d44434bc3",
    "beefy_root": "0xde9a8d5dd28b20bc55550845a9a7c7e498e997f5d3ffed0e7ff35093258bf1c5",
    "lookup_anchor": "0x0b1d4b6aac20c253f8015060894053cd25b76a4a5fb1e902979b3f466f9bd681",
    "lookup_anchor_slot": 40,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0xdbad0cdf5f85244335ad125a2176700b359ab9082b22ac9c8a01202927aaffc9",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x5a5b5c5d5e5f60616263"
      },
      "refine_load": {
        "gas_used": 123960,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}
```


### ALGO(10) workPackageHash=0xaeb453bd5992a04aba9fa7622183fc4dc1d8ee7fdb4e4d1e140408f26671a96b

```
{
  "package_spec": {
    "hash": "0x6d68757bfda0e35ca577e355e42968411fbc66d040516cfb4459b73d2148edd6",
    "length": 310,
    "erasure_root": "0xdeaa6f3e5fe465cc7524808ebc3a8c5b0215e8c324bd48e3a594b11d29da13e3",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x607614e61e0ec02704b6df0d3b3b43124a968c52d1f44147502720548c1581b6",
    "state_root": "0x65a32d3437639f4b72df6535a0a430a58617c0e536fdca8837ed8a70ab3daef5",
    "beefy_root": "0x04159907837b82d09baddddb5cd36410a1fae13d9ca1f3770a96c521fb59ee2f",
    "lookup_anchor": "0x607614e61e0ec02704b6df0d3b3b43124a968c52d1f44147502720548c1581b6",
    "lookup_anchor_slot": 43,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0x718c98271b4b0eda38fdde5d18ed3a9598250faa3deeb6f5ca32f69e5bf9b194",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x6465666768696a6b6c6d"
      },
      "refine_load": {
        "gas_used": 80297,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}



INFO [07-23|00:09:46.387] ALGO(11)                                 workPackageHash=0xc0181e1da63eafd059bd8f251f25f340f4671e7bd9b3b0d617f338c6a825e135 wr='{
  "package_spec": {
    "hash": "0x5199ce3e8262067c4770afa0fc59b8a24995bd6358419b6afb0f0b1da978162b",
    "length": 310,
    "erasure_root": "0x9cf40ca70f410b10971a4378ce70c0f81a8a8935fc7c23a146aeff4590bfc3b9",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x1de91e1634b298e5e8042cea72361cede1cc6eafe0351c9125e8aa228ae35c09",
    "state_root": "0x7183eac35437504ca28715933e834ec8dbfb11fde1785cc1b7d728ea0522279b",
    "beefy_root": "0x26bf6b47de31f1c72f99d2bff9a760135b5c2aa83e488746f5b7b9c2bc9dad6a",
    "lookup_anchor": "0x1de91e1634b298e5e8042cea72361cede1cc6eafe0351c9125e8aa228ae35c09",
    "lookup_anchor_slot": 45,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0x7f7549283009e216b76defae3558f829ee2ae2932986ea4cf2636c9d1b2dd4a9",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x6e6f7071727374757677"
      },
      "refine_load": {
        "gas_used": 34775,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}



INFO [07-23|00:10:06.391] ALGO(12)                                 workPackageHash=0x795ba6a55a1c042c4f85d203b8882bca499dfaf8403f0fd3ca4a599e0d811d21 wr='{
  "package_spec": {
    "hash": "0xfc77c78828e19dc2428045197ded5b0e5f0795dcb6a6272818b5c15175532bb2",
    "length": 310,
    "erasure_root": "0x38ab8995d13684d22db85580e66e1db36d34f700bb5229bf67f6039ced59f34c",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x2fd70668de05dfe3457ef3813397ae5ae1c22a750b3e9076b37331836a330dc6",
    "state_root": "0x74c39d839f127471e8a3ec8e1c0138377dd1a72a720ced904789d9196ff8eacb",
    "beefy_root": "0xfd2c50c49f46018709c6aad7712091f3655900e7115b37ed7da6f636f2e64ce0",
    "lookup_anchor": "0x2fd70668de05dfe3457ef3813397ae5ae1c22a750b3e9076b37331836a330dc6",
    "lookup_anchor_slot": 47,
    "prerequisites": []
  },
  "core_index": 0,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0xbd50f9b2a0387135fc016311ca31cfe584852185a47d42366b724c3cdeed727e",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x78797a7b7c7d7e7f8081"
      },
      "refine_load": {
        "gas_used": 79266,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}



INFO [07-23|00:10:22.392] ALGO(13)                                 workPackageHash=0x0e35ba28bfa41cfe667045504796083fdeb68faeaa9dd51f20c97c53eaee236f wr='{
  "package_spec": {
    "hash": "0xf85ce5368a2382af5c35ee699f9d50933ef0d84e6a694a105caa85231dab799e",
    "length": 310,
    "erasure_root": "0xd9986e0216e9498d4bd6ece37c36531664054c9caa70dc90483f9d156cd00a59",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x1b6078abc1ac13f7ff526eaa8e747aa4dd5d50e45a81b247607777e9314fa2de",
    "state_root": "0x1e719ccef2b22abc4ff90cacd58f6c0f94ac17858573c22594243b454429ce14",
    "beefy_root": "0xe0567ae9630a9f9700ae8b77226e48e7db2d728dd48118d44dc381700ab418db",
    "lookup_anchor": "0x1b6078abc1ac13f7ff526eaa8e747aa4dd5d50e45a81b247607777e9314fa2de",
    "lookup_anchor_slot": 51,
    "prerequisites": []
  },
  "core_index": 0,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0xe70843b0f8c25fcb98612e89736926ff25db137f2287bc2e8db10935cb1b3a13",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x82838485868788898a8b"
      },
      "refine_load": {
        "gas_used": 67209,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}


INFO [07-23|00:10:36.403] ALGO(14)                                 workPackageHash=0x1babd48d9a8b5c1420e9a51588db10aceb5a29ddab94685ac1cd1d401bd45a90 wr='{
  "package_spec": {
    "hash": "0x8d3647c49042a2849dd41c647727ce9d767157f9698a1ddac596883726658cdd",
    "length": 310,
    "erasure_root": "0x675e322c44b2777b4ede9179826a2a0ec88b36567e3d3c32bf9332a02547a52d",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x38ecc6b5f6748a6b43d9007f794e75a96291436f0437e05316a4911c810edac0",
    "state_root": "0xe3ddac69e87a6f3e7d39248ae616222df67238097af55bc36d7f2190402b7061",
    "beefy_root": "0x81ea2bb6aac2cb8a2ab743c2cde9668fe5c8362e73037ec449801d2f620afdca",
    "lookup_anchor": "0x38ecc6b5f6748a6b43d9007f794e75a96291436f0437e05316a4911c810edac0",
    "lookup_anchor_slot": 53,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0x75e0b824c6ee37b4f6cf3d017eddb63857b8b066a00e12cacb24bdec42757617",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x8c8d8e8f909192939495"
      },
      "refine_load": {
        "gas_used": 50764,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}


INFO [07-23|00:11:02.405] ALGO(15)                                 workPackageHash=0xfd86e29bb1bcac0ceba7f51d7c431d85415ca844290c2c92dcf0d408ffe6963a wr='{
  "package_spec": {
    "hash": "0xbbec9ed659891ae8992b71ce29d58b79efca4a6164ea343c075dad08b76ebe5a",
    "length": 310,
    "erasure_root": "0x5a30739caf6557bb0cc640289a8c3c7a093116d5b01ec3c4a9920da9ba6552fb",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0xdad3f5aa1b8d577b027fb074b09fc7402080a3f1b5492d67a91f11b4d19c0e68",
    "state_root": "0xc5eb4032c36faf5e3ae3bd169b4913a3256daa35c2d9272a786b3a2aba767829",
    "beefy_root": "0x111b33788c19edf2e77776be0f15f358eef38a3b18a3a332b73753067ede481c",
    "lookup_anchor": "0xdad3f5aa1b8d577b027fb074b09fc7402080a3f1b5492d67a91f11b4d19c0e68",
    "lookup_anchor_slot": 56,
    "prerequisites": []
  },
  "core_index": 1,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0xf4195cad7e5148da3f711c92408fdfa25ec8edad732592998188f2c69ee8e5a0",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0x969798999a9b9c9d9e9f"
      },
      "refine_load": {
        "gas_used": 50144,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}



INFO [07-23|00:11:18.406] ALGO(16)                                 workPackageHash=0x4719e74dc77c51a6d2bb92507833bd1e38b1a750d53313c1053db4f31211a04f wr='{
  "package_spec": {
    "hash": "0x7fb021a9b3add81385a46abb058aa01c292ce1ec62e408fd7e7ad4d6144d6d01",
    "length": 310,
    "erasure_root": "0x806178d66eee834bcb01e126375ab4f608ee6bd79fb722f500dc38fd5c98814d",
    "exports_root": "0x0000000000000000000000000000000000000000000000000000000000000000",
    "exports_count": 0
  },
  "context": {
    "anchor": "0x0819c25a3051a2bc2962878793798348ab16229a9aea94ffe3ebd51c6249708f",
    "state_root": "0x136e2904d3166a411f29536d2a70d5971839cb0e807e1e68f7a98ba7c2dab715",
    "beefy_root": "0xfab8d843c4fb36e26093d8a68924fb68d6c7899288e27548b36cf9950cc50f1c",
    "lookup_anchor": "0x0819c25a3051a2bc2962878793798348ab16229a9aea94ffe3ebd51c6249708f",
    "lookup_anchor_slot": 60,
    "prerequisites": []
  },
  "core_index": 0,
  "authorizer_hash": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db7",
  "auth_output": "0x",
  "segment_root_lookup": [],
  "results": [
    {
      "service_id": 20,
      "code_hash": "0x5631d0fd972ba010731bf0024f99af2c1d1fb37f1660a3a7a6e9355c06fc22e9",
      "payload_hash": "0x3446ec44815bf9b53a1f81a81b1c42c83706567fb66561b3244f08755c2276c5",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xce07e7687972cccfbfe304e4217db58cde9b833750fe557d4e543771b2214db714000000"
      },
      "refine_load": {
        "gas_used": 23172,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    },
    {
      "service_id": 10,
      "code_hash": "0x86a8cc6ee29ce01ec3987922beb2a8cc0c4569a6c502a7f1abb743006386e92f",
      "payload_hash": "0xfc0ba48b0ab879bbbe0e2e74ed5fedb9a451405729c4ad75eba9f790281f6712",
      "accumulate_gas": 5000000,
      "result": {
        "ok": "0xa0a1a2a3a4a5a6a7a8a9"
      },
      "refine_load": {
        "gas_used": 210824,
        "imports": 0,
        "extrinsic_count": 0,
        "extrinsic_size": 0,
        "exports": 0
      }
    }
  ],
  "auth_gas_used": 8
}