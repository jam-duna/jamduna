// CUDA implementation of batch RSA modular exponentiation using CGBN
#include <stdint.h>
#include <cuda_runtime.h>
#include <cgbn/cgbn.h>

extern "C" {

typedef struct {
    uint32_t limb[64];  // 2048 bits = 64 * 32-bit limbs
} uint2048_t;

typedef cgbn_context_t<cgbn_default_tpi> context_t;
typedef cgbn_env_t<context_t, 2048> env2048_t;

__global__ void rsa_batch_modexp_kernel(
    const uint2048_t* bases,
    const uint2048_t* exps,
    uint2048_t* results,
    uint2048_t n,
    int count
) {
    int i = blockIdx.x * blockDim.x + threadIdx.x;
    if (i >= count) return;

    context_t ctx;
    env2048_t env(ctx.report_monitor, cgbn_no_checks, i);

    env2048_t::cgbn_t base, exp, mod, result;

    // Load values from memory into CGBN types
    cgbn_load(env, base, &(bases[i].limb[0]));
    cgbn_load(env, exp, &(exps[i].limb[0]));
    cgbn_load(env, mod, &(n.limb[0]));

    // Compute modular exponentiation: result = base^exp mod n
    cgbn_modular_power(env, result, base, exp, mod);

    // Store result back to memory
    cgbn_store(env, &(results[i].limb[0]), result);
}

int rsa_batch_modexp(
    const void* bases_ptr,
    const void* exps_ptr,
    void* results_ptr,
    const void* n_ptr,
    int count
) {
    const uint2048_t* bases = (const uint2048_t*)bases_ptr;
    const uint2048_t* exps = (const uint2048_t*)exps_ptr;
    uint2048_t* results = (uint2048_t*)results_ptr;
    const uint2048_t* n = (const uint2048_t*)n_ptr;

    if (count <= 0) return 0;

    // Allocate device memory
    uint2048_t *d_bases, *d_exps, *d_results;
    size_t size = count * sizeof(uint2048_t);

    cudaError_t err;

    err = cudaMalloc(&d_bases, size);
    if (err != cudaSuccess) return -1;

    err = cudaMalloc(&d_exps, size);
    if (err != cudaSuccess) {
        cudaFree(d_bases);
        return -1;
    }

    err = cudaMalloc(&d_results, size);
    if (err != cudaSuccess) {
        cudaFree(d_bases);
        cudaFree(d_exps);
        return -1;
    }

    // Copy input data to device
    err = cudaMemcpy(d_bases, bases, size, cudaMemcpyHostToDevice);
    if (err != cudaSuccess) {
        cudaFree(d_bases);
        cudaFree(d_exps);
        cudaFree(d_results);
        return -1;
    }

    err = cudaMemcpy(d_exps, exps, size, cudaMemcpyHostToDevice);
    if (err != cudaSuccess) {
        cudaFree(d_bases);
        cudaFree(d_exps);
        cudaFree(d_results);
        return -1;
    }

    // Launch kernel
    int blockSize = 256;
    int gridSize = (count + blockSize - 1) / blockSize;
    rsa_batch_modexp_kernel<<<gridSize, blockSize>>>(
        d_bases, d_exps, d_results, *n, count
    );

    // Check for kernel launch errors
    err = cudaGetLastError();
    if (err != cudaSuccess) {
        cudaFree(d_bases);
        cudaFree(d_exps);
        cudaFree(d_results);
        return -1;
    }

    // Wait for kernel to complete
    err = cudaDeviceSynchronize();
    if (err != cudaSuccess) {
        cudaFree(d_bases);
        cudaFree(d_exps);
        cudaFree(d_results);
        return -1;
    }

    // Copy results back to host
    err = cudaMemcpy(results, d_results, size, cudaMemcpyDeviceToHost);
    if (err != cudaSuccess) {
        cudaFree(d_bases);
        cudaFree(d_exps);
        cudaFree(d_results);
        return -1;
    }

    // Cleanup device memory
    cudaFree(d_bases);
    cudaFree(d_exps);
    cudaFree(d_results);

    return 0;
}

} // extern "C"
