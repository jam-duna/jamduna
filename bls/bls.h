
#include <stdint.h>
#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

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


#ifdef __cplusplus
}
#endif
