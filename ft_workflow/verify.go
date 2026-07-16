package ft_workflow

import "fmt"

// VerificationStatus is the result of a verification check.
type VerificationStatus string

const (
	VerificationPass VerificationStatus = "PASS"
	VerificationFail VerificationStatus = "FAIL"
	VerificationWarn VerificationStatus = "WARN"
)

// VerificationCheck is a single check result.
type VerificationCheck struct {
	Name    string             `json:"name"`
	Status  VerificationStatus `json:"status"`
	Message string             `json:"message,omitempty"`
}

// VerificationResult aggregates all verification checks.
type VerificationResult struct {
	Status VerificationStatus  `json:"status"`
	Checks []VerificationCheck `json:"checks"`
	Passed bool                `json:"passed"`
}

// VerifyYield runs deterministic checks on a yield result.
func VerifyYield(result *YieldResult, expectedCols []string) *VerificationResult {
	vr := &VerificationResult{
		Status: VerificationPass,
		Passed: true,
	}

	// Check 1: required columns.
	vr.addCheck(checkRequiredColumns(expectedCols, result.Params.PassColumn, result.Params.GroupBy))

	// Check 2: denominator non-zero.
	if result.TotalCount == 0 {
		vr.addCheck(VerificationCheck{
			Name: "denominator_non_zero", Status: VerificationFail,
			Message: "total count is zero — cannot calculate yield",
		})
	} else {
		vr.addCheck(VerificationCheck{
			Name: "denominator_non_zero", Status: VerificationPass,
			Message: fmt.Sprintf("total count = %d", result.TotalCount),
		})
	}

	// Check 3: yield in valid range.
	if result.Yield < 0 || result.Yield > 1 {
		vr.addCheck(VerificationCheck{
			Name: "yield_range", Status: VerificationFail,
			Message: fmt.Sprintf("yield %.4f out of valid range [0.0, 1.0]", result.Yield),
		})
	} else {
		vr.addCheck(VerificationCheck{
			Name: "yield_range", Status: VerificationPass,
			Message: fmt.Sprintf("yield = %.4f (%.2f%%)", result.Yield, result.Yield*100),
		})
	}

	// Check 4: pass + fail = total.
	if result.PassCount+result.FailCount != result.TotalCount {
		vr.addCheck(VerificationCheck{
			Name: "pass_fail_total", Status: VerificationFail,
			Message: fmt.Sprintf("pass(%d) + fail(%d) != total(%d)",
				result.PassCount, result.FailCount, result.TotalCount),
		})
	} else {
		vr.addCheck(VerificationCheck{
			Name: "pass_fail_total", Status: VerificationPass,
			Message: "pass + fail = total",
		})
	}

	// Check 5: group counts reconcile.
	if len(result.Groups) > 0 {
		groupTotal := 0
		groupPass := 0
		groupFail := 0
		for _, gy := range result.Groups {
			groupTotal += gy.Total
			groupPass += gy.Pass
			groupFail += gy.Fail
		}
		if groupTotal != result.TotalCount || groupPass != result.PassCount || groupFail != result.FailCount {
			vr.addCheck(VerificationCheck{
				Name: "group_reconciliation", Status: VerificationFail,
				Message: fmt.Sprintf("group totals (%d/%d/%d) != overall (%d/%d/%d)",
					groupPass, groupFail, groupTotal,
					result.PassCount, result.FailCount, result.TotalCount),
			})
		} else {
			vr.addCheck(VerificationCheck{
				Name: "group_reconciliation", Status: VerificationPass,
				Message: fmt.Sprintf("%d groups reconciled", len(result.Groups)),
			})
		}
	}
	return vr
}

func (vr *VerificationResult) addCheck(c VerificationCheck) {
	vr.Checks = append(vr.Checks, c)
	if c.Status == VerificationFail {
		vr.Status = VerificationFail
		vr.Passed = false
	} else if c.Status == VerificationWarn && vr.Status == VerificationPass {
		vr.Status = VerificationWarn
	}
}

func checkRequiredColumns(available []string, passCol, groupCol string) VerificationCheck {
	avail := make(map[string]bool)
	for _, c := range available {
		avail[c] = true
	}
	var missing []string
	if !avail[passCol] {
		missing = append(missing, passCol)
	}
	if groupCol != "" && !avail[groupCol] {
		missing = append(missing, groupCol)
	}
	if len(missing) > 0 {
		return VerificationCheck{
			Name: "required_columns", Status: VerificationFail,
			Message: fmt.Sprintf("missing columns: %v (available: %v)", missing, available),
		}
	}
	return VerificationCheck{
		Name: "required_columns", Status: VerificationPass,
		Message: fmt.Sprintf("all required columns present: pass=%q group=%q", passCol, groupCol),
	}
}

// Summary returns a human-readable verification summary.
func (vr *VerificationResult) Summary() string {
	s := fmt.Sprintf("Verification: %s\n", vr.Status)
	for _, c := range vr.Checks {
		s += fmt.Sprintf("  [%s] %s: %s\n", c.Status, c.Name, c.Message)
	}
	return s
}
