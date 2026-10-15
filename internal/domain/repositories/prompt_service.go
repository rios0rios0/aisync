package repositories

import "github.com/rios0rios0/aisync/internal/domain/entities"

// PromptService provides interactive user prompts. Implementations can use TUI
// frameworks (e.g., charmbracelet/huh) for rich terminals or fall back to simple
// stdin/stdout for non-interactive environments.
type PromptService interface {
	// PromptToolAction asks the user what to do for a specific tool during pull.
	// Returns "apply", "skip", "diff", or "abort".
	PromptToolAction(toolName string) string

	// PromptConfirmation asks a yes/no question. Returns true for yes.
	PromptConfirmation(prompt string) bool

	// PromptSyncConflict asks how to resolve one sync conflict. The detail
	// carries both timestamps, where the repo change came from, and the
	// unified diff, so implementations can let the user inspect the diff
	// before answering. Implementations without a terminal return
	// [entities.ChoiceSkip] so nothing is overwritten unattended.
	PromptSyncConflict(detail entities.ConflictDetail) entities.ConflictChoice

	// PromptFileAction asks the user what to do for a specific file.
	// Returns "apply" or "skip".
	PromptFileAction(path, direction string) string
}
