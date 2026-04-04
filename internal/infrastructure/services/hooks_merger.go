package services

import (
	"encoding/json"
	"fmt"
	"slices"

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
	merged := make(map[string]any)
	hooks := make(map[string][]any)

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

	deduped := make(map[string][]any, len(hooks))
	for event, entries := range hooks {
		deduped[event] = deduplicateHookEntries(entries)
	}

	filtered := make(map[string][]any, len(deduped))
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
func parseHooksJSON(data []byte) (map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hooks JSON: %w", err)
	}

	hooksRaw, ok := root["hooks"]
	if !ok {
		return make(map[string]any), nil
	}

	hooksMap, ok := hooksRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected 'hooks' to be an object, got %T", hooksRaw)
	}

	return hooksMap, nil
}

// collectHooks extracts hook arrays from a parsed hooks map and appends them
// to the accumulator keyed by event name.
func collectHooks(parsed map[string]any, acc map[string][]any) {
	for event, entriesRaw := range parsed {
		entries, ok := entriesRaw.([]any)
		if !ok {
			continue
		}
		acc[event] = append(acc[event], entries...)
	}
}

// deduplicateHookEntries removes duplicate hook entries by comparing their
// JSON-serialized form. The first occurrence is kept.
func deduplicateHookEntries(entries []any) []any {
	seen := make(map[string]struct{})
	result := make([]any, 0, len(entries))

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
func (m *HooksMerger) applyExcludes(event string, entries []any) []any {
	if len(m.excludes) == 0 {
		return entries
	}

	result := make([]any, 0, len(entries))
	for _, entry := range entries {
		if m.matchesExclude(event, entry) {
			continue
		}
		result = append(result, entry)
	}

	return result
}

// matchesExclude checks if a hook entry matches any exclude rule for the given event.
func (m *HooksMerger) matchesExclude(event string, entry any) bool {
	entryMap, ok := entry.(map[string]any)
	if !ok {
		return false
	}

	matcher, _ := entryMap["matcher"].(string)
	commands := extractHookCommands(entryMap)

	for _, exclude := range m.excludes {
		if m.excludeMatches(exclude, event, matcher, commands) {
			return true
		}
	}

	return false
}

// extractHookCommands extracts command strings from the "hooks" array of a hook entry.
func extractHookCommands(entryMap map[string]any) []string {
	hooksRaw, ok := entryMap["hooks"].([]any)
	if !ok {
		return nil
	}

	var commands []string
	for _, hookRaw := range hooksRaw {
		if hookMap, mapOk := hookRaw.(map[string]any); mapOk {
			if cmd, cmdOk := hookMap["command"].(string); cmdOk {
				commands = append(commands, cmd)
			}
		}
	}
	return commands
}

// excludeMatches checks if a single exclude rule matches the given event, matcher,
// and commands.
func (m *HooksMerger) excludeMatches(
	exclude entities.HooksExcludeEntry,
	event, matcher string,
	commands []string,
) bool {
	if exclude.Event != event {
		return false
	}
	if exclude.Matcher != "" && exclude.Matcher != matcher {
		return false
	}
	if exclude.Command != "" && !slices.Contains(commands, exclude.Command) {
		return false
	}
	return true
}
