package services

import (
	"encoding/json"
	"fmt"
)

// SettingsMerger merges JSON settings files (e.g., settings.json) from multiple
// sources using deep merge semantics. Later sources override earlier ones, and
// personal keys always win on collision. Arrays are merged by union.
type SettingsMerger struct{}

// NewSettingsMerger creates a new SettingsMerger.
func NewSettingsMerger() *SettingsMerger {
	return &SettingsMerger{}
}

// Merge combines settings from multiple shared sources and optional personal content.
// Deep merge is applied in order: earlier sources first, later override. Personal
// keys win on collision. Arrays are merged by appending unique elements.
func (m *SettingsMerger) Merge(sharedSources [][]byte, personal []byte) ([]byte, error) {
	merged := make(map[string]any)

	for i, source := range sharedSources {
		var parsed map[string]any
		if err := json.Unmarshal(source, &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse shared source %d: %w", i, err)
		}
		deepMerge(merged, parsed)
	}

	if len(personal) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(personal, &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse personal settings: %w", err)
		}
		deepMerge(merged, parsed)
	}

	result, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged settings: %w", err)
	}

	return append(result, '\n'), nil
}

// deepMerge recursively merges the src map into dst. For nested maps, the merge
// recurses. For arrays, unique elements from src are appended to dst. For scalar
// values, src overwrites dst.
func deepMerge(dst, src map[string]any) {
	for key, srcVal := range src {
		dstVal, exists := dst[key]
		if !exists {
			dst[key] = srcVal
			continue
		}

		srcMap, srcIsMap := srcVal.(map[string]any)
		dstMap, dstIsMap := dstVal.(map[string]any)
		if srcIsMap && dstIsMap {
			deepMerge(dstMap, srcMap)
			continue
		}

		srcArr, srcIsArr := srcVal.([]any)
		dstArr, dstIsArr := dstVal.([]any)
		if srcIsArr && dstIsArr {
			dst[key] = mergeArraysByUnion(dstArr, srcArr)
			continue
		}

		dst[key] = srcVal
	}
}

// mergeArraysByUnion appends elements from src to dst, skipping elements whose
// JSON serialization already exists in dst.
func mergeArraysByUnion(dst, src []any) []any {
	seen := make(map[string]struct{}, len(dst))
	for _, item := range dst {
		serialized, err := json.Marshal(item)
		if err != nil {
			continue
		}
		seen[string(serialized)] = struct{}{}
	}

	result := make([]any, 0, len(dst))
	result = append(result, dst...)

	for _, item := range src {
		serialized, err := json.Marshal(item)
		if err != nil {
			result = append(result, item)
			continue
		}
		if _, exists := seen[string(serialized)]; exists {
			continue
		}
		seen[string(serialized)] = struct{}{}
		result = append(result, item)
	}

	return result
}
