package ft_workflow

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// YieldParams defines the parameters for yield calculation.
type YieldParams struct {
	TableIndex int    `json:"table_index"` // which table from the catalog
	GroupBy    string `json:"group_by"`    // column name to group by (e.g., "LOT_ID"), empty = overall
	PassValue  string `json:"pass_value"`  // value that indicates PASS (e.g., "PASS", "1", "Y")
	PassColumn string `json:"pass_column"` // column name containing pass/fail
}

// YieldResult is the output of yield calculation.
type YieldResult struct {
	Params     YieldParams  `json:"params"`
	TotalCount int          `json:"total_count"`
	PassCount  int          `json:"pass_count"`
	FailCount  int          `json:"fail_count"`
	Yield      float64      `json:"yield"` // 0.0 ~ 1.0
	Groups     []GroupYield `json:"groups,omitempty"`
	GroupBy    string       `json:"group_by,omitempty"`
}

// GroupYield is per-group yield data.
type GroupYield struct {
	GroupValue string  `json:"group_value"`
	Total      int     `json:"total"`
	Pass       int     `json:"pass"`
	Fail       int     `json:"fail"`
	Yield      float64 `json:"yield"`
}

// CalculateYield reads the Excel file and computes yield deterministically.
func CalculateYield(filePath, sheetName string, params YieldParams) (*YieldResult, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("read rows: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("sheet %q has no data rows (need header + >=1 row)", sheetName)
	}

	header := rows[0]
	colIdx := mapHeader(header)

	passCol, ok := colIdx[params.PassColumn]
	if !ok {
		return nil, fmt.Errorf("pass column %q not found. Available: %v", params.PassColumn, header)
	}

	// Determine group column.
	var groupCol int
	hasGroup := params.GroupBy != ""
	if hasGroup {
		gc, ok := colIdx[params.GroupBy]
		if !ok {
			return nil, fmt.Errorf("group column %q not found. Available: %v", params.GroupBy, header)
		}
		groupCol = gc
	}

	// Calculate.
	var total, pass, fail int
	groups := map[string]*GroupYield{}

	for _, row := range rows[1:] {
		if len(row) <= passCol {
			continue
		}
		passVal := strings.TrimSpace(row[passCol])
		isPass := strings.EqualFold(passVal, params.PassValue)

		total++
		if isPass {
			pass++
		} else {
			fail++
		}

		if hasGroup {
			gv := ""
			if len(row) > groupCol {
				gv = strings.TrimSpace(row[groupCol])
			}
			if gv == "" {
				gv = "(empty)"
			}
			gy, ok := groups[gv]
			if !ok {
				gy = &GroupYield{GroupValue: gv}
				groups[gv] = gy
			}
			gy.Total++
			if isPass {
				gy.Pass++
			} else {
				gy.Fail++
			}
		}
	}

	for _, gy := range groups {
		if gy.Total > 0 {
			gy.Yield = round4(float64(gy.Pass) / float64(gy.Total))
		}
	}

	r := &YieldResult{
		Params:     params,
		TotalCount: total,
		PassCount:  pass,
		FailCount:  fail,
		GroupBy:    params.GroupBy,
	}
	if total > 0 {
		r.Yield = round4(float64(pass) / float64(total))
	}
	if hasGroup {
		r.Groups = make([]GroupYield, 0, len(groups))
		for _, gy := range groups {
			r.Groups = append(r.Groups, *gy)
		}
	}
	return r, nil
}

func mapHeader(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

func round4(f float64) float64 {
	return float64(int(f*10000+0.5)) / 10000
}

// YieldSummary returns a human-readable yield summary.
func (r *YieldResult) YieldSummary() string {
	s := fmt.Sprintf("Yield: %.2f%% (%d/%d pass, %d fail)", r.Yield*100, r.PassCount, r.TotalCount, r.FailCount)
	if len(r.Groups) > 0 {
		s += fmt.Sprintf("\nGrouped by %s:", r.GroupBy)
		for _, gy := range r.Groups {
			s += fmt.Sprintf("\n  %s: %.2f%% (%d/%d)", gy.GroupValue, gy.Yield*100, gy.Pass, gy.Total)
		}
	}
	return s
}
