package fuzz

import (
	"math"

	"github.com/colorfulnotion/jam/types"
)

type FuzzStats struct {
	TotalBlocks            int `json:"generated"`
	FuzzedBlocks           int `json:"fuzzed"`
	OriginalBlocks         int `json:"original"`
	FuzzTruePositives      int `json:"fuzz_true_positives"`     // Fuzzed blocks correctly detected.
	FuzzFalseNegatives     int `json:"fuzz_false_negatives"`    // Fuzzed blocks that were missed.
	FuzzResponseErrors     int `json:"fuzz_response_errors"`    // Response errors in fuzzed blocks.
	FuzzMisclassifications int `json:"fuzz_misclassifications"` // Fuzzed blocks misclassified.
	OrigFalsePositives     int `json:"orig_false_positives"`    // Original blocks wrongly flagged.
	OrigTrueNegatives      int `json:"orig_true_negatives"`     // Original blocks correctly validated.
	OrigResponseErrors     int `json:"orig_response_errors"`    // Response errors in original blocks.
	OrigMisclassifications int `json:"orig_misclassifications"` // Original blocks misclassified.
}

func Ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0.0
	}
	ratio := float64(numerator) / float64(denominator)
	return math.Round(ratio*1000) / 1000
}

func (fs *FuzzStats) Metrics() map[string]interface{} {
	if fs.TotalBlocks == 0 {
		return nil
	}
	basic := map[string]interface{}{
		"TotalBlocks":    fs.TotalBlocks,
		"FuzzedBlocks":   fs.FuzzedBlocks,
		"OriginalBlocks": fs.OriginalBlocks,
		"FuzzedRate":     Ratio(fs.FuzzedBlocks, fs.TotalBlocks),
	}

	fuzz := map[string]interface{}{
		"Fuzzed_Blocks_Correctly_Detected": Ratio(fs.FuzzTruePositives, fs.FuzzedBlocks),
		"Fuzzed_Blocks_Undetected":         Ratio(fs.FuzzFalseNegatives, fs.FuzzedBlocks),
		"Fuzzed_Blocks_Response_Error":     Ratio(fs.FuzzResponseErrors, fs.FuzzedBlocks),
		"Fuzzed_Blocks_Misclassified":      Ratio(fs.FuzzMisclassifications, fs.FuzzedBlocks),
	}

	original := map[string]interface{}{
		"Original_Blocks_Falsely_Flagged":     Ratio(fs.OrigFalsePositives, fs.OriginalBlocks),
		"Original_Blocks_Correctly_Validated": Ratio(fs.OrigTrueNegatives, fs.OriginalBlocks),
		"Original_ResponseError":              Ratio(fs.OrigResponseErrors, fs.OriginalBlocks),
		"Original_Misclassified":              Ratio(fs.OrigMisclassifications, fs.OriginalBlocks),
	}

	overall := map[string]interface{}{
		//"FlaggedRate":           Ratio(fs.FuzzTruePositives+fs.OrigFalsePositives, fs.TotalBlocks),
		"CorrectRate":           Ratio(fs.FuzzTruePositives+fs.OrigTrueNegatives, fs.TotalBlocks),
		"ResponseErrorRate":     Ratio(fs.FuzzResponseErrors+fs.OrigResponseErrors, fs.TotalBlocks),
		"MisclassificationRate": Ratio(fs.FuzzMisclassifications+fs.OrigMisclassifications, fs.TotalBlocks),
	}

	return map[string]interface{}{
		"basic":    basic,
		"fuzz":     fuzz,
		"original": original,
		"overall":  overall,
	}
}

func (fs *FuzzStats) DumpMetrics() string {
	metrics := fs.Metrics()
	return types.ToJSON(metrics)
}
