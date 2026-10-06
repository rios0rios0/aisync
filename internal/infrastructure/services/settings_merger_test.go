package services_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	services "github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func TestSettingsMerger_Merge_ShouldDeepMergeNestedKeysFromTwoSources(t *testing.T) {
	// given
	source1 := []byte(`{
		"editor": {
			"tabSize": 4,
			"formatOnSave": true
		}
	}`)
	source2 := []byte(`{
		"editor": {
			"wordWrap": "on"
		},
		"terminal": {
			"fontSize": 14
		}
	}`)
	merger := services.NewSettingsMerger()

	// when
	result, err := merger.Merge([][]byte{source1, source2}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(result, &parsed))

	editor := parsed["editor"].(map[string]any)
	assert.InDelta(t, 4, editor["tabSize"], 0)
	assert.Equal(t, true, editor["formatOnSave"])
	assert.Equal(t, "on", editor["wordWrap"])

	terminal := parsed["terminal"].(map[string]any)
	assert.InDelta(t, 14, terminal["fontSize"], 0)
}

func TestSettingsMerger_Merge_ShouldLetPersonalWinOnCollision(t *testing.T) {
	// given
	shared := []byte(`{
		"editor": {
			"tabSize": 4
		}
	}`)
	personal := []byte(`{
		"editor": {
			"tabSize": 2
		}
	}`)
	merger := services.NewSettingsMerger()

	// when
	result, err := merger.Merge([][]byte{shared}, personal)

	// then
	assert.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(result, &parsed))

	editor := parsed["editor"].(map[string]any)
	assert.InDelta(t, 2, editor["tabSize"], 0)
}

func TestSettingsMerger_Merge_ShouldMergeArraysByUnionWithUniqueElementsOnly(t *testing.T) {
	// given
	source1 := []byte(`{
		"plugins": ["pluginA", "pluginB"]
	}`)
	source2 := []byte(`{
		"plugins": ["pluginB", "pluginC"]
	}`)
	merger := services.NewSettingsMerger()

	// when
	result, err := merger.Merge([][]byte{source1, source2}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(result, &parsed))

	plugins := parsed["plugins"].([]any)
	assert.Len(t, plugins, 3)
	assert.Contains(t, plugins, "pluginA")
	assert.Contains(t, plugins, "pluginB")
	assert.Contains(t, plugins, "pluginC")
}

func TestSettingsMerger_Merge_ShouldReturnEmptyObjectWhenInputsAreEmpty(t *testing.T) {
	// given
	merger := services.NewSettingsMerger()

	// when
	result, err := merger.Merge(nil, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(result, &parsed))
	assert.Empty(t, parsed)
}

func TestSettingsMerger_Merge_ShouldMergeSingleSourceWithPersonal(t *testing.T) {
	// given
	source := []byte(`{
		"theme": "dark",
		"editor": {"fontSize": 14}
	}`)
	personal := []byte(`{
		"theme": "light",
		"keybindings": {"save": "ctrl+s"}
	}`)
	merger := services.NewSettingsMerger()

	// when
	result, err := merger.Merge([][]byte{source}, personal)

	// then
	assert.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(result, &parsed))

	assert.Equal(t, "light", parsed["theme"])

	editor := parsed["editor"].(map[string]any)
	assert.InDelta(t, 14, editor["fontSize"], 0)

	keybindings := parsed["keybindings"].(map[string]any)
	assert.Equal(t, "ctrl+s", keybindings["save"])
}

func TestSettingsMerger_Merge_ShouldReturnErrorWhenSharedSourceIsMalformed(t *testing.T) {
	// given
	malformed := []byte(`{invalid json`)
	merger := services.NewSettingsMerger()

	// when
	_, err := merger.Merge([][]byte{malformed}, nil)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse shared source 0")
}

func TestSettingsMerger_Merge_ShouldReturnErrorWhenPersonalIsMalformed(t *testing.T) {
	// given
	valid := []byte(`{"key": "value"}`)
	malformed := []byte(`{broken`)
	merger := services.NewSettingsMerger()

	// when
	_, err := merger.Merge([][]byte{valid}, malformed)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse personal settings")
}

func TestSettingsMerger_Merge_ShouldDeepMergeThreeLevelsDeep(t *testing.T) {
	// given
	source1 := []byte(`{
		"editor": {
			"formatting": {
				"indentation": {
					"tabSize": 4,
					"useTabs": true
				}
			}
		}
	}`)
	source2 := []byte(`{
		"editor": {
			"formatting": {
				"indentation": {
					"insertSpaces": false
				},
				"trimWhitespace": true
			}
		}
	}`)
	merger := services.NewSettingsMerger()

	// when
	result, err := merger.Merge([][]byte{source1, source2}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(result, &parsed))

	editor := parsed["editor"].(map[string]any)
	formatting := editor["formatting"].(map[string]any)
	indentation := formatting["indentation"].(map[string]any)

	assert.InDelta(t, 4, indentation["tabSize"], 0)
	assert.Equal(t, true, indentation["useTabs"])
	assert.Equal(t, false, indentation["insertSpaces"])
	assert.Equal(t, true, formatting["trimWhitespace"])
}

func TestSettingsMerger_Merge_ShouldMergeEmptyArrays(t *testing.T) {
	// given
	source1 := []byte(`{
		"plugins": [],
		"rules": ["ruleA"]
	}`)
	source2 := []byte(`{
		"plugins": ["pluginX"],
		"rules": []
	}`)
	merger := services.NewSettingsMerger()

	// when
	result, err := merger.Merge([][]byte{source1, source2}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(result, &parsed))

	plugins := parsed["plugins"].([]any)
	assert.Len(t, plugins, 1)
	assert.Equal(t, "pluginX", plugins[0])

	rules := parsed["rules"].([]any)
	assert.Len(t, rules, 1)
	assert.Equal(t, "ruleA", rules[0])
}

func TestSettingsMerger_Merge_ShouldHandleNilPersonal(t *testing.T) {
	// given
	source := []byte(`{
		"theme": "dark",
		"editor": {"tabSize": 4}
	}`)
	merger := services.NewSettingsMerger()

	// when
	result, err := merger.Merge([][]byte{source}, nil)

	// then
	assert.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(result, &parsed))

	assert.Equal(t, "dark", parsed["theme"])
	editor := parsed["editor"].(map[string]any)
	assert.InDelta(t, 4, editor["tabSize"], 0)
}
