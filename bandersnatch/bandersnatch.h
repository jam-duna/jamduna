#include <stdint.h>
#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

void get_public_key(
    const unsigned char* seed_bytes,
    size_t seed_len,
    unsigned char* pub_key,
    size_t pub_key_len
);

void get_private_key(
    const unsigned char* seed_bytes,
    size_t seed_len,
    unsigned char* secret_bytes,
    size_t secret_len
);

void ietf_vrf_sign(
    const unsigned char* private_key_bytes,
    size_t private_key_len,
    const unsigned char* vrf_input_data_bytes,
    size_t vrf_input_data_len,
    const unsigned char* aux_data_bytes,
    size_t aux_data_len,
    unsigned char* sig,
    size_t sig_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

int ietf_vrf_verify(
    const unsigned char* pub_key_bytes,
    size_t pub_key_len,
    const unsigned char* signature_bytes,
    size_t signature_len,
    const unsigned char* vrf_input_data_bytes,
    size_t vrf_input_data_len,
    const unsigned char* aux_data_bytes,
    size_t aux_data_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

int ring_vrf_sign(
    const unsigned char* private_key_bytes,
    size_t private_key_len,
    const unsigned char* ring_set_bytes,
    size_t ring_set_len,
    size_t ring_size,
    const unsigned char* vrf_input_data_bytes,
    size_t vrf_input_data_len,
    const unsigned char* aux_data_bytes,
    size_t aux_data_len,
    unsigned char* sig,
    size_t sig_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
    /*
    ,size_t prover_idx
    */
);

void get_ring_commitment(
    const unsigned char* ring_set_bytes,
    size_t ring_set_len,
    size_t ring_size,
    unsigned char* commitment,
    size_t commitment_len
);

int ring_vrf_verify(
    const unsigned char* pubkeys_bytes,
    size_t pubkeys_length,
    size_t ring_size,
    const unsigned char* signature_bytes,
    size_t signature_len,
    const unsigned char* vrf_input_data_bytes,
    size_t vrf_input_data_len,
    const unsigned char* aux_data_bytes,
    size_t aux_data_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

int get_ietf_vrf_output(
    const unsigned char* sig,
    size_t sig_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

int get_ring_vrf_output(
    const unsigned char* sig,
    size_t sig_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

void get_secret_key(
    const unsigned char* seed_bytes,
    size_t seed_len,
    unsigned char* secret_key,
    size_t secret_key_len
);

void get_double_pubkey(
    const unsigned char* seed_bytes,
    size_t seed_len,
    unsigned char* pub_key,
    size_t pub_key_len
);

void get_pubkey_g2(
    const unsigned char* seed_bytes,
    size_t seed_len,
    unsigned char* pub_key,
    size_t pub_key_len
);

void sign(
    const unsigned char* secret_key_bytes,
    size_t secret_key_len,
    const unsigned char* message_bytes,
    size_t message_len,
    unsigned char* signature,
    size_t signature_len
);

int verify(
    const unsigned char* pub_key_bytes,
    size_t pub_key_len,
    const unsigned char* message_bytes,
    size_t message_len,
    const unsigned char* signature_bytes,
    size_t signature_len
);

void aggregate_sign(
    const unsigned char* signatures_bytes,
    size_t signatures_len,
    const unsigned char* message_bytes,
    size_t message_len,
    unsigned char* aggregated_signature,
    size_t aggregated_signature_len
);

int aggregate_verify_by_signature(
    const unsigned char* pub_keys_bytes,
    size_t pub_keys_len,
    const unsigned char* message_bytes,
    size_t message_len,
    const unsigned char* signature_bytes,
    size_t signature_len
);

void* create_verifier(
    const unsigned char* ring_set_bytes,
    size_t ring_set_len,
    size_t ring_size
);

int ring_vrf_verify_with_verifier(
    void* verifier_ptr,
    const unsigned char* signature_bytes,
    size_t signature_len,
    const unsigned char* vrf_input_data_bytes,
    size_t vrf_input_data_len,
    const unsigned char* aux_data_bytes,
    size_t aux_data_len,
    unsigned char* vrf_output,
    size_t vrf_output_len
);

void free_verifier(void* verifier_ptr);

void init_cache();

void encode(
    const unsigned char* input_ptr,
    size_t input_len,
    size_t V,
    unsigned char* output_ptr,
    size_t shard_size
);

void decode(
    const unsigned char* shards_ptr,
    const unsigned int* indexes_ptr,
    size_t V,
    size_t shard_size,
    unsigned char* output_ptr,
    size_t output_size
);

#ifdef __cplusplus
}
#endif
