#ifndef ERASURE_CODING_H
#define ERASURE_CODING_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stddef.h>

// Function to encode segments
int encode_segments(
    const unsigned char* input,
    size_t input_len,
    unsigned char* output,
    size_t output_len
);


// Function to decode a segment
int decode_segment(
    int segment,
    const unsigned char* input,
    const unsigned char* available,
    const unsigned char* output
);


#ifdef __cplusplus
}
#endif

#endif // ERASURE_CODING_H
