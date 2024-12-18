
#include <stdint.h>
#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

 /**
   * Verifies a Groth16 proof using the given verification key, proof, and public values.
   * 
   * @param proof A pointer to the proof bytes.
   * @param vk A pointer to the verification key bytes.
   * @param public_values A pointer to the public values bytes.
   * @param pub_values_len The length of the public values in bytes.
   * 
   * @return 1 if the proof is verified successfully, 0 otherwise.
   */
  int sp1_groth16_verify(
    const uint8_t* proof,
    const uint8_t* vk,
    const uint8_t* public_values,
    size_t pub_values_len
  );

#ifdef __cplusplus
}
#endif
