//go:build testing
// +build testing

package statedb

import "testing"

func TestTestVectors(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{"Safrole", TestSafroleVerify},
		{"ReportInvalid", TestReportVerify},
		{"ReportValid", TestReportValidReportCase},
		{"Assurance", TestVerifyAssurance},
		{"Disputes", TestVerifyDisputes},
		{"Statics", TestStaticsSTFVerify},
		{"Auth", TestVerifyAuths},
		{"Accumulate", TestAccumulateSTF},
		// {"Traces", TestTraces},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic in %s: %v", tc.name, r)
				}
			}()
			tc.test(t)
		})
	}
}
