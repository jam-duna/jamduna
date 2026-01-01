package verkle

import (
	"fmt"

	verkle "github.com/ethereum/go-verkle"
)

// marshalCompactProof serializes a go-verkle VerkleProof into the compact binary
// layout expected by the Rust verifier:
//   - 1 byte: other_stems length
//   - N * 32 bytes: other_stems
//   - 1 byte: depth_extension_present length
//   - depth_extension_present bytes
//   - 1 byte: commitments_by_path length
//   - M * 32 bytes: commitments_by_path
//   - 32 bytes: D
//   - 1 byte: ipa_proof length
//   - L * 32 bytes: CL
//   - L * 32 bytes: CR
//   - 32 bytes: final_evaluation (little-endian)
func marshalCompactProof(vp *verkle.VerkleProof) ([]byte, error) {
	if vp == nil || vp.IPAProof == nil {
		return nil, fmt.Errorf("compact proof: missing proof data")
	}

	if len(vp.OtherStems) > 255 {
		return nil, fmt.Errorf("compact proof: other_stems length %d exceeds 255", len(vp.OtherStems))
	}
	if len(vp.DepthExtensionPresent) > 255 {
		return nil, fmt.Errorf("compact proof: depth_extension_present length %d exceeds 255", len(vp.DepthExtensionPresent))
	}
	if len(vp.CommitmentsByPath) > 255 {
		return nil, fmt.Errorf("compact proof: commitments_by_path length %d exceeds 255", len(vp.CommitmentsByPath))
	}
	if len(vp.IPAProof.CL) != len(vp.IPAProof.CR) {
		return nil, fmt.Errorf("compact proof: ipa_proof CL/CR length mismatch")
	}
	if len(vp.IPAProof.CL) > 255 {
		return nil, fmt.Errorf("compact proof: ipa_proof length %d exceeds 255", len(vp.IPAProof.CL))
	}

	// Estimate capacity: counts + data + D + final evaluation
	size := 1 + len(vp.OtherStems)*32 +
		1 + len(vp.DepthExtensionPresent) +
		1 + len(vp.CommitmentsByPath)*32 +
		32 +
		1 + len(vp.IPAProof.CL)*64 +
		32

	buf := make([]byte, 0, size)

	buf = append(buf, byte(len(vp.OtherStems)))
	for _, stem := range vp.OtherStems {
		buf = append(buf, stem[:]...)
	}

	buf = append(buf, byte(len(vp.DepthExtensionPresent)))
	buf = append(buf, vp.DepthExtensionPresent...)

	buf = append(buf, byte(len(vp.CommitmentsByPath)))
	for _, c := range vp.CommitmentsByPath {
		buf = append(buf, c[:]...)
	}

	buf = append(buf, vp.D[:]...)

	ipaLen := len(vp.IPAProof.CL)
	buf = append(buf, byte(ipaLen))
	for i := 0; i < ipaLen; i++ {
		buf = append(buf, vp.IPAProof.CL[i][:]...)
	}
	for i := 0; i < ipaLen; i++ {
		buf = append(buf, vp.IPAProof.CR[i][:]...)
	}

	// Binary wire format uses little-endian encoding for finalEvaluation.
	var finalEvalLE [32]byte
	copy(finalEvalLE[:], vp.IPAProof.FinalEvaluation[:])
	for i := 0; i < 16; i++ {
		finalEvalLE[i], finalEvalLE[31-i] = finalEvalLE[31-i], finalEvalLE[i]
	}
	buf = append(buf, finalEvalLE[:]...)

	return buf, nil
}

// unmarshalCompactProof parses the compact binary layout back into a go-verkle
// VerkleProof. The function performs bounds checking and converts the
// little-endian finalEvaluation back to big-endian representation expected by
// go-verkle.
func unmarshalCompactProof(data []byte) (*verkle.VerkleProof, error) {
	cursor := 0

	readByte := func() (byte, error) {
		if cursor >= len(data) {
			return 0, fmt.Errorf("compact proof: unexpected end of data")
		}
		b := data[cursor]
		cursor++
		return b, nil
	}

	readBlock := func(length int) ([]byte, error) {
		if cursor+length > len(data) {
			return nil, fmt.Errorf("compact proof: unexpected end of data")
		}
		block := data[cursor : cursor+length]
		cursor += length
		return block, nil
	}

	readArray32 := func() ([32]byte, error) {
		block, err := readBlock(32)
		if err != nil {
			return [32]byte{}, err
		}
		var out [32]byte
		copy(out[:], block)
		return out, nil
	}

	otherCount, err := readByte()
	if err != nil {
		return nil, err
	}
	otherStems := make([][31]byte, otherCount)
	for i := 0; i < int(otherCount); i++ {
		stem, err := readArray32()
		if err != nil {
			return nil, err
		}
		// Stems are 31 bytes - take first 31 bytes
		otherStems[i] = [31]byte(stem[:31])
	}

	depthLen, err := readByte()
	if err != nil {
		return nil, err
	}
	depthExt, err := readBlock(int(depthLen))
	if err != nil {
		return nil, err
	}

	commitmentLen, err := readByte()
	if err != nil {
		return nil, err
	}
	commitments := make([][32]byte, commitmentLen)
	for i := 0; i < int(commitmentLen); i++ {
		c, err := readArray32()
		if err != nil {
			return nil, err
		}
		commitments[i] = c
	}

	dBytes, err := readArray32()
	if err != nil {
		return nil, err
	}

	ipaLen, err := readByte()
	if err != nil {
		return nil, err
	}
	if ipaLen != 8 {
		return nil, fmt.Errorf("compact proof: expected 8 IPA rounds, got %d", ipaLen)
	}
	var cl [8][32]byte
	for i := 0; i < 8; i++ {
		point, err := readArray32()
		if err != nil {
			return nil, err
		}
		cl[i] = point
	}
	var cr [8][32]byte
	for i := 0; i < 8; i++ {
		point, err := readArray32()
		if err != nil {
			return nil, err
		}
		cr[i] = point
	}

	finalEvalLE, err := readArray32()
	if err != nil {
		return nil, err
	}
	for i := 0; i < 16; i++ {
		finalEvalLE[i], finalEvalLE[31-i] = finalEvalLE[31-i], finalEvalLE[i]
	}

	if cursor != len(data) {
		return nil, fmt.Errorf("compact proof: unexpected trailing %d bytes", len(data)-cursor)
	}

	return &verkle.VerkleProof{
		OtherStems:            otherStems,
		DepthExtensionPresent: depthExt,
		CommitmentsByPath:     commitments,
		D:                     dBytes,
		IPAProof: &verkle.IPAProof{
			CL:              cl,
			CR:              cr,
			FinalEvaluation: finalEvalLE,
		},
	}, nil
}
