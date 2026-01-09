#ifndef ORCHARD_FFI_H
#define ORCHARD_FFI_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Error codes for FFI functions */
#define FFI_SUCCESS 0
#define FFI_ERROR_NULL_POINTER -1
#define FFI_ERROR_INVALID_INPUT -2
#define FFI_ERROR_COMPUTATION_FAILED -3

/* Compute Sinsemilla Merkle tree root from leaves */
int32_t sinsemilla_compute_root(const uint8_t *leaves, size_t leaves_len, uint8_t *root_out);

/* Compute root and frontier from leaves */
int32_t sinsemilla_compute_root_and_frontier(const uint8_t *leaves,
                                             size_t leaves_len,
                                             uint8_t *root_out,
                                             uint8_t *frontier_out,
                                             size_t *frontier_len_out);

/* Append new leaves to existing tree using frontier */
int32_t sinsemilla_append_with_frontier(const uint8_t *old_root,
                                        uint64_t old_size,
                                        const uint8_t *frontier,
                                        size_t frontier_len,
                                        const uint8_t *new_leaves,
                                        size_t new_leaves_len,
                                        uint8_t *new_root_out);

/* Append new leaves and return updated frontier */
int32_t sinsemilla_append_with_frontier_update(const uint8_t *old_root,
                                               uint64_t old_size,
                                               const uint8_t *frontier,
                                               size_t frontier_len,
                                               const uint8_t *new_leaves,
                                               size_t new_leaves_len,
                                               uint8_t *new_root_out,
                                               uint8_t *new_frontier_out,
                                               size_t *new_frontier_len_out);

/* Generate Merkle proof for commitment at given position */
int32_t sinsemilla_generate_proof(const uint8_t *commitment,
                                  uint64_t tree_position,
                                  const uint8_t *all_commitments,
                                  size_t commitments_len,
                                  uint8_t *proof_out,
                                  size_t *proof_len_out);

#ifdef __cplusplus
}
#endif

#endif /* ORCHARD_FFI_H */