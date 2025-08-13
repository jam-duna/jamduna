package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

func encodeapi(objectType string, inp string) (string, error) {
	var err error
	var obj interface{}

	fmt.Printf("encodeapi: objectType=%s\ninput=%s\n", objectType, inp)
	input := []byte(inp)

	// Unmarshal JSON → Go struct → Encode (hex)
	switch objectType {
	case "Block":
		var block types.Block
		err = json.Unmarshal(input, &block)
		obj = block
	case "Header":
		var header types.BlockHeader
		err = json.Unmarshal(input, &header)
		obj = header
	case "Extrinsic":
		var extrinsic types.ExtrinsicData
		err = json.Unmarshal(input, &extrinsic)
		obj = extrinsic
	case "Ticket":
		var ticket types.Ticket
		err = json.Unmarshal(input, &ticket)
		obj = ticket
	case "Guarantee":
		var guarantee types.Guarantee
		err = json.Unmarshal(input, &guarantee)
		obj = guarantee
	case "Assurance":
		var assurance types.Assurance
		err = json.Unmarshal(input, &assurance)
		obj = assurance
	case "Preimages":
		var preimages types.Preimages
		err = json.Unmarshal(input, &preimages)
		obj = preimages
	case "WorkPackage":
		var workPackage types.WorkPackage
		err = json.Unmarshal(input, &workPackage)
		obj = workPackage
	case "WorkPackageBundle":
		var workPackageBundle types.WorkPackageBundle
		err = json.Unmarshal(input, &workPackageBundle)
		encode := workPackageBundle.Encode()
		fmt.Printf("!!!%v encode(len:%v)=%x\n", objectType, len(encode), encode)
		obj = workPackageBundle
	case "WorkItem":
		var workItem types.WorkItem
		err = json.Unmarshal(input, &workItem)
		obj = workItem
	case "WorkReport":
		var workReport types.WorkReport
		err = json.Unmarshal(input, &workReport)
		obj = workReport
	case "PageProof":
		var pageProof types.PageProof
		err = json.Unmarshal(input, &pageProof)
		obj = pageProof
	case "WorkResult":
		var workResult types.WorkResult
		err = json.Unmarshal(input, &workResult)
		obj = workResult
	case "Announcement":
		var announcement types.Announcement
		err = json.Unmarshal(input, &announcement)
		obj = announcement
	case "Judgement":
		var judgement types.Judgement
		err = json.Unmarshal(input, &judgement)
		obj = judgement
	case "C1":
		var c1 [types.TotalCores][]common.Hash
		err = json.Unmarshal(input, &c1)
		obj = c1
	case "C2":
		var c2 types.AuthorizationQueue
		err = json.Unmarshal(input, &c2)
		obj = c2
	case "C3":
		var c3 statedb.RecentBlocks
		err = json.Unmarshal(input, &c3)
		obj = c3
	case "C3-Beta":
		var beta_state statedb.Beta_state
		err = json.Unmarshal(input, &beta_state)
		obj = beta_state
	case "C4":
		var c4 statedb.SafroleBasicState
		err = json.Unmarshal(input, &c4)
		obj = c4
	case "C4-Gamma_s":
		var c4gammas statedb.TicketsOrKeys
		err = json.Unmarshal(input, &c4gammas)
		obj = c4gammas
	case "C5":
		var c5 statedb.Psi_state
		err = json.Unmarshal(input, &c5)
		obj = c5
	case "C6":
		var c6 statedb.Entropy
		err = json.Unmarshal(input, &c6)
		obj = c6
	case "C7", "C8", "C9":
		var validators types.Validators
		err = json.Unmarshal(input, &validators)
		obj = validators
	case "C10":
		var availabilityAssignments statedb.AvailabilityAssignments
		err = json.Unmarshal(input, &availabilityAssignments)
		obj = availabilityAssignments
	case "C11":
		var c11 uint32
		err = json.Unmarshal(input, &c11)
		obj = c11
	case "C12":
		var kaiState types.Kai_state
		err = json.Unmarshal(input, &kaiState)
		obj = kaiState
	case "C13":
		var c13 types.ValidatorStatistics
		err = json.Unmarshal(input, &c13)
		obj = c13
	case "C14":
		var c14 [types.EpochLength][]types.AccumulationQueue
		err = json.Unmarshal(input, &c14)
		obj = c14
	case "C15":
		var c15 [types.EpochLength]types.AccumulationHistory
		err = json.Unmarshal(input, &c15)
		obj = c15
	case "C16":
		var c16 []types.AccumulationOutput
		err = json.Unmarshal(input, &c16)
		obj = c16
	case "JamState":
		var jamstate statedb.StateSnapshot
		err = json.Unmarshal(input, &jamstate)
		obj = jamstate
	case "STF":
		var stf statedb.StateTransition
		err = json.Unmarshal(input, &stf)
		obj = stf
	case "SC":
		var sc statedb.StateTransitionChallenge
		err = json.Unmarshal(input, &sc)
		obj = sc
	case "ServiceAccount":
		// Special case
		var serviceAccount types.ServiceAccount
		err = json.Unmarshal(input, &serviceAccount)
		if err != nil {
			return "", err
		}
		encodedBytes, err := serviceAccount.Bytes()
		if err != nil {
			return "", err
		}
		return common.Bytes2Hex(encodedBytes), nil

	default:
		return "", errors.New("Unknown object type")
	}

	if err != nil {
		return "", err
	}

	// Encode the unmarshaled Go object into bytes
	encodedBytes, err := types.Encode(obj)
	if err != nil {
		return "", err
	}

	// Return hex string
	return common.Bytes2Hex(encodedBytes), nil
}

