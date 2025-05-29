package types

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSingleCodecTest(t *testing.T) {
	t.Skip("Temporarily disabled for debugging")
	jsonstring := `{
        "vals_current": [
            {
                "blocks": 1,
                "tickets": 3,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 0,
                "assurances": 0
            },
            {
                "blocks": 0,
                "tickets": 0,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 1,
                "assurances": 0
            },
            {
                "blocks": 0,
                "tickets": 0,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 1,
                "assurances": 0
            },
            {
                "blocks": 0,
                "tickets": 0,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 1,
                "assurances": 0
            },
            {
                "blocks": 3,
                "tickets": 6,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 0,
                "assurances": 0
            },
            {
                "blocks": 1,
                "tickets": 3,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 0,
                "assurances": 0
            }
        ],
        "vals_last": [
            {
                "blocks": 0,
                "tickets": 0,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 0,
                "assurances": 0
            },
            {
                "blocks": 0,
                "tickets": 0,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 0,
                "assurances": 0
            },
            {
                "blocks": 0,
                "tickets": 0,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 0,
                "assurances": 0
            },
            {
                "blocks": 0,
                "tickets": 0,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 0,
                "assurances": 0
            },
            {
                "blocks": 0,
                "tickets": 0,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 0,
                "assurances": 0
            },
            {
                "blocks": 0,
                "tickets": 0,
                "pre_images": 0,
                "pre_images_size": 0,
                "guarantees": 0,
                "assurances": 0
            }
        ],
        "cores": [
            {
                "gas_used": 0,
                "imports": 0,
                "extrinsic_count": 0,
                "extrinsic_size": 0,
                "exports": 0,
                "bundle_size": 270,
                "da_load": 0,
                "popularity": 0
            },
            {
                "gas_used": 0,
                "imports": 0,
                "extrinsic_count": 0,
                "extrinsic_size": 0,
                "exports": 0,
                "bundle_size": 0,
                "da_load": 0,
                "popularity": 0
            }
        ],
        "services": {
            "0": {
                "provided_count": 0,
                "provided_size": 0,
                "refinement_count": 1,
                "refinement_gas_used": 0,
                "imports": 0,
                "exports": 0,
                "extrinsic_size": 0,
                "extrinsic_count": 0,
                "accumulate_count": 0,
                "accumulate_gas_used": 0,
                "on_transfers_count": 0,
                "on_transfers_gas_used": 0
            }
        }
    }`

	var test_types = ValidatorStatistics{}
	err := json.Unmarshal([]byte(jsonstring), &test_types)
	if err != nil {
		t.Error(err)
	}
	encoding_bytes, err := Encode(test_types)
	if err != nil {
		t.Error(err)
	}
	// fmt.Printf("expected_bytes: %x\n", expected_bytes)
	fmt.Printf("encoding_bytes:\n")
	for i := 0; i < len(encoding_bytes); i += 20 {
		if i+20 > len(encoding_bytes) {
			fmt.Printf("%x \n", encoding_bytes[i:])
		} else {

			fmt.Printf("%x \n", encoding_bytes[i:i+20])
		}
	}
	// if len(expected_bytes) != len(encoding_bytes) {
	// 	t.Errorf("expected %d bytes, got %d bytes", len(expected_bytes), len(encoding_bytes))
	// }

}
