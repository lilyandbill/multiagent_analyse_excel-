package ft_workflow

import (
	"fmt"
	"strings"
	"time"
)

// ReportData contains all information needed to generate a report.
type ReportData struct {
	TaskID       string
	Task         string
	PlanSummary  string
	Catalog      *DataCatalog
	YieldResult  *YieldResult
	Verification *VerificationResult
	ArtifactRefs []string
	GeneratedAt  time.Time
}

// GenerateReport produces a markdown report from verified results.
// Only verified results are included; the LLM may only summarize them.
func GenerateReport(data ReportData) string {
	var b strings.Builder

	b.WriteString("# FT Yield Analysis Report\n\n")
	b.WriteString(fmt.Sprintf("**Task**: %s\n", data.Task))
	b.WriteString(fmt.Sprintf("**Task ID**: %s\n", data.TaskID))
	b.WriteString(fmt.Sprintf("**Generated**: %s\n\n", data.GeneratedAt.Format(time.RFC3339)))

	// Plan.
	b.WriteString("## Plan\n\n")
	b.WriteString(data.PlanSummary)
	b.WriteString("\n\n")

	// Data Catalog.
	b.WriteString("## Data Catalog\n\n")
	if data.Catalog != nil {
		b.WriteString("```\n")
		b.WriteString(data.Catalog.Summary())
		b.WriteString("```\n\n")
	} else {
		b.WriteString("(no catalog)\n\n")
	}

	// Yield Result.
	b.WriteString("## Yield Result\n\n")
	if data.YieldResult != nil {
		b.WriteString(fmt.Sprintf("- **Overall Yield**: %.2f%% (%d/%d pass)\n",
			data.YieldResult.Yield*100, data.YieldResult.PassCount, data.YieldResult.TotalCount))
		b.WriteString(fmt.Sprintf("- **Total Tested**: %d\n", data.YieldResult.TotalCount))
		b.WriteString(fmt.Sprintf("- **Pass**: %d\n", data.YieldResult.PassCount))
		b.WriteString(fmt.Sprintf("- **Fail**: %d\n", data.YieldResult.FailCount))
		if data.YieldResult.GroupBy != "" && len(data.YieldResult.Groups) > 0 {
			b.WriteString(fmt.Sprintf("\n### By %s\n\n", data.YieldResult.GroupBy))
			b.WriteString("| Group | Total | Pass | Fail | Yield |\n")
			b.WriteString("|-------|-------|------|------|-------|\n")
			for _, gy := range data.YieldResult.Groups {
				b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %.2f%% |\n",
					gy.GroupValue, gy.Total, gy.Pass, gy.Fail, gy.Yield*100))
			}
		}
		b.WriteString("\n")
	} else {
		b.WriteString("(no yield result)\n\n")
	}

	// Verification.
	b.WriteString("## Verification\n\n")
	if data.Verification != nil {
		b.WriteString(fmt.Sprintf("**Status**: %s\n\n", data.Verification.Status))
		b.WriteString("| Check | Status | Message |\n")
		b.WriteString("|-------|--------|----------|\n")
		for _, c := range data.Verification.Checks {
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", c.Name, c.Status, c.Message))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("(no verification)\n\n")
	}

	// Artifacts.
	if len(data.ArtifactRefs) > 0 {
		b.WriteString("## Artifacts\n\n")
		for _, ref := range data.ArtifactRefs {
			b.WriteString(fmt.Sprintf("- %s\n", ref))
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("_Report generated at %s_\n", data.GeneratedAt.Format(time.RFC3339)))

	return b.String()
}
