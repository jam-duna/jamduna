package types

import (
	"crypto/ed25519"
	"fmt"
	"reflect"
	"sync"

	"github.com/colorfulnotion/jam/common"
)

type Judgement struct {
	Judge          bool             `json:"judge"`
	WorkReportHash common.Hash      `json:"work_report"`
	Validator      uint16           `json:"validator"`
	Signature      Ed25519Signature `json:"signature"`
}

func (j Judgement) DeepCopy() (Judgement, error) {
	var copiedJudgement Judgement

	// Serialize the original Judgement to JSON
	data, err := Encode(j)
	if err != nil {
		return copiedJudgement, err
	}

	// Deserialize the JSON back into a new Judgement instance
	decoded, _, err := Decode(data, reflect.TypeOf(Judgement{}))
	if err != nil {
		return copiedJudgement, err
	}
	copiedJudgement = decoded.(Judgement)

	return copiedJudgement, nil
}

func (j *Judgement) UnsignedBytesWithSalt() []byte {
	var signtext []byte
	if j.Judge {
		signtext = append([]byte(X_True), j.WorkReportHash.Bytes()...)
	} else {
		signtext = append([]byte(X_False), j.WorkReportHash.Bytes()...)
	}
	return signtext
}

func (j *Judgement) Sign(Ed25519Secret []byte) {
	signtext := j.UnsignedBytesWithSalt()

	j.Signature = Ed25519Signature(ed25519.Sign(Ed25519Secret, signtext))
}

func (j *Judgement) Verify(key Ed25519Key) error {
	signtext := j.UnsignedBytesWithSalt()

	if !ed25519.Verify(key.PublicKey(), signtext, j.Signature.Bytes()) {
		return fmt.Errorf("invalid signature by validator %v", j.Validator)
	}
	return nil
}

// func (j *Judgement) Bytes() []byte {
// 	enc := Encode(j)
// 	return enc
// }

// func (j *Judgement) Hash() common.Hash {
// 	data := j.Bytes()
// 	if data == nil {
// 		return common.Hash{}

// 	}
// 	return common.Blake2Hash(data)
// }

type JudgeBucket struct {
	sync.RWMutex
	Judgements      map[common.Hash][]Judgement `json:"judgements"`
	KnownJudgements map[common.Hash]bool        // use identifier to filter duplicate J
}

func (J *JudgeBucket) PutJudgement(j Judgement) {
	J.Lock()
	defer J.Unlock()
	//consider adding a separate judgementHash to avoid unnecessary duplication check
	if J.Judgements == nil {
		J.Judgements = make(map[common.Hash][]Judgement)
	}
	if J.KnownJudgements == nil {
		J.KnownJudgements = make(map[common.Hash]bool)
	}
	workReportHash := j.WorkReportHash
	coreJudgements, exists := J.Judgements[workReportHash]
	if !exists {
		coreJudgements = make([]Judgement, 0)
	}
	// if judgement already exists within workPackageHash, exit
	if J.KnownJudgements[j.Hash()] {
		return
	}
	for _, jj := range coreJudgements {
		if jj.Validator == j.Validator {
			return
		}
	}
	J.KnownJudgements[j.Hash()] = true
	coreJudgements = append(coreJudgements, j)
	J.Judgements[workReportHash] = coreJudgements
}

func (J *JudgeBucket) GetLen(w common.Hash) int {
	J.RLock()
	defer J.RUnlock()
	return len(J.Judgements[w])
}

func (J *JudgeBucket) HaveMadeJudgement(j Judgement) bool {
	J.RLock()
	defer J.RUnlock()
	if J.KnownJudgements == nil {
		return false
	}
	return J.KnownJudgements[j.Hash()]
}

func (J *JudgeBucket) HaveMadeJudgementByValidator(workreporthash common.Hash, validator uint16) bool {
	J.RLock()
	defer J.RUnlock()
	for _, j := range J.Judgements[workreporthash] {
		if j.Validator == validator {
			return true
		}
	}
	return false
}

func (J *JudgeBucket) GetJudgementByValidator(workreporthash common.Hash, validator uint16) (Judgement, error) {
	J.RLock()
	defer J.RUnlock()
	for _, j := range J.Judgements[workreporthash] {
		if j.Validator == validator {
			return j, nil
		}
	}
	return Judgement{}, fmt.Errorf("validator %v has not made judgement on work report %v", validator, workreporthash)
}

// func (J *JudgeBucket) GetJudgement(w common.Hash, core uint16) (Judgement, bool) {
// 	for _, j := range J.Judgements[w] {
// 		if j.Core == core {
// 			return j, true
// 		}
// 	}
// 	return Judgement{}, false
// }

func (J *JudgeBucket) GetTrueCount(W common.Hash) int {
	J.RLock()
	defer J.RUnlock()
	count := 0
	for _, j := range J.Judgements[W] {
		if j.Judge {
			count++
		}
	}
	return count
}

func (J *JudgeBucket) GetFalseCount(W common.Hash) int {
	J.RLock()
	defer J.RUnlock()
	count := 0
	for _, j := range J.Judgements[W] {
		if !j.Judge {
			count++
		}
	}
	return count
}

func (J *JudgeBucket) GetTrueJudgement(W common.Hash) []Judgement {
	J.RLock()
	defer J.RUnlock()
	judgements := make([]Judgement, 0)
	for _, j := range J.Judgements[W] {
		if j.Judge {
			judgements = append(judgements, j)
		}
	}
	return judgements[:ValidatorsSuperMajority]
}

func (J *JudgeBucket) GetFalseJudgement(W common.Hash) []Judgement {
	J.RLock()
	defer J.RUnlock()
	judgements := make([]Judgement, 0)
	for _, j := range J.Judgements[W] {
		if !j.Judge {
			judgements = append(judgements, j)
		}
	}
	if len(judgements) < WonkyFalseThreshold {
		return judgements
	}
	return judgements[:ValidatorsSuperMajority]
}

func (J *JudgeBucket) GetWonkeyJudgement(W common.Hash) []Judgement {
	J.RLock()
	defer J.RUnlock()
	judgements := make([]Judgement, 0)
	trueCount := 0
	falseCount := 0
	for _, j := range J.Judgements[W] {
		if !j.Judge {
			if falseCount >= WonkyFalseThreshold {
				continue
			}
			judgements = append(judgements, j)
			falseCount++
		} else {
			if trueCount >= WonkyTrueThreshold {
				continue
			}
			judgements = append(judgements, j)
			trueCount++
		}
	}

	return judgements[:ValidatorsSuperMajority]
}

func (j *Judgement) Bytes() []byte {
	enc, err := Encode(j)
	if err != nil {
		return nil
	}
	return enc
}

func (j *Judgement) Hash() common.Hash {
	data := j.Bytes()
	return common.Blake2Hash(data)
}
