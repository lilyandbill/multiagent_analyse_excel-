// Package ft_workflow implements the single-agent FT yield analysis workflow.
// All calculations and verifications are deterministic; the LLM is used only for
// intent understanding, skill selection, plan generation, and result summarization.
package ft_workflow

import (
	"fmt"
	"path/filepath"
	"strings"

	"excel-agent/generic"
)

// DataCatalog is a structured description of all tables in scope.
type DataCatalog struct {
	Tables []TableSchema `json:"tables"`
}

// TableSchema describes a single table's structure.
type TableSchema struct {
	FileName  string       `json:"file_name"`
	SheetName string       `json:"sheet_name"`
	RowCount  int          `json:"row_count"`
	Columns   []ColumnInfo `json:"columns"`
}

// ColumnInfo describes a single column.
type ColumnInfo struct {
	Name     string `json:"name"`
	Position int    `json:"position"` // 0-based column index
	Sample   string `json:"sample"`   // first data row value
}

// BuildCatalog reads all Excel files in a directory and builds a DataCatalog.
func BuildCatalog(workDir string) (*DataCatalog, error) {
	previews, err := generic.PreviewPath(workDir)
	if err != nil {
		return nil, fmt.Errorf("preview files: %w", err)
	}
	if len(previews) == 0 {
		return nil, fmt.Errorf("no files found in %s", workDir)
	}

	catalog := &DataCatalog{}
	for _, pf := range previews {
		baseName := filepath.Base(pf.FilePath)
		for _, sfp := range pf.SingleFilePreviews {
			ts := TableSchema{
				FileName:  baseName,
				SheetName: sfp.SheetName,
				RowCount:  len(sfp.Content),
				Columns:   buildColumns(sfp),
			}
			catalog.Tables = append(catalog.Tables, ts)
		}
	}
	return catalog, nil
}

func buildColumns(sfp *generic.SingleFilePreview) []ColumnInfo {
	cols := make([]ColumnInfo, 0, len(sfp.Header))
	for i, cell := range sfp.Header {
		sample := ""
		if len(sfp.Content) > 0 && i < len(sfp.Content[0]) {
			sample = sfp.Content[0][i].Value
		}
		cols = append(cols, ColumnInfo{
			Name:     strings.TrimSpace(cell.Value),
			Position: i,
			Sample:   sample,
		})
	}
	return cols
}

// Summary returns a human-readable summary of the catalog.
func (c *DataCatalog) Summary() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Data Catalog: %d table(s)\n", len(c.Tables)))
	for _, t := range c.Tables {
		b.WriteString(fmt.Sprintf("  - %s / %s (%d rows)\n", t.FileName, t.SheetName, t.RowCount))
		for _, col := range t.Columns {
			b.WriteString(fmt.Sprintf("    [%d] %s (sample: %q)\n", col.Position, col.Name, col.Sample))
		}
	}
	return b.String()
}

// ColumnNames returns the column names for a specific table (by index).
func (c *DataCatalog) ColumnNames(tableIdx int) []string {
	if tableIdx < 0 || tableIdx >= len(c.Tables) {
		return nil
	}
	names := make([]string, len(c.Tables[tableIdx].Columns))
	for i, col := range c.Tables[tableIdx].Columns {
		names[i] = col.Name
	}
	return names
}
