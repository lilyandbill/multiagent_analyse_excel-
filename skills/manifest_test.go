package skills

import (
	"context"
	"testing"
)

func TestVersion_String(t *testing.T) {
	v := Version{1, 2, 3}
	if v.String() != "v1.2.3" {
		t.Errorf("String() = %q, want %q", v.String(), "v1.2.3")
	}
}

func TestVersion_Compare(t *testing.T) {
	tests := []struct {
		a, b Version
		want int
	}{
		{Version{1, 0, 0}, Version{1, 0, 0}, 0},
		{Version{2, 0, 0}, Version{1, 0, 0}, 1},
		{Version{1, 0, 0}, Version{2, 0, 0}, -1},
		{Version{1, 1, 0}, Version{1, 0, 0}, 1},
		{Version{1, 0, 0}, Version{1, 1, 0}, -1},
		{Version{1, 0, 1}, Version{1, 0, 0}, 1},
		{Version{1, 0, 0}, Version{1, 0, 1}, -1},
	}
	for _, tt := range tests {
		got := tt.a.Compare(tt.b)
		if got != tt.want {
			t.Errorf("%s.Compare(%s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestManifest_Validate_Valid(t *testing.T) {
	m := Manifest{
		Name:        "ft_yield_analysis",
		DisplayName: "FT Yield Analysis",
		Version:     Version{1, 0, 0},
		Description: "Calculates and verifies yield for Final Test data.",
		InputContract: []FieldDefinition{
			{Name: "table_id", Type: "string", Required: true, Description: "Target table"},
		},
	}
	if err := m.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestManifest_Validate_MissingName(t *testing.T) {
	m := Manifest{
		Version:     Version{1, 0, 0},
		Description: "desc",
	}
	if err := m.Validate(); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestManifest_Validate_ZeroVersion(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     Version{0, 0, 0},
		Description: "desc",
	}
	if err := m.Validate(); err == nil {
		t.Error("expected error for zero version")
	}
}

func TestManifest_Validate_MissingDescription(t *testing.T) {
	m := Manifest{
		Name:    "test",
		Version: Version{1, 0, 0},
	}
	if err := m.Validate(); err == nil {
		t.Error("expected error for missing description")
	}
}

func TestManifest_Validate_InputContractEmptyName(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     Version{1, 0, 0},
		Description: "desc",
		InputContract: []FieldDefinition{
			{Name: "", Type: "string", Required: true},
		},
	}
	if err := m.Validate(); err == nil {
		t.Error("expected error for empty field name")
	}
}

func TestManifest_Validate_InputContractRequiredNoType(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     Version{1, 0, 0},
		Description: "desc",
		InputContract: []FieldDefinition{
			{Name: "field1", Required: true, Type: ""},
		},
	}
	if err := m.Validate(); err == nil {
		t.Error("expected error for required field without type")
	}
}

func TestManifest_Validate_OptionalFieldNoType(t *testing.T) {
	// Optional fields without a type are allowed (they just have defaults).
	m := Manifest{
		Name:        "test",
		Version:     Version{1, 0, 0},
		Description: "desc",
		InputContract: []FieldDefinition{
			{Name: "opt", Required: false, Type: "", Default: "fallback"},
		},
	}
	if err := m.Validate(); err != nil {
		t.Errorf("optional field without type should be allowed: %v", err)
	}
}

func TestNewSkill(t *testing.T) {
	m := Manifest{
		Name:        "test_skill",
		Version:     Version{1, 0, 0},
		Description: "A test skill.",
	}
	handler := func(ctx context.Context, input map[string]any) (any, error) {
		return "ok", nil
	}

	skill, err := NewSkill(m, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Manifest.Name != "test_skill" {
		t.Errorf("Name = %q, want %q", skill.Manifest.Name, "test_skill")
	}
}

func TestNewSkill_InvalidManifest(t *testing.T) {
	m := Manifest{} // invalid
	handler := func(ctx context.Context, input map[string]any) (any, error) {
		return nil, nil
	}
	_, err := NewSkill(m, handler)
	if err == nil {
		t.Error("expected error for invalid manifest")
	}
}

func TestNewSkill_NilHandler(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     Version{1, 0, 0},
		Description: "desc",
	}
	_, err := NewSkill(m, nil)
	if err == nil {
		t.Error("expected error for nil handler")
	}
}

func TestSkill_Execute(t *testing.T) {
	m := Manifest{
		Name:        "echo",
		Version:     Version{1, 0, 0},
		Description: "Echoes the input.",
	}
	handler := func(ctx context.Context, input map[string]any) (any, error) {
		return input["message"], nil
	}

	skill, err := NewSkill(m, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := skill.Handler(context.Background(), map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result != "hello" {
		t.Errorf("result = %v, want %q", result, "hello")
	}
}

func TestManifest_Validate_DuplicateInputFields(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     Version{1, 0, 0},
		Description: "desc",
		InputContract: []FieldDefinition{
			{Name: "field_a", Type: "string", Required: true},
			{Name: "field_b", Type: "int", Required: false},
			{Name: "field_a", Type: "string", Required: false}, // duplicate
		},
	}
	err := m.Validate()
	if err == nil {
		t.Error("expected error for duplicate input field name")
	}
}

func TestManifest_Validate_DuplicateHookNames(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     Version{1, 0, 0},
		Description: "desc",
		HookNames:   []string{"hook_a", "hook_b", "hook_a"},
	}
	err := m.Validate()
	if err == nil {
		t.Error("expected error for duplicate hook name")
	}
}

func TestManifest_Validate_DuplicateToolNames(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     Version{1, 0, 0},
		Description: "desc",
		AllowedTools: []ToolPermission{
			{ToolName: "tool_a", MaxCalls: 5},
			{ToolName: "tool_b", MaxCalls: 3},
			{ToolName: "tool_a", MaxCalls: 10},
		},
	}
	err := m.Validate()
	if err == nil {
		t.Error("expected error for duplicate tool name")
	}
}

func TestVersion_Validate_NegativeMajor(t *testing.T) {
	v := Version{-1, 0, 0}
	if err := v.Validate(); err == nil {
		t.Error("expected error for negative major version")
	}
}

func TestVersion_Validate_NegativeMinor(t *testing.T) {
	v := Version{0, -1, 0}
	if err := v.Validate(); err == nil {
		t.Error("expected error for negative minor version")
	}
}

func TestVersion_Validate_NegativePatch(t *testing.T) {
	v := Version{0, 0, -1}
	if err := v.Validate(); err == nil {
		t.Error("expected error for negative patch version")
	}
}

func TestVersion_Validate_ZeroVersion(t *testing.T) {
	v := Version{0, 0, 0}
	if err := v.Validate(); err == nil {
		t.Error("expected error for zero version")
	}
}

func TestVersion_Validate_Valid(t *testing.T) {
	v := Version{1, 2, 3}
	if err := v.Validate(); err != nil {
		t.Errorf("unexpected error for valid version: %v", err)
	}
}

func TestManifest_Validate_DuplicateHookNames_WithEmptyIgnored(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     Version{1, 0, 0},
		Description: "desc",
		HookNames:   []string{"", ""}, // empty strings are not duplicates
	}
	if err := m.Validate(); err != nil {
		t.Errorf("empty hook names should be ignored in duplicate check: %v", err)
	}
}

func TestManifest_Validate_ValidatesVersionViaManifest(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     Version{-1, 0, 0},
		Description: "desc",
	}
	err := m.Validate()
	if err == nil {
		t.Error("expected error from version validation via manifest")
	}
}

func TestVersion_String_Zero(t *testing.T) {
	v := Version{0, 0, 0}
	if v.String() != "v0.0.0" {
		t.Errorf("String() = %q, want %q", v.String(), "v0.0.0")
	}
}

func TestPredefinedSkillNames(t *testing.T) {
	names := []string{
		SkillInspectWorkbook,
		SkillYieldAnalysis,
		SkillBinDistribution,
		SkillFailItemTopN,
		SkillFTYieldAnalysis,
	}
	seen := make(map[string]bool)
	for _, name := range names {
		if name == "" {
			t.Error("skill name should not be empty")
		}
		if seen[name] {
			t.Errorf("duplicate skill name: %q", name)
		}
		seen[name] = true
	}
	if len(seen) != 5 {
		t.Errorf("expected 5 unique skill names, got %d", len(seen))
	}
}