func decodeapi(objectType, input string) (string, error) {
	// Convert input hex → bytes
	encodedBytes := common.Hex2Bytes(input)
	if len(encodedBytes) == 0 {
		return "", errors.New("Invalid hex input")
	}

	var err error
	var decodedStruct interface{}

	switch objectType {
	case "Block":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Block{}))
	case "Header":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.BlockHeader{}))
	case "Extrinsic":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.ExtrinsicData{}))
	case "Ticket":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Ticket{}))
	case "Guarantee":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Guarantee{}))
	case "Assurance":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Assurance{}))
	case "Preimages":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Preimages{}))
	case "Announcement":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Announcement{}))
	case "Judgement":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Judgement{}))
	case "WorkPackage":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkPackage{}))
	case "WorkPackageBundle":
		//decodedStruct, _, err = types.DecodeBundle(encodedBytes)
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkPackageBundle{}))
	case "WorkResult":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkResult{}))
	case "WorkReport":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkReport{}))
	case "PageProof":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.PageProof{}))
	case "WorkItem":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkItem{}))
	case "C1":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.TotalCores][]common.Hash{}))
	case "C2":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.AuthorizationQueue{}))
	case "C3":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.RecentBlocks{}))
	case "C3-Beta":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Beta_state{}))
	case "C4":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.SafroleBasicState{}))
	case "C4-Gamma_s":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.TicketsOrKeys{}))
	case "C5":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Psi_state{}))
	case "C6":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Entropy{}))
	case "C7":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Validators{}))
	case "C8":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Validators{}))
	case "C9":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Validators{}))
	case "C10":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.AvailabilityAssignments{}))
	case "C11":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(uint32(0)))
	case "C12":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Kai_state{}))
	case "C13":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.ValidatorStatistics{}))
	case "C14":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.EpochLength][]types.AccumulationQueue{}))
	case "C15":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.EpochLength]types.AccumulationHistory{}))
	case "JamState":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.StateSnapshot{}))
	case "STF":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.StateTransition{}))
	case "SC":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.StateTransitionChallenge{}))
	case "ServiceAccount":
		decodedStruct, err = types.AccountStateFromBytes(0, encodedBytes)
	default:
		return "", errors.New("Unknown object type")
	}

	if err != nil {
		return "", err
	}

	// Convert decoded structure → JSON (indented)
	decodedJSON, err := json.MarshalIndent(decodedStruct, "", "    ")
	if err != nil {
		return "", err
	}

	return string(decodedJSON), nil
}

type Vrf_Response struct {
	VrfOutput common.Hash `json:"vrf_output"`
	ErrorMsg  string      `json:"errormsg"`
}

