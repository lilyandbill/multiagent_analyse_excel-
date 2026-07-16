// Package skills provides versioned, pluggable business capabilities for the
// single-agent V1 architecture.
//
// Each Skill is a self-contained unit with a manifest, version, input contract,
// allowed tools, hooks, permissions, budget, handler, and tests.
package skills

import (
	"context"
	"fmt"
)

// Version represents a semantic version for a skill.
type Version struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
}

// String returns the dotted version string.
func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1 if v < other, 0 if equal, +1 if v > other.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return sign(v.Major - other.Major)
	}
	if v.Minor != other.Minor {
		return sign(v.Minor - other.Minor)
	}
	return sign(v.Patch - other.Patch)
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	if n > 0 {
		return 1
	}
	return 0
}

// FieldDefinition describes a required or optional input field for a skill.
type FieldDefinition struct {
	// Name is the field identifier.
	Name string `json:"name"`

	// Type is the expected Go type (e.g., "string", "int", "[]string").
	Type string `json:"type"`

	// Required is true if the field must be present.
	Required bool `json:"required"`

	// Description explains what the field is used for.
	Description string `json:"description"`

	// Default is the default value if the field is not provided (optional fields only).
	Default any `json:"default,omitempty"`
}

// ToolPermission defines which tools a skill is allowed to use and under what conditions.
type ToolPermission struct {
	// ToolName identifies the tool.
	ToolName string `json:"tool_name"`

	// MaxCalls limits the number of times this tool can be invoked.
	MaxCalls int `json:"max_calls"`

	// RequiresApproval is true if each call needs explicit user approval.
	RequiresApproval bool `json:"requires_approval"`
}

// Manifest describes a skill's identity, contract, and constraints.
type Manifest struct {
	// Name is a unique, machine-readable identifier (e.g., "ft_yield_analysis").
	Name string `json:"name"`

	// DisplayName is a human-readable label.
	DisplayName string `json:"display_name"`

	// Version is the skill's semantic version.
	Version Version `json:"version"`

	// Description explains what the skill does.
	Description string `json:"description"`

	// InputContract lists the required and optional input fields.
	InputContract []FieldDefinition `json:"input_contract"`

	// AllowedTools lists the tools this skill is permitted to use.
	AllowedTools []ToolPermission `json:"allowed_tools,omitempty"`

	// HookNames lists the hook names this skill subscribes to.
	HookNames []string `json:"hook_names,omitempty"`

	// Permissions lists coarse-grained permissions (e.g., "read_file", "write_artifact").
	Permissions []string `json:"permissions,omitempty"`

	// MaxEstimatedCost is the skill-level cost ceiling.
	MaxEstimatedCost float64 `json:"max_estimated_cost"`
}

// Validate checks that the manifest has all required fields and is internally consistent.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	if err := m.Version.Validate(); err != nil {
		return fmt.Errorf("skill version: %w", err)
	}
	if m.Description == "" {
		return fmt.Errorf("skill description is required")
	}
	// Validate input contract: required fields must have non-empty names and types,
	// and field names must be unique.
	if err := validateFieldDefinitions(m.InputContract); err != nil {
		return err
	}
	// Validate that hook names and tool names are unique within their lists.
	if err := validateUniqueStrings(m.HookNames, "hook_names"); err != nil {
		return err
	}
	if err := validateToolPermissions(m.AllowedTools); err != nil {
		return err
	}
	return nil
}

// Validate checks that the version components are non-negative and the version is not 0.0.0.
func (v Version) Validate() error {
	if v.Major < 0 || v.Minor < 0 || v.Patch < 0 {
		return fmt.Errorf("version components must be non-negative, got %s", v)
	}
	if v.Major == 0 && v.Minor == 0 && v.Patch == 0 {
		return fmt.Errorf("version must not be 0.0.0")
	}
	return nil
}

func validateFieldDefinitions(fields []FieldDefinition) error {
	seen := make(map[string]int) // name -> first index
	for i, fd := range fields {
		if fd.Name == "" {
			return fmt.Errorf("input_contract[%d]: field name is required", i)
		}
		if fd.Required && fd.Type == "" {
			return fmt.Errorf("input_contract[%d] (%s): type is required for required fields", i, fd.Name)
		}
		if firstIdx, exists := seen[fd.Name]; exists {
			return fmt.Errorf("input_contract[%d] (%s): duplicate field name (first defined at index %d)", i, fd.Name, firstIdx)
		}
		seen[fd.Name] = i
	}
	return nil
}

func validateUniqueStrings(values []string, label string) error {
	seen := make(map[string]bool)
	for _, v := range values {
		if v == "" {
			continue // empty strings are not considered duplicates
		}
		if seen[v] {
			return fmt.Errorf("%s: duplicate value %q", label, v)
		}
		seen[v] = true
	}
	return nil
}

func validateToolPermissions(tools []ToolPermission) error {
	seen := make(map[string]bool)
	for _, tp := range tools {
		if tp.ToolName == "" {
			continue
		}
		if seen[tp.ToolName] {
			return fmt.Errorf("allowed_tools: duplicate tool name %q", tp.ToolName)
		}
		seen[tp.ToolName] = true
	}
	return nil
}

// Handler is the function signature for executing a skill.
// It receives the execution context, resolved input parameters, and returns
// the skill result or an error.
type Handler func(ctx context.Context, input map[string]any) (any, error)

// Skill bundles a manifest with its handler implementation.
type Skill struct {
	Manifest Manifest `json:"manifest"`
	Handler  Handler  `json:"-"`
}

// NewSkill creates a new Skill with the given manifest and handler.
func NewSkill(manifest Manifest, handler Handler) (*Skill, error) {
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid skill manifest: %w", err)
	}
	if handler == nil {
		return nil, fmt.Errorf("skill handler must not be nil")
	}
	return &Skill{
		Manifest: manifest,
		Handler:  handler,
	}, nil
}

// Predefined V1 built-in skill names.
const (
	SkillInspectWorkbook = "inspect_workbook"
	SkillYieldAnalysis   = "yield_analysis"
	SkillBinDistribution = "bin_distribution"
	SkillFailItemTopN    = "fail_item_topn"
	SkillFTYieldAnalysis = "ft_yield_analysis"
)
