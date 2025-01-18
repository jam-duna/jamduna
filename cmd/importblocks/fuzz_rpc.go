package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

func decodeData(contentType string, bodyBytes []byte, targetType reflect.Type) (interface{}, error) {
	var result interface{}
	var err error

	switch contentType {
	case "application/json":
		// Decode JSON
		result = reflect.New(targetType).Interface() // Create a new instance of the target type
		err = json.Unmarshal(bodyBytes, result)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON data: %w", err)
		}
		// Dereference pointer to the actual value
		return reflect.ValueOf(result).Elem().Interface(), nil
	case "application/octet-stream":
		// Decode Binary
		decoded, _, err := types.Decode(bodyBytes, targetType)
		if err != nil {
			return nil, fmt.Errorf("invalid binary data: %w", err)
		}
		return decoded, nil
	//TODO: support hex encoding
	default:
		// Fallback: Try JSON first
		result = reflect.New(targetType).Interface() // Create a new instance of the target type
		err = json.Unmarshal(bodyBytes, result)
		if err != nil {
			// If JSON fails, try binary decoding
			fmt.Println("JSON decoding failed, attempting binary decoding...")
			decoded, _, err := types.Decode(bodyBytes, targetType)
			if err != nil {
				return nil, fmt.Errorf("unable to decode data as JSON or Binary: %w", err)
			}
			return decoded, nil
		}
		// Dereference pointer to the actual value
		return reflect.ValueOf(result).Elem().Interface(), nil
	}
}

type FuzzedResult struct {
	Mutated bool
	STF     *statedb.StateTransition
}

// ValidationResult is the JSON structure for validate responses.
type ValidationResult struct {
	Valid bool                    `json:"valid"`
	Error string                  `json:"error,omitempty"`
	STF   statedb.StateTransition `json:"stf,omitempty"`
}

func handleSTFValidation(sdb_storage *storage.StateDBStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}

		// Read request body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			sendValidateResult(w, http.StatusBadRequest, ValidationResult{
				Valid: false,
				Error: "Failed to read request body",
			})
			return
		}

		contentType := r.Header.Get("Content-Type")
		fmt.Printf("[%v] Received STF bytes(len=%v)\n", contentType, len(bodyBytes))

		// Decode the incoming data into a statedb.StateTransition
		decoded, err := decodeData(contentType, bodyBytes, reflect.TypeOf(statedb.StateTransition{}))
		if err != nil {
			sendValidateResult(w, http.StatusBadRequest, ValidationResult{
				Valid: false,
				Error: fmt.Sprintf("Decoding failed: %v", err.Error()),
			})
			return
		}

		stf, ok := decoded.(statedb.StateTransition)
		if !ok {
			sendValidateResult(w, http.StatusInternalServerError, ValidationResult{
				Valid: false,
				Error: "Failed to type assert to StateTransition",
			})
			return
		}

		// Validate the state transition
		stfErr := statedb.CheckStateTransition(sdb_storage, &stf, nil)
		if stfErr != nil {
			errorStr := jamerrors.GetErrorStr(stfErr)
			sendValidateResult(w, http.StatusBadRequest, ValidationResult{
				Valid: false,
				Error: errorStr,
			})
			return
		}

		// If everything is valid, return a success JSON response
		sendValidateResult(w, http.StatusOK, ValidationResult{
			Valid: true,
			Error: "",
			STF:   stf,
		})
	}
}

// Helper to write ValidationResult as JSON
func sendValidateResult(w http.ResponseWriter, code int, res ValidationResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(res) // ignoring encode error for brevity
}

func handleFuzz(sdb_storage *storage.StateDBStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}

		// Read the request body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		var stf statedb.StateTransition

		contentType := r.Header.Get("Content-Type")
		fmt.Printf("[%v] Received STF bytes(len=%v)\n", contentType, len(bodyBytes))
		decoded, err := decodeData(contentType, bodyBytes, reflect.TypeOf(statedb.StateTransition{}))
		if err != nil {
			http.Error(w, fmt.Sprintf("%v", err), http.StatusBadRequest)
			fmt.Println("Decoding failed:", err)
			return
		}

		// Type assert to statedb.StateTransition
		stf, ok := decoded.(statedb.StateTransition)
		if !ok {
			fmt.Println("Failed to type assert to StateTransition")
			http.Error(w, "Failed to type assert to StateTransition", http.StatusInternalServerError)
			return
		}

		// Fuzzing
		// TODO: filter modes from fuzzer but accept all modes for now
		//modes := []string{"safrole", "disputes", "fallback", "guarantees", "reports", "assurances"}
		fuzzed := false
		modes := []string{"assurances"}
		mutatedStf, expectedErr, possibleErrs := selectImportBlocksError(sdb_storage, modes, &stf)
		if expectedErr != nil {
			fmt.Printf("Expected error: %v | %v possibleErrs = %v\n", jamerrors.GetErrorStr(expectedErr), len(possibleErrs), jamerrors.GetErrorStrs(possibleErrs))
			errActual := statedb.CheckStateTransition(sdb_storage, mutatedStf, nil)
			if errActual == expectedErr {
				fuzzed = true
				log.Printf("[fuzzed!] %v", jamerrors.GetErrorStr(errActual))
			} else {
				// Theoratically fuzzavle but we are unable to fuzz correctly ourselves
				log.Printf("[fuzzed failed!] Actual %v | Expected %v", jamerrors.GetErrorStr(errActual), jamerrors.GetErrorStr(expectedErr))
			}
		}

		fuzzRes := FuzzedResult{
			Mutated: fuzzed,
			STF:     mutatedStf,
		}
		if fuzzRes.STF == nil {
			fuzzRes.STF = &stf
		}

		// Respond with the decoded or processed STF
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fuzzRes)
	}
}

func runRPCServer(sdbStorage *storage.StateDBStorage) {
	http.HandleFunc("/fuzz", handleFuzz(sdbStorage))              //accept stf via rpc
	http.HandleFunc("/validate", handleSTFValidation(sdbStorage)) //accept stf validation rpc

	log.Println("RPC server listening on :8088")
	log.Fatal(http.ListenAndServe(":8088", nil))
}