func bandersnatch_api(objectType string, input string) (string, error) {
	var err error
	var obj interface{}
	var output_json_string string
	input_byte := []byte(input)
	switch objectType {
	case "GetTicketVRF":
		var ticket types.Ticket
		err = json.Unmarshal(input_byte, &ticket)
		obj = ticket
		// transform ticket to types.Ticket
		ticket_type := obj.(types.Ticket)
		// get vrf_output
		output, err := ticket_type.TicketID()
		var Vrf_Response Vrf_Response
		Vrf_Response.VrfOutput = output
		if err != nil {
			Vrf_Response.ErrorMsg = err.Error()
		}
		output_json_byte, err := json.Marshal(Vrf_Response)
		if err != nil {
			Vrf_Response.ErrorMsg = err.Error()
		}
		output_json_string = string(output_json_byte)
	default:
		return "", errors.New("Unknown object type")
	}
	if err != nil {
		return "", err
	}
	return output_json_string, nil
}

// withCORS is a simple middleware that adds CORS headers.
func withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow any origin; adjust as needed
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// Handle preflight request
		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			return
		}
		h(w, r)
	}
}

func initStorage(testDir string) (*storage.StateDBStorage, error) {
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err = os.MkdirAll(testDir, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("Failed to create directory /tmp/fuzz: %v", err)
		}
	}

	sdb_storage, err := storage.NewStateDBStorage(testDir)
	if err != nil {
		return nil, fmt.Errorf("Error with storage: %v", err)
	}
	return sdb_storage, nil

}

func main() {
	port := 8999
	mux := http.NewServeMux()
	buildV := common.GetCommitHash()

	// Serve all static files under /static/
	fileServer := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	testDir := "/tmp/test_local"
	test_storage, err := initStorage(testDir)
	if err != nil {
		panic(err)
	}
	// Serve codec.html at the root path
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Parse and serve the template
		fp := path.Join(".", "codec.html")
		tmpl, err := template.ParseFiles(fp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Pass build version to the template
		tmpl.Execute(w, map[string]interface{}{"BuildVersion": buildV})
	})

	mux.HandleFunc("/api/stf", withCORS(func(w http.ResponseWriter, r *http.Request) {
		// Read the POST input into content
		content, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("ERR %v\n", err)
			http.Error(w, "unable to read request body", http.StatusBadRequest)
			return
		}

		var stf statedb.StateTransition
		err = json.Unmarshal(content, &stf)
		if err != nil {
			// Return a 400 error code for invalid JSON
			var errorsArr []map[string]interface{}
			errorsArr = append(errorsArr, map[string]interface{}{
				"details": err.Error(),
			})
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": errorsArr,
			})
			return
		}

		// Perform the state transition check.
		diffs, err := statedb.CheckStateTransitionWithOutput(test_storage, &stf, nil, pvm.BackendInterpreter, false)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			// Build an errors array from the diffs map using PoststateCompared
			var errorsArr []map[string]interface{}
			for key, diff := range diffs {
				errorsArr = append(errorsArr, map[string]interface{}{
					"title": fmt.Sprintf("%s", key),
					"left":  fmt.Sprintf("%x", diff.ActualPostState),
					"right": fmt.Sprintf("%x", diff.ExpectedPostState),
				})
			}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": errorsArr,
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"ok": nil,
			},
		})
	}))

	// Expose /api/encode endpoint with CORS middleware
	mux.HandleFunc("/api/encode", withCORS(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ObjectType string `json:"objectType"`
			InputText  string `json:"inputText"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		result, err := encodeapi(req.ObjectType, req.InputText)
		if err != nil {
			log.Printf("encodeapi error: %v\n", err)
			http.Error(w, fmt.Sprintf("Error during encoding: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"result": result})
	}))

	// Expose /api/decode endpoint with CORS middleware
	mux.HandleFunc("/api/decode", withCORS(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ObjectType string `json:"objectType"`
			InputText  string `json:"inputText"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		result, err := decodeapi(req.ObjectType, req.InputText)
		if err != nil {
			log.Printf("decodeapi error: %v\n", err)
			http.Error(w, fmt.Sprintf("Error during decoding: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"result": result})
	}))

	// Expose /api/bandersnatch endpoint with CORS middleware
	mux.HandleFunc("/api/bandersnatch", withCORS(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ObjectType string `json:"objectType"`
			InputText  string `json:"inputText"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		result, err := bandersnatch_api(req.ObjectType, req.InputText)
		if err != nil {
			log.Printf("bandersnatch_api error: %v\n", err)
			http.Error(w, fmt.Sprintf("Error during bandersnatch: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"result": result})
	}))

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	fmt.Printf("Starting Codec-Web server on %s. Build=%v\n", addr, buildV)
	log.Fatal(http.ListenAndServe(addr, mux))
}
