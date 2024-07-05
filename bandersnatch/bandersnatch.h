#include <stdint.h>
#include <stdlib.h>

// Declare the function signature
extern int ring_vrf_verify_external(
    const uint8_t* pubkeys_bytes,
    uint64_t pubkeys_length,
    const uint8_t* signature_bytes,
    uint64_t signature_hex_len,
    const uint8_t* vrf_input_data_bytes,
    uint64_t vrf_input_data_len,
    const uint8_t* aux_data_bytes,
    uint64_t aux_data_len,
    uint8_t* vrf_output
);
