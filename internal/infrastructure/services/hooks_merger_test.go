//go:build unit

package services_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	services "github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func TestHooksMerger_Merge_ShouldConcatenateArraysWhenTwoSourcesShareSameEventKey(t *testing.T) {
	// given
	source1 := []byte(`{
		"hooks": {
			"PreToolUse": [
				{"matcher": "*.go", "hooks": [{"command": "golint"}]}
			]
		}
	}`)
	source2 := []byte(`{
		"hooks": {
			"PreToolUse": [
				{"matcher": "*.py", "hooks": [{"command": "pylint"}]}
			]
		}
	}`)
	merger := services.NewHooksMerger(nil)

	// when
	result, err := merger.Merge([][]byte{source1, source2}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})
	assert.Len(t, preToolUse, 2)
}

func TestHooksMerger_Merge_ShouldDeduplicateWhenSameHookAppearsInTwoSources(t *testing.T) {
	// given
	hook := `{
		"hooks": {
			"PreToolUse": [
				{"matcher": "*.go", "hooks": [{"command": "golint"}]}
			]
		}
	}`
	source1 := []byte(hook)
	source2 := []byte(hook)
	merger := services.NewHooksMerger(nil)

	// when
	result, err := merger.Merge([][]byte{source1, source2}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})
	assert.Len(t, preToolUse, 1)
}

func TestHooksMerger_Merge_ShouldPlacePersonalHooksLast(t *testing.T) {
	// given
	shared := []byte(`{
		"hooks": {
			"PreToolUse": [
				{"matcher": "shared", "hooks": [{"command": "shared-cmd"}]}
			]
		}
	}`)
	personal := []byte(`{
		"hooks": {
			"PreToolUse": [
				{"matcher": "personal", "hooks": [{"command": "personal-cmd"}]}
			]
		}
	}`)
	merger := services.NewHooksMerger(nil)

	// when
	result, err := merger.Merge([][]byte{shared}, personal)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})
	assert.Len(t, preToolUse, 2)

	first := preToolUse[0].(map[string]interface{})
	second := preToolUse[1].(map[string]interface{})
	assert.Equal(t, "shared", first["matcher"])
	assert.Equal(t, "personal", second["matcher"])
}

func TestHooksMerger_Merge_ShouldRemoveExcludedHook(t *testing.T) {
	// given
	source := []byte(`{
		"hooks": {
			"PreToolUse": [
				{"matcher": "*.go", "hooks": [{"command": "golint"}]},
				{"matcher": "*.py", "hooks": [{"command": "pylint"}]}
			]
		}
	}`)
	excludes := []entities.HooksExcludeEntry{
		{Event: "PreToolUse", Matcher: "*.go", Command: "golint"},
	}
	merger := services.NewHooksMerger(excludes)

	// when
	result, err := merger.Merge([][]byte{source}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})
	assert.Len(t, preToolUse, 1)

	remaining := preToolUse[0].(map[string]interface{})
	assert.Equal(t, "*.py", remaining["matcher"])
}

