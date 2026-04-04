package repositories

// PromptService provides interactive user prompts. Implementations can use TUI
// frameworks (e.g., charmbracelet/huh) for rich terminals or fall back to simple
// stdin/stdout for non-interactive environments.
type PromptService interface {
	// PromptToolAction asks the user what to do for a specific tool during pull.
	// Returns "apply", "skip", "diff", or "abort".
	PromptToolAction(toolName string) string

	// PromptConfirmation asks a yes/no question. Returns true for yes.
	PromptConfirmation(prompt string) bool

	// PromptConflictResolution asks how to resolve a file conflict.
	// Returns "local", "remote", or "skip".
	PromptConflictResolution(path, remoteDevice string) string

	// PromptFileAction asks the user what to do for a specific file.
	// Returns "apply" or "skip".
	PromptFileAction(path, direction string) string
}
