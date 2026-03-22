package record

import (
	"testing"
)

func TestFlatten_NestedMap(t *testing.T) {
	input := map[string]any{
		"a": map[string]any{
			"b": 1,
		},
	}
	result := Flatten(input)
	if result["a_b"] != 1 {
		t.Errorf("expected a_b=1, got %v", result["a_b"])
	}
}

func TestFlatten_DeeplyNested(t *testing.T) {
	input := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "deep",
			},
		},
	}
	result := Flatten(input)
	if result["a_b_c"] != "deep" {
		t.Errorf("expected a_b_c=deep, got %v", result["a_b_c"])
	}
}

func TestFlatten_Arrays(t *testing.T) {
	input := map[string]any{
		"items": []any{1, 2, 3},
	}
	result := Flatten(input)
	if result["items_0"] != 1 {
		t.Errorf("expected items_0=1, got %v", result["items_0"])
	}
	if result["items_1"] != 2 {
		t.Errorf("expected items_1=2, got %v", result["items_1"])
	}
	if result["items_2"] != 3 {
		t.Errorf("expected items_2=3, got %v", result["items_2"])
	}
}

func TestFlatten_NilValues(t *testing.T) {
	input := map[string]any{
		"a": nil,
	}
	result := Flatten(input)
	if v, ok := result["a"]; !ok || v != nil {
		t.Errorf("expected a=nil, got %v (ok=%v)", v, ok)
	}
}

func TestFlatten_EmptyMap(t *testing.T) {
	result := Flatten(map[string]any{})
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestFlatten_AlreadyFlat(t *testing.T) {
	input := map[string]any{
		"x": 1,
		"y": "hello",
		"z": true,
	}
	result := Flatten(input)
	if result["x"] != 1 || result["y"] != "hello" || result["z"] != true {
		t.Errorf("flat map not preserved: %v", result)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 keys, got %d", len(result))
	}
}

func TestFlatten_NestedArray(t *testing.T) {
	input := map[string]any{
		"data": []any{
			map[string]any{"name": "alice"},
			map[string]any{"name": "bob"},
		},
	}
	result := Flatten(input)
	if result["data_0_name"] != "alice" {
		t.Errorf("expected data_0_name=alice, got %v", result["data_0_name"])
	}
	if result["data_1_name"] != "bob" {
		t.Errorf("expected data_1_name=bob, got %v", result["data_1_name"])
	}
}

func TestEnsureFlattened_PopulatesFromPayload(t *testing.T) {
	r := NewRecord()
	r.Payload = map[string]any{
		"a": map[string]any{"b": 1},
	}
	r.Flattened = nil

	r.EnsureFlattened()

	if r.Flattened == nil {
		t.Fatal("Flattened should not be nil after EnsureFlattened")
	}
	if r.Flattened["a_b"] != 1 {
		t.Errorf("expected a_b=1, got %v", r.Flattened["a_b"])
	}
}

func TestEnsureFlattened_DoesNotOverwrite(t *testing.T) {
	r := NewRecord()
	r.Payload = map[string]any{"a": 1}
	r.Flattened = map[string]any{"existing": "value"}

	r.EnsureFlattened()

	if _, ok := r.Flattened["a"]; ok {
		t.Error("EnsureFlattened should not overwrite existing Flattened")
	}
	if r.Flattened["existing"] != "value" {
		t.Error("existing Flattened data should be preserved")
	}
}

func TestEnsureFlattened_NilPayload(t *testing.T) {
	r := NewRecord()
	r.Payload = nil
	r.Flattened = nil

	r.EnsureFlattened()

	if r.Flattened != nil {
		t.Error("Flattened should remain nil when Payload is nil")
	}
}
