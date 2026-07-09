// Copyright 2024-2026 Nexus Protocol Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ledger

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// QueryReport is the result of `nexus ledger query <field>`.
type QueryReport struct {
	// Field is the queried field name (e.g., "gpu", "kernel").
	Field string `json:"field"`
	// Value is the queried value (e.g., "NVIDIA GeForce RTX 4090").
	Value string `json:"value"`
	// Matches is the total number of records matching this value.
	Matches int `json:"matches"`
	// SuccessRate is the fraction of matching records that succeeded (0.0-1.0).
	SuccessRate float64 `json:"success_rate"`
	// TotalRecords is the total number of records in the ledger.
	TotalRecords int `json:"total_records"`
}

// CheckReport is the result of `nexus ledger check`.
type CheckReport struct {
	// HardwareOK is true when the current hardware has a >=50% success rate
	// in the ledger. False when the hardware is unknown or has a poor rate.
	HardwareOK bool `json:"hardware_ok"`
	// SuccessRate is the overall success rate for this hardware config.
	SuccessRate float64 `json:"success_rate"`
	// TotalRecords is the number of matching records in the ledger.
	TotalRecords int `json:"total_records"`
	// Unknown is true when there are zero matching records.
	Unknown bool `json:"unknown"`
	// Warnings lists any non-fatal diagnostics.
	Warnings []string `json:"warnings,omitempty"`
}

// QueryField searches the ledger for records matching a specific field value.
// Supported fields: "gpu", "kernel", "os", "arch", "cpu".
// Returns a QueryReport with match count and success rate.
func QueryField(ctx context.Context, state *engine.StateTracker, field, value string) (*QueryReport, error) {
	if state == nil {
		return nil, fmt.Errorf("ledger: state tracker is nil")
	}
	if field == "" || value == "" {
		return nil, fmt.Errorf("field and value are required")
	}

	switch strings.ToLower(field) {
	case "gpu", "kernel", "os", "arch", "cpu":
	default:
		return nil, fmt.Errorf("unsupported field %q (supported: gpu, kernel, os, arch, cpu)", field)
	}

	ledger := state.GetLedger()
	total := len(ledger.Records)
	if total == 0 {
		return &QueryReport{Field: field, Value: value, TotalRecords: 0}, nil
	}

	fieldLower := strings.ToLower(field)
	valueLower := strings.ToLower(value)
	numWorkers := runtime.NumCPU()
	if numWorkers < 2 {
		numWorkers = 2
	}
	if numWorkers > total {
		numWorkers = total
	}

	type chunkResult struct {
		matches   int
		successes int
	}

	chunkSize := (total + numWorkers - 1) / numWorkers
	resultCh := make(chan chunkResult, numWorkers)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > total {
			end = total
		}
		if start >= end {
			break
		}

		wg.Add(1)
		go func(records []engine.HardwareReport) {
			defer wg.Done()
			cm := 0
			cs := 0
			for _, r := range records {
				var fieldValue string
				switch fieldLower {
				case "gpu":
					fieldValue = r.GPU
				case "kernel":
					fieldValue = r.Kernel
				case "os":
					fieldValue = r.OS
				case "arch":
					fieldValue = r.Arch
				case "cpu":
					fieldValue = r.CPUModel
				}

				if strings.Contains(strings.ToLower(fieldValue), valueLower) {
					cm++
					if r.Success {
						cs++
					}
				}
			}
			resultCh <- chunkResult{matches: cm, successes: cs}
		}(ledger.Records[start:end])
	}

	wg.Wait()
	close(resultCh)

	totalMatches := 0
	totalSuccesses := 0
	for res := range resultCh {
		totalMatches += res.matches
		totalSuccesses += res.successes
	}

	rate := 0.0
	if totalMatches > 0 {
		rate = float64(totalSuccesses) / float64(totalMatches)
	}

	return &QueryReport{
		Field:        field,
		Value:        value,
		Matches:      totalMatches,
		SuccessRate:  rate,
		TotalRecords: total,
	}, nil
}

// CheckHardware checks whether the current hardware configuration has a
// good track record in the ledger. Returns a CheckReport with the verdict.
func CheckHardware(ctx context.Context, state *engine.StateTracker) (*CheckReport, error) {
	if state == nil {
		return nil, fmt.Errorf("ledger: state tracker is nil")
	}

	info, err := engine.Probe(ctx)
	if err != nil {
		// Probe warnings are non-fatal — proceed with partial data
	}

	fp := GenerateFingerprint(info)
	ledger := state.GetLedger()

	matching := 0
	successes := 0
	for _, r := range ledger.Records {
		if r.DeviceFingerprint == fp {
			matching++
			if r.Success {
				successes++
			}
		}
	}

	report := &CheckReport{
		TotalRecords: matching,
	}

	if matching == 0 {
		report.Unknown = true
		report.Warnings = []string{"no records found for this hardware configuration — run 'nexus ledger record'"}
		return report, nil
	}

	report.SuccessRate = float64(successes) / float64(matching)
	report.HardwareOK = report.SuccessRate >= 0.5

	if !report.HardwareOK {
		report.Warnings = []string{
			fmt.Sprintf("this hardware has a %.0f%% failure rate in the ledger", (1-report.SuccessRate)*100),
		}
	}

	return report, nil
}
