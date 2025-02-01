package fuzz

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
	switch contentType {
	case "application/json":
		result := reflect.New(targetType).Interface()
		if err := json.Unmarshal(bodyBytes, result); err != nil {
			return nil, fmt.Errorf("invalid JSON data: %w", err)
		}
		return reflect.ValueOf(result).Elem().Interface(), nil
	case "application/octet-stream":
		decoded, _, err := types.Decode(bodyBytes, targetType)
		if err != nil {
			return nil, fmt.Errorf("invalid binary data: %w", err)
		}
		return decoded, nil
	default:
		result := reflect.New(targetType).Interface()
		if err := json.Unmarshal(bodyBytes, result); err != nil {
			decoded, _, err := types.Decode(bodyBytes, targetType)
			if err != nil {
				return nil, fmt.Errorf("unable to decode data as JSON or Binary: %w", err)
			}
			return decoded, nil
		}
		return reflect.ValueOf(result).Elem().Interface(), nil
	}
}

type FuzzedResult struct {
	Mutated bool
	STF     *statedb.StateTransition
}

type ValidationResult struct {
	Valid bool                    `json:"valid"`
	Error string                  `json:"error,omitempty"`
	STF   statedb.StateTransition `json:"stf,omitempty"`
}

func handleSTFValidation(sdb_storage *storage.StateDBStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			sendValidateResult(w, http.StatusBadRequest, ValidationResult{Valid: false, Error: "Failed to read request body"})
			return
		}
		contentType := r.Header.Get("Content-Type")
		fmt.Printf("[%v] Received STF bytes(len=%v)\n", contentType, len(bodyBytes))
		decoded, err := decodeData(contentType, bodyBytes, reflect.TypeOf(statedb.StateTransition{}))
		if err != nil {
			sendValidateResult(w, http.StatusBadRequest, ValidationResult{Valid: false, Error: fmt.Sprintf("Decoding failed: %v", err.Error())})
			return
		}
		stf, ok := decoded.(statedb.StateTransition)
		if !ok {
			sendValidateResult(w, http.StatusInternalServerError, ValidationResult{Valid: false, Error: "Failed to type assert to StateTransition"})
			return
		}
		stfErr := statedb.CheckStateTransition(sdb_storage, &stf, nil)
		if stfErr != nil {
			errorStr := jamerrors.GetErrorStr(stfErr)
			sendValidateResult(w, http.StatusBadRequest, ValidationResult{Valid: false, Error: errorStr})
			return
		}
		sendValidateResult(w, http.StatusOK, ValidationResult{Valid: true, Error: "", STF: stf})
	}
}

func handleStateTransitionChallenge(sdb_storage *storage.StateDBStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		var stc statedb.StateTransitionChallenge
		contentType := r.Header.Get("Content-Type")
		decoded, err := decodeData(contentType, bodyBytes, reflect.TypeOf(statedb.StateTransitionChallenge{}))
		if err != nil {
			http.Error(w, fmt.Sprintf("%v", err), http.StatusBadRequest)
			return
		}
		stc, ok := decoded.(statedb.StateTransitionChallenge)
		if !ok {
			http.Error(w, "Failed to type assert to StateTransitionChallenge", http.StatusInternalServerError)
			return
		}
		ok, postStateSnapshotRaw, jamErr, _ := statedb.ComputeStateTransition(sdb_storage, &stc)
		if !ok {
			http.Error(w, "BadChallenge", http.StatusInternalServerError)
			return
		}
		if jamErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotAcceptable)
			stRespJamErr := JamError{Error: jamErr.Error()}
			_ = json.NewEncoder(w).Encode(stRespJamErr)
		} else if postStateSnapshotRaw != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(postStateSnapshotRaw)
		} else {
			http.Error(w, "UnknownError", http.StatusInternalServerError)
		}
	}
}

func sendValidateResult(w http.ResponseWriter, code int, res ValidationResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(res)
}

func handleFuzz(sdb_storage *storage.StateDBStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}
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
		stf, ok := decoded.(statedb.StateTransition)
		if !ok {
			fmt.Println("Failed to type assert to StateTransition")
			http.Error(w, "Failed to type assert to StateTransition", http.StatusInternalServerError)
			return
		}
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fuzzRes)
	}
}

func runRPCServer(sdbStorage *storage.StateDBStorage) {
	http.HandleFunc("/fuzz", handleFuzz(sdbStorage))
	http.HandleFunc("/validate", handleSTFValidation(sdbStorage))
	http.HandleFunc("/challenge", handleStateTransitionChallenge(sdbStorage))
	log.Println("RPC server listening on :8088")
	log.Fatal(http.ListenAndServe(":8088", nil))
}
