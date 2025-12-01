# RSA CUDA Library

This directory contains the CUDA implementation for GPU-accelerated batch RSA modular exponentiation using NVIDIA's CGBN (CUDA GPU Big Number) library.

## Prerequisites

1. **CUDA Toolkit** (11.0 or later)
   - Download from [NVIDIA CUDA Toolkit](https://developer.nvidia.com/cuda-downloads)
   - Ensure `nvcc` is in your PATH

2. **CGBN Library**
   - Clone from [NVIDIA CGBN GitHub](https://github.com/NVlabs/CGBN)
   - By default, the build expects CGBN at `../cgbn/include`
   - You can override this by setting the `CGBN_INCLUDE_DIR` CMake variable

3. **GPU Requirements**
   - NVIDIA GPU with compute capability 7.0 or higher (Volta, Turing, Ampere, or newer)
   - Examples: V100, RTX 2080, RTX 3090, A100, etc.

## Setup

### 1. Install CGBN

```bash
cd /path/to/your/workspace
git clone https://github.com/NVlabs/CGBN.git cgbn
```

### 2. Build the CUDA Library

```bash
cd librsa_cuda
mkdir build && cd build
cmake .. -DCGBN_INCLUDE_DIR=/path/to/cgbn/include
cmake --build . --config Release
```

### 3. Build Rust Library with GPU Support

From the `cambrian-accumulator` root directory:

```bash
cargo build --features gpu
```

Or to run tests:

```bash
cargo test --features gpu
```

## Architecture

- **rsa_cuda_cgbn.cu**: CUDA kernel implementation
  - `rsa_batch_modexp_kernel`: GPU kernel that performs parallel modular exponentiation
  - `rsa_batch_modexp`: Host function that manages memory and launches the kernel

- **CMakeLists.txt**: Build configuration for the CUDA library

## Performance Notes

- The GPU implementation is most efficient for **large batches** (typically > 1000 operations)
- For small batches, CPU implementations may be faster due to GPU memory transfer overhead
- Batch sizes are limited by available GPU memory (each 2048-bit number requires 256 bytes)

## Troubleshooting

### CGBN Not Found

If CMake cannot find CGBN:

```bash
cmake .. -DCGBN_INCLUDE_DIR=/absolute/path/to/cgbn/include
```

### CUDA Compute Capability Errors

If your GPU has a different compute capability, modify `CMakeLists.txt`:

```cmake
set(CMAKE_CUDA_ARCHITECTURES 70 75 80 86)  # Adjust these values
```

Common compute capabilities:
- 7.0: V100
- 7.5: RTX 2080, RTX 2080 Ti
- 8.0: A100
- 8.6: RTX 3090, RTX 3080

### Library Linking Issues

Ensure the CUDA library path is in your `LD_LIBRARY_PATH` (Linux) or `DYLD_LIBRARY_PATH` (macOS):

```bash
export LD_LIBRARY_PATH=/path/to/librsa_cuda/build:$LD_LIBRARY_PATH
```

## Testing

Run the CUDA-enabled tests:

```bash
cargo test --features gpu test_batch_exp
```

This will verify that the GPU implementation produces correct results.
