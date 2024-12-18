package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func encodeapi(objectType string, inp string) (string, error) {
	// Convert input hex string into bytes
	var err error
	var obj interface{}
	fmt.Printf("objectType:%s\ninput:%s\n", objectType, inp)
	input := []byte(inp)
	// Decode the input string into the appropriate Go struct based on objectType
	switch objectType {
	case "Block":
		var block types.Block
		err = json.Unmarshal(input, &block)
		if err != nil {
			return "", err
		}
		obj = block
	case "Ticket":
		var ticket types.Ticket
		err = json.Unmarshal(input, &ticket)
		if err != nil {
			return "", err
		}
		obj = ticket
	case "Guarantee":
		var guarantee types.Guarantee
		err = json.Unmarshal(input, &guarantee)
		if err != nil {
			return "", err
		}
		obj = guarantee
	case "Assurance":
		var assurance types.Assurance
		err = json.Unmarshal(input, &assurance)
		if err != nil {
			return "", err
		}
		obj = assurance
	case "Preimages":
		var preimages types.Preimages
		err = json.Unmarshal(input, &preimages)
		if err != nil {
			return "", err
		}
		obj = preimages
	case "WorkPackage":
		var workPackage types.WorkPackage
		err = json.Unmarshal(input, &workPackage)
		if err != nil {
			return "", err
		}
		obj = workPackage
	case "WorkItem":
		var workItem types.WorkItem
		err = json.Unmarshal(input, &workItem)
		if err != nil {
			return "", err
		}
		obj = workItem
	case "WorkReport":
		var workReport types.WorkReport
		err = json.Unmarshal(input, &workReport)
		if err != nil {
			return "", err
		}
		obj = workReport
	case "WorkResult":
		var workResult types.WorkResult
		err = json.Unmarshal(input, &workResult)
		if err != nil {
			return "", err
		}
		obj = workResult
	case "Announcement":
		var announcement types.Announcement
		err = json.Unmarshal(input, &announcement)
		if err != nil {
			return "", err
		}
		obj = announcement
	case "Judgement":
		var judgement types.Judgement
		err = json.Unmarshal(input, &judgement)
		if err != nil {
			return "", err
		}
		obj = judgement
	case "C1":
		var c1 [types.TotalCores][]common.Hash
		err = json.Unmarshal(input, &c1)
		if err != nil {
			return "", err
		}
		obj = c1
	case "C2":
		var c2 statedb.AuthorizationQueue
		err = json.Unmarshal(input, &c2)
		if err != nil {
			return "", err
		}
		obj = c2
	case "C3":
		var c3 statedb.RecentBlocks
		err = json.Unmarshal(input, &c3)
		if err != nil {
			return "", err
		}
		obj = c3
	case "C4":
		var c4 statedb.SafroleBasicState
		err = json.Unmarshal(input, &c4)
		if err != nil {
			return "", err
		}
		obj = c4
	case "C5":
		var c5 statedb.Psi_state
		err = json.Unmarshal(input, &c5)
		if err != nil {
			return "", err
		}
		obj = c5
	case "C6":
		var c6 statedb.Entropy
		err = json.Unmarshal(input, &c6)
		if err != nil {
			return "", err
		}
		obj = c6
	/*case "C7", "C8", "C9":
	var validators statedb.Validators
	err = json.Unmarshal(input, &validators)
	if err != nil {
		return "", err
	}
	obj = validators*/
	case "C10":
		var availabilityAssignments statedb.AvailabilityAssignments
		err = json.Unmarshal(input, &availabilityAssignments)
		if err != nil {
			return "", err
		}
		obj = availabilityAssignments
	case "C11":
		var c11 uint32
		err = json.Unmarshal(input, &c11)
		if err != nil {
			return "", err
		}
		obj = c11
	case "C12":
		var kaiState statedb.Kai_state
		err = json.Unmarshal(input, &kaiState)
		if err != nil {
			return "", err
		}
		obj = kaiState
	case "C13":
		var c13 [2][types.TotalValidators]statedb.Pi_state
		err = json.Unmarshal(input, &c13)
		if err != nil {
			return "", err
		}
		obj = c13
	case "C14":
		var c14 [types.EpochLength][]types.AccumulationQueue
		err = json.Unmarshal(input, &c14)
		if err != nil {
			return "", err
		}
		obj = c14
	case "C15":
		var c15 [types.EpochLength]types.AccumulationHistory
		err = json.Unmarshal(input, &c15)
		if err != nil {
			return "", err
		}
		obj = c15
	case "JamState":
		var jamstate statedb.StateSnapshot
		err = json.Unmarshal(input, &jamstate)
		if err != nil {
			return "", err
		}
		obj = jamstate
	default:
		return "", errors.New("Unknown object type")
	}

	// Encode the unmarshaled Go object into bytes
	encodedBytes, err := types.Encode(obj)
	if err != nil {
		return "", err
	}

	// Return the encoded bytes as a hex string
	return common.Bytes2Hex(encodedBytes), nil
}

func decodeapi(objectType, input string) (string, error) {
	// Convert input hex string into bytes
	encodedBytes := common.Hex2Bytes(input)
	if len(encodedBytes) == 0 {
		return "", errors.New("Invalid hex input")
	}

	var err error
	//var length uint32
	var decodedStruct interface{}
	// Switch on objectType to handle different cases and decode accordingly

	switch objectType {
	case "Block":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Block{}))
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
	case "WorkResult":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkResult{}))
	case "WorkReport":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkReport{}))
	case "WorkItem":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.WorkItem{}))
	case "C1":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.TotalCores][]common.Hash{}))
	case "C2":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.AuthorizationQueue{}))
	case "C3":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.RecentBlocks{}))
	case "C4":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.SafroleBasicState{}))
	case "C5":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Psi_state{}))
	case "C6":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Entropy{}))
	/*case "C7":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Validators{}))
	case "C8":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Validators{}))
	case "C9":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Validators{}))*/
	case "C10":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.AvailabilityAssignments{}))
	case "C11":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(uint32(0)))
	case "C12":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.Kai_state{}))
	case "C13":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([2][types.TotalValidators]statedb.Pi_state{}))
	case "JamState":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(statedb.StateSnapshot{}))
	default:
		return "", errors.New("Unknown object type")
	}

	if err != nil {
		return "", err
	}

	// Convert decoded structure to JSON string
	decodedJSON, err := json.MarshalIndent(decodedStruct, "", "    ")
	if err != nil {
		return "", err
	}

	return string(decodedJSON), nil
}

func main() {
	port := 8080
	mux := http.NewServeMux()

	// Serve static files from the "./static" directory
	fileServer := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fp := path.Join(".", "codec.html")
		tmpl, err := template.ParseFiles(fp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
	})

	// Handle encoding requests
	mux.HandleFunc("/encode", func(w http.ResponseWriter, r *http.Request) {
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
			fmt.Printf("encodeapi %v\n", err)
			http.Error(w, fmt.Sprintf("Error during encoding: %v", err), http.StatusInternalServerError)
		}
		json.NewEncoder(w).Encode(map[string]string{"result": result})
	})

	// Handle decoding requests
	mux.HandleFunc("/decode", func(w http.ResponseWriter, r *http.Request) {
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
			fmt.Printf("encodeapi %v\n", err)
			http.Error(w, fmt.Sprintf("Error during encoding: %v", err), http.StatusInternalServerError)
		}
		json.NewEncoder(w).Encode(map[string]string{"result": result})
	})

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	fmt.Printf("Starting server on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))

}
