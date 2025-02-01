package fuzz

import (
	"encoding/json"
	"log"
	"math"
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
		"TruePositiveRate":      Ratio(fs.FuzzTruePositives, fs.FuzzedBlocks),
		"FalseNegativeRate":     Ratio(fs.FuzzFalseNegatives, fs.FuzzedBlocks),
		"ResponseErrorRate":     Ratio(fs.FuzzResponseErrors, fs.FuzzedBlocks),
		"MisclassificationRate": Ratio(fs.FuzzMisclassifications, fs.FuzzedBlocks),
	}

	original := map[string]interface{}{
		"FalsePositiveRate":     Ratio(fs.OrigFalsePositives, fs.OriginalBlocks),
		"TrueNegativeRate":      Ratio(fs.OrigTrueNegatives, fs.OriginalBlocks),
		"ResponseErrorRate":     Ratio(fs.OrigResponseErrors, fs.OriginalBlocks),
		"MisclassificationRate": Ratio(fs.OrigMisclassifications, fs.OriginalBlocks),
	}

	overall := map[string]interface{}{
		"FlaggedRate":           Ratio(fs.FuzzTruePositives+fs.OrigFalsePositives, fs.TotalBlocks),
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
	jsonBytes, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		log.Fatalf("Error marshalling metrics to JSON: %v", err)
	}
	return string(jsonBytes)
}