func TestHooksMerger_Merge_ShouldReturnEmptyHooksWhenInputsAreEmpty(t *testing.T) {
	// given
	merger := services.NewHooksMerger(nil)

	// when
	result, err := merger.Merge(nil, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	assert.Len(t, hooks, 0)
}

func TestHooksMerger_Merge_ShouldReturnErrorWhenJSONIsMalformed(t *testing.T) {
	// given
	malformed := []byte(`{not valid json}`)
	merger := services.NewHooksMerger(nil)

	// when
	_, err := merger.Merge([][]byte{malformed}, nil)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse shared source 0")
}

func TestHooksMerger_Merge_ShouldReturnErrorWhenPersonalJSONIsMalformed(t *testing.T) {
	// given
	valid := []byte(`{"hooks": {}}`)
	malformed := []byte(`{not valid}`)
	merger := services.NewHooksMerger(nil)

	// when
	_, err := merger.Merge([][]byte{valid}, malformed)

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse personal hooks")
}

func TestHooksMerger_Merge_ShouldMergePersonalOnlyWhenNoSharedSources(t *testing.T) {
	// given
	personal := []byte(`{
		"hooks": {
			"PostToolUse": [
				{"matcher": "*.md", "hooks": [{"command": "format-md"}]}
			]
		}
	}`)
	merger := services.NewHooksMerger(nil)

	// when
	result, err := merger.Merge(nil, personal)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	postToolUse := hooks["PostToolUse"].([]interface{})
	assert.Len(t, postToolUse, 1)

	entry := postToolUse[0].(map[string]interface{})
	assert.Equal(t, "*.md", entry["matcher"])
}

func TestHooksMerger_Merge_ShouldHandleMultipleEventKeysInOneSource(t *testing.T) {
	// given
	source := []byte(`{
		"hooks": {
			"PreToolUse": [
				{"matcher": "*.go", "hooks": [{"command": "golint"}]}
			],
			"PostToolUse": [
				{"matcher": "*.py", "hooks": [{"command": "black"}]}
			],
			"Notification": [
				{"matcher": "*", "hooks": [{"command": "notify"}]}
			]
		}
	}`)
	merger := services.NewHooksMerger(nil)

	// when
	result, err := merger.Merge([][]byte{source}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	assert.Len(t, hooks, 3)

	preToolUse := hooks["PreToolUse"].([]interface{})
	assert.Len(t, preToolUse, 1)

	postToolUse := hooks["PostToolUse"].([]interface{})
	assert.Len(t, postToolUse, 1)

	notification := hooks["Notification"].([]interface{})
	assert.Len(t, notification, 1)
}

func TestHooksMerger_Merge_ShouldSkipNonArrayHookEntries(t *testing.T) {
	// given
	source := []byte(`{
		"hooks": {
			"PreToolUse": "this is not an array"
		}
	}`)
	merger := services.NewHooksMerger(nil)

	// when
	result, err := merger.Merge([][]byte{source}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	// The non-array entry should be skipped by collectHooks
	assert.Len(t, hooks, 0)
}

func TestHooksMerger_Merge_ShouldHandleSourceWithoutHooksKey(t *testing.T) {
	// given
	source := []byte(`{"version": "1.0"}`)
	merger := services.NewHooksMerger(nil)

	// when
	result, err := merger.Merge([][]byte{source}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	assert.Len(t, hooks, 0)
}

func TestHooksMerger_Merge_ShouldExcludeByEventOnly(t *testing.T) {
	// given
	source := []byte(`{
		"hooks": {
			"PreToolUse": [
				{"matcher": "*.go", "hooks": [{"command": "golint"}]}
			],
			"PostToolUse": [
				{"matcher": "*.py", "hooks": [{"command": "pylint"}]}
			]
		}
	}`)
	// Exclude all entries for PostToolUse event (empty matcher/command matches all)
	excludes := []entities.HooksExcludeEntry{
		{Event: "PostToolUse"},
	}
	merger := services.NewHooksMerger(excludes)

	// when
	result, err := merger.Merge([][]byte{source}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal(result, &parsed))

	hooks := parsed["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})
	assert.Len(t, preToolUse, 1, "PreToolUse should not be affected")

	postToolUse := hooks["PostToolUse"].([]interface{})
	assert.Len(t, postToolUse, 0, "PostToolUse should be excluded entirely")
}

func TestHooksMerger_SetExcludes_ShouldUpdateExcludeRules(t *testing.T) {
	// given
	source := []byte(`{
		"hooks": {
			"PreToolUse": [
				{"matcher": "*.go", "hooks": [{"command": "golint"}]},
				{"matcher": "*.py", "hooks": [{"command": "pylint"}]}
			]
		}
	}`)
	merger := services.NewHooksMerger(nil)

	// First merge with no excludes — both should appear
	result1, err := merger.Merge([][]byte{source}, nil)
	assert.NoError(t, err)

	var parsed1 map[string]interface{}
	assert.NoError(t, json.Unmarshal(result1, &parsed1))
	hooks1 := parsed1["hooks"].(map[string]interface{})
	preToolUse1 := hooks1["PreToolUse"].([]interface{})
	assert.Len(t, preToolUse1, 2)

	// when
	merger.SetExcludes([]entities.HooksExcludeEntry{
		{Event: "PreToolUse", Matcher: "*.go", Command: "golint"},
	})
	result2, err := merger.Merge([][]byte{source}, nil)

	// then
	assert.NoError(t, err)

	var parsed2 map[string]interface{}
	assert.NoError(t, json.Unmarshal(result2, &parsed2))
	hooks2 := parsed2["hooks"].(map[string]interface{})
	preToolUse2 := hooks2["PreToolUse"].([]interface{})
	assert.Len(t, preToolUse2, 1)

	remaining := preToolUse2[0].(map[string]interface{})
	assert.Equal(t, "*.py", remaining["matcher"])
}
