package services

import (
	"encoding/json"
	"fmt"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// HooksMerger merges Claude Code hooks.json files from multiple sources,
// deduplicating entries and applying exclude rules.
type HooksMerger struct {
	excludes []entities.HooksExcludeEntry
}

// NewHooksMerger creates a new HooksMerger with the given exclude rules.
func NewHooksMerger(excludes []entities.HooksExcludeEntry) *HooksMerger {
	return &HooksMerger{excludes: excludes}
}

// SetExcludes updates the exclude rules used during hook merging. This allows
// excludes to be configured after construction, when config is loaded at pull time.
func (m *HooksMerger) SetExcludes(excludes []entities.HooksExcludeEntry) {
	m.excludes = excludes
}

// Merge combines hooks.json content from multiple shared sources and optional
// personal content. Hooks are concatenated per event key, deduplicated by their
// JSON serialization, and filtered by the configured exclude rules.
func (m *HooksMerger) Merge(sharedSources [][]byte, personal []byte) ([]byte, error) {
	merged := make(map[string]interface{})
	hooks := make(map[string][]interface{})

	for i, source := range sharedSources {
		parsed, err := parseHooksJSON(source)
		if err != nil {
			return nil, fmt.Errorf("failed to parse shared source %d: %w", i, err)
		}
		collectHooks(parsed, hooks)
	}

	if len(personal) > 0 {
		parsed, err := parseHooksJSON(personal)
		if err != nil {
			return nil, fmt.Errorf("failed to parse personal hooks: %w", err)
		}
		collectHooks(parsed, hooks)
	}

	deduped := make(map[string][]interface{}, len(hooks))
	for event, entries := range hooks {
		deduped[event] = deduplicateHookEntries(entries)
	}

	filtered := make(map[string][]interface{}, len(deduped))
	for event, entries := range deduped {
		filtered[event] = m.applyExcludes(event, entries)
	}

	merged["hooks"] = filtered

	result, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged hooks: %w", err)
	}

	return append(result, '\n'), nil
}

// parseHooksJSON parses a hooks.json byte slice and returns the inner "hooks" map.
func parseHooksJSON(data []byte) (map[string]interface{}, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hooks JSON: %w", err)
	}

	hooksRaw, ok := root["hooks"]
	if !ok {
		return make(map[string]interface{}), nil
	}

	hooksMap, ok := hooksRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected 'hooks' to be an object, got %T", hooksRaw)
	}

	return hooksMap, nil
}

// collectHooks extracts hook arrays from a parsed hooks map and appends them
// to the accumulator keyed by event name.
func collectHooks(parsed map[string]interface{}, acc map[string][]interface{}) {
	for event, entriesRaw := range parsed {
		entries, ok := entriesRaw.([]interface{})
		if !ok {
			continue
		}
		acc[event] = append(acc[event], entries...)
	}
}

// deduplicateHookEntries removes duplicate hook entries by comparing their
// JSON-serialized form. The first occurrence is kept.
func deduplicateHookEntries(entries []interface{}) []interface{} {
	seen := make(map[string]struct{})
	result := make([]interface{}, 0, len(entries))

	for _, entry := range entries {
		serialized, err := json.Marshal(entry)
		if err != nil {
			result = append(result, entry)
			continue
		}

		key := string(serialized)
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		result = append(result, entry)
	}

	return result
}

// applyExcludes removes hook entries that match any of the configured exclude rules
// for the given event key.
func (m *HooksMerger) applyExcludes(event string, entries []interface{}) []interface{} {
	if len(m.excludes) == 0 {
		return entries
	}

	result := make([]interface{}, 0, len(entries))
	for _, entry := range entries {
		if m.matchesExclude(event, entry) {
			continue
		}
		result = append(result, entry)
	}

	return result
}

// matchesExclude checks if a hook entry matches any exclude rule for the given event.
func (m *HooksMerger) matchesExclude(event string, entry interface{}) bool {
	entryMap, ok := entry.(map[string]interface{})
	if !ok {
		return false
	}

	matcher, _ := entryMap["matcher"].(string)

	var command string
	if hooksRaw, ok := entryMap["hooks"].([]interface{}); ok {
		for _, hookRaw := range hooksRaw {
			if hookMap, ok := hookRaw.(map[string]interface{}); ok {
				if cmd, ok := hookMap["command"].(string); ok {
					command = cmd
					break
				}
			}
		}
	}

	for _, exclude := range m.excludes {
		if exclude.Event != event {
			continue
		}
		if exclude.Matcher != "" && exclude.Matcher != matcher {
			continue
		}
		if exclude.Command != "" && exclude.Command != command {
			continue
		}
		return true
	}

	return false
}
