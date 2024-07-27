#include <stdint.h>
#include <stdlib.h>


extern void get_public_key(
    const unsigned char* seed_bytes,
    size_t seed_len,
    unsigned char* pub_key,
    size_t pub_key_len
);

extern void ietf_vrf_sign(
    const unsigned char* seed_bytes,
    size_t seed_len,
    const unsigned char* vrf_input_data_bytes,
    size_t vrf_input_data_len,
    const unsigned char* aux_data_bytes,
    size_t aux_data_len,
    unsigned char* sig,
    size_t sig_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);
  
extern void ring_vrf_sign(
    const unsigned char* seed_bytes,
    size_t seed_len,
    const unsigned char* ring_set_bytes,
    size_t ring_set_len,
    size_t prover_idx,
    const unsigned char* vrf_input_data_bytes,
    size_t vrf_input_data_len,
    const unsigned char* aux_data_bytes,
    size_t aux_data_len,
    unsigned char* sig,
    size_t sig_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

extern int ietf_vrf_verify(
    const unsigned char* ring_set_bytes,
    size_t ring_set_len,
    const unsigned char* signature_bytes,
    size_t signature_len,
    const unsigned char* vrf_input_data_bytes,
    size_t vrf_input_data_len,
    const unsigned char* aux_data_bytes,
    size_t aux_data_len,
    size_t signer_key_index,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

extern int ring_vrf_verify(
    const unsigned char* pubkeys_bytes,
    size_t pubkeys_length,
    const unsigned char* signature_bytes,
    size_t signature_hex_len,
    const unsigned char* vrf_input_data_bytes,
    size_t vrf_input_data_len,
    const unsigned char* aux_data_bytes,
    size_t aux_data_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);


extern int get_ietf_vrf_output(
    const unsigned char* sig,
    size_t sig_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

extern int get_ring_vrf_output(
    const unsigned char* sig,
    size_t sig_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

