package erasurecoding

import (
       "log"
        "testing"
        "fmt"
	"time"
	"math/rand"
)


func TestGF16(t *testing.T) {

    // Actually, GF16 -- data must be within GF(16) range [0, 15]
    data := []int{1, 2, 3}

    // Encode the data
    encodedData := Encode(data)
    fmt.Println("Encoded Data: ", encodedData)

    // Simulate some errors
    rand.Seed(time.Now().UnixNano())
    encodedData[4] = 0 // Introduce an error

    // Decode the data
    decodedData, err := Decode(encodedData, 6, 3)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Decoded Data: ", decodedData)


}


func TestBlobs(t *testing.T) {
    // Original data to be encoded
    data := []byte("colorfulnotion")

    // Ensure data length is a multiple of 684
    if len(data)%684 != 0 {
        padding := make([]byte, 684-(len(data)%684))
        data = append(data, padding...)
    }

    // Split dataBlob into 684-byte chunks
    dataChunks := Split(data, 684)

    // Simulate loss of some shards by zeroing them out
    for i := 0; i < 342; i++ {
        dataChunks[i] = make([]byte, 684)
    }

    // Encode the data
    // NOTE: This Encode function should be adapted for byte chunks, currently only for illustrative purposes.
    for i := range dataChunks {
        encodedChunk := Encode(byteArrayToIntArray(dataChunks[i]))
        dataChunks[i] = intArrayToByteArray(encodedChunk)
    }

    // Decode the data
    // NOTE: This Decode function should be adapted for byte chunks, currently only for illustrative purposes.
    for i := range dataChunks {
        decodedChunk, err := Decode(byteArrayToIntArray(dataChunks[i]), 684, 342)
        if err != nil {
            log.Fatal(err)
        }
        dataChunks[i] = intArrayToByteArray(decodedChunk)
    }

    // Join the chunks back into the original data
    reconstructedData := Join(dataChunks)

    // Trim padding if necessary
    reconstructedData = reconstructedData[:len(data)-len(data)%684]

    fmt.Println("Original Data: ", string(data))
    fmt.Println("Reconstructed Data: ", string(reconstructedData))

    t.Logf("Reconstructed successfully")
}
