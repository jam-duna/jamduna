#ifndef RING_VRF_H
#define RING_VRF_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdbool.h>
#include <stddef.h>

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


#ifdef __cplusplus
}
#endif

#endif 
