#ifndef RING_VRF_H
#define RING_VRF_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdbool.h>
#include <stddef.h>

// Function to sign Ring VRF signature with provided public keys and secret
size_t ring_vrf_sign_c_pks(
    const uint8_t* secret,
    const uint8_t* domain,
    size_t domain_len,
    const uint8_t* message,
    size_t message_len,
    const uint8_t* transcript,
    size_t transcript_len,
    const uint8_t* pks,
    size_t pks_len,
    uint8_t* signature,
    size_t signature_len
);

// Function to verify Ring VRF signature with provided public keys
size_t ring_vrf_verify_c_pks(
    const uint8_t* domain,
    size_t domain_len,
    const uint8_t* message,
    size_t message_len,
    const uint8_t* transcript,
    size_t transcript_len,
    const uint8_t* pks,
    size_t pks_len,
    const uint8_t* signature,
    size_t signature_len
);

size_t ring_vrf_sign_c(
    const char* secret,
    const char* domain,
    size_t domain_len,
    const char* message,
    size_t message_len,
    const char* transcript,
    size_t transcript_len,
    char* signature,
    size_t signature_len
);

size_t ring_vrf_verify_c(
    const char* pks,
    const char* pubkey,
    size_t pubkey_len,
    const char* domain,
    size_t domain_len,
    const char* message,
    size_t message_len,
    const char* transcript,
    size_t transcript_len,
    char* signature,
    size_t signature_len
);

size_t ring_vrf_public_key(
    const unsigned char* secret,
    unsigned char* pubkey,
    size_t pubkey_len
);

#ifdef __cplusplus
}
#endif

#endif
