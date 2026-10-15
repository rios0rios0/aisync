package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

const actionAbort = "abort"
const actionSkipValue = "skip"
const actionApplyValue = "apply"

// HuhPromptService provides interactive prompts using charmbracelet/huh for
// TTY environments, falling back to simple stdin/stdout otherwise.
type HuhPromptService struct{}

// NewHuhPromptService creates a new HuhPromptService.
func NewHuhPromptService() *HuhPromptService {
	return &HuhPromptService{}
}

// PromptToolAction asks the user what to do for a specific tool during pull.
func (s *HuhPromptService) PromptToolAction(toolName string) string {
	if !isInteractive() {
		return promptToolActionFallback(toolName)
	}

	var choice string
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Apply changes to %s?", toolName)).
		Options(
			huh.NewOption("Apply", actionApplyValue),
			huh.NewOption("Skip", actionSkipValue),
			huh.NewOption("Show diff", "diff"),
			huh.NewOption("Abort", actionAbort),
		).
		Value(&choice).
		Run()

	if err != nil {
		return actionAbort
	}
	return choice
}

// PromptConfirmation asks a yes/no question.
func (s *HuhPromptService) PromptConfirmation(prompt string) bool {
	if !isInteractive() {
		return promptConfirmationFallback(prompt)
	}

	var confirmed bool
	err := huh.NewConfirm().
		Title(prompt).
		Affirmative("Yes").
		Negative("No").
		Value(&confirmed).
		Run()

	if err != nil {
		return false
	}
	return confirmed
}

// PromptSyncConflict prints when each side last changed, then asks how to
// resolve the conflict, offering the colored diff first, as `chezmoi apply`
// does for a target changed since it was last written. Interrupting the
// prompt quits the whole run, which leaves every file untouched.
func (s *HuhPromptService) PromptSyncConflict(detail entities.ConflictDetail) entities.ConflictChoice {
	printConflictHeader(detail)
	if !isInteractive() {
		return promptSyncConflictFallback(detail)
	}

	for {
		var choice string
		err := huh.NewSelect[string]().
			Title(fmt.Sprintf("How should %s/%s be resolved?", detail.Tool, detail.Path)).
			Options(conflictOptions(detail)...).
			Value(&choice).
			Run()
		if err != nil {
			return entities.ChoiceQuit
		}
		if choice == choiceShowDiff {
			showConflictDiff(detail)
			continue
		}
		return entities.ConflictChoice(choice)
	}
}

// choiceShowDiff is the prompt value that displays the diff and asks again.
const choiceShowDiff = "diff"

// conflictOptions builds the prompt choices, wording the two "keep" options
// as deletions when one side no longer has the file.
func conflictOptions(detail entities.ConflictDetail) []huh.Option[string] {
	keepLocal := "Keep this machine's version"
	if detail.Reason == entities.ConflictLocalDeletedRemoteModified {
		keepLocal = "Keep it deleted (delete it in the repo too)"
	}
	keepRemote := "Take the repo's version"
	if detail.Reason == entities.ConflictLocalModifiedRemoteDeleted {
		keepRemote = "Accept the deletion (delete it here too)"
	}

	var options []huh.Option[string]
	if !detail.Binary {
		options = append(options, huh.NewOption("Show diff", choiceShowDiff))
	}
	return append(options,
		huh.NewOption(keepLocal, string(entities.ChoiceKeepLocal)),
		huh.NewOption(keepRemote, string(entities.ChoiceKeepRemote)),
		huh.NewOption("Skip for now (ask again next sync)", string(entities.ChoiceSkip)),
		huh.NewOption("Keep this machine's version for all remaining conflicts", string(entities.ChoiceKeepLocalAll)),
		huh.NewOption("Take the repo's version for all remaining conflicts", string(entities.ChoiceKeepRemoteAll)),
		huh.NewOption("Quit without changing anything", string(entities.ChoiceQuit)),
	)
}

// printConflictHeader describes the conflict and when each side changed.
func printConflictHeader(detail entities.ConflictDetail) {
	formatter := NewLipglossFormatter()
	fmt.Fprintf(os.Stdout, "\n%s %s/%s — %s\n",
		formatter.Warning("Conflict:"), detail.Tool, detail.Path, detail.Reason.Describe())

	local := "deleted"
	if !detail.LocalModified.IsZero() {
		local = detail.LocalModified.Local().Format(conflictTimeLayout)
	}
	remote := "unknown"
	if !detail.RemoteModified.IsZero() {
		remote = detail.RemoteModified.Local().Format(conflictTimeLayout)
		if detail.RemoteSummary != "" {
			remote += " · " + detail.RemoteSummary
		}
	}
	fmt.Fprintln(os.Stdout, formatter.Subtle("  this machine: "+local))
	fmt.Fprintln(os.Stdout, formatter.Subtle("  repo:         "+remote))
	switch detail.NewerSide() {
	case entities.ChoiceKeepLocal:
		fmt.Fprintln(os.Stdout, formatter.Subtle("  newer:        this machine"))
	case entities.ChoiceKeepRemote:
		fmt.Fprintln(os.Stdout, formatter.Subtle("  newer:        the repo"))
	case entities.ChoiceSkip, entities.ChoiceKeepLocalAll, entities.ChoiceKeepRemoteAll, entities.ChoiceQuit:
	}
}

// conflictTimeLayout formats conflict timestamps in local time.
const conflictTimeLayout = "2006-01-02 15:04"

// showConflictDiff prints the colored diff, paging it when it is taller than
// the terminal.
func showConflictDiff(detail entities.ConflictDetail) {
	switch {
	case detail.Binary:
		fmt.Fprintln(os.Stdout, "  Binary content: no text diff is available.")
		return
	case detail.Diff == "":
		fmt.Fprintln(os.Stdout, "  The two versions have the same text; only the executable permission differs.")
		return
	}

	colored := entities.ColorizeDiff(detail.Diff, NewLipglossFormatter())
	_, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err == nil && strings.Count(colored, "\n") > height-2 {
		if _, runErr := tea.NewProgram(newDiffModel(colored), tea.WithAltScreen()).Run(); runErr == nil {
			return
		}
	}
	fmt.Fprint(os.Stdout, colored)
}

// PromptFileAction asks the user what to do for a specific file.
func (s *HuhPromptService) PromptFileAction(path, direction string) string {
	if !isInteractive() {
		return promptFileActionFallback(path, direction)
	}

	var choice string
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("[%s] %s", direction, path)).
		Options(
			huh.NewOption("Apply", actionApplyValue),
			huh.NewOption("Skip", actionSkipValue),
		).
		Value(&choice).
		Run()

	if err != nil {
		return actionApplyValue
	}
	return choice
}

// isInteractive returns true if stdin is a terminal.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// Fallback prompts for non-interactive environments.

func promptToolActionFallback(toolName string) string {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprintf(os.Stdout, "Apply changes to %s? [y/n/d(iff)/s(kip)]: ", toolName)
		if !scanner.Scan() {
			return actionAbort
		}
		switch strings.TrimSpace(strings.ToLower(scanner.Text())) {
		case "y", "yes":
			return actionApplyValue
		case "n", "no":
			return actionAbort
		case "d", "diff":
			return "diff"
		case "s", "skip":
			return actionSkipValue
		default:
			fmt.Fprintln(os.Stdout, "Please enter y, n, d, or s.")
		}
	}
}

func promptConfirmationFallback(prompt string) bool {
	fmt.Fprintf(os.Stdout, "%s [y/N]: ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

func promptFileActionFallback(path, direction string) string {
	fmt.Fprintf(os.Stdout, "  [%s] %s — apply? [Y/n]: ", direction, path)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "n" || answer == "no" {
			return actionSkipValue
		}
	}
	return actionApplyValue
}

// promptSyncConflictFallback reads one answer per line from stdin, so a
// script can pipe in decisions. End of input skips the conflict: without a
// person to ask, nothing is overwritten.
func promptSyncConflictFallback(detail entities.ConflictDetail) entities.ConflictChoice {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(os.Stdout,
			"  [d]iff / keep [l]ocal / keep [r]emote / [s]kip / [L] local for all / [R] remote for all / [q]uit: ")
		if !scanner.Scan() {
			fmt.Fprintln(os.Stdout, "(no input: skipped)")
			return entities.ChoiceSkip
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer == "d" || answer == choiceShowDiff {
			showConflictDiffPlain(detail)
			continue
		}
		if choice, ok := fallbackAnswers[answer]; ok {
			return choice
		}
		fmt.Fprintln(os.Stdout, "  Please enter d, l, r, s, L, R, or q.")
	}
}

// fallbackAnswers maps the answers the non-interactive conflict prompt
// accepts to choices; an empty answer skips, like end of input.
//
//nolint:gochecknoglobals // compiled-in lookup table
var fallbackAnswers = map[string]entities.ConflictChoice{
	"l": entities.ChoiceKeepLocal, "local": entities.ChoiceKeepLocal,
	"r": entities.ChoiceKeepRemote, "remote": entities.ChoiceKeepRemote,
	"s": entities.ChoiceSkip, "skip": entities.ChoiceSkip, "": entities.ChoiceSkip,
	"L": entities.ChoiceKeepLocalAll,
	"R": entities.ChoiceKeepRemoteAll,
	"q": entities.ChoiceQuit, "quit": entities.ChoiceQuit,
}

// showConflictDiffPlain prints the diff without colors or paging, for the
// non-interactive fallback.
func showConflictDiffPlain(detail entities.ConflictDetail) {
	switch {
	case detail.Binary:
		fmt.Fprintln(os.Stdout, "  Binary content: no text diff is available.")
	case detail.Diff == "":
		fmt.Fprintln(os.Stdout, "  The two versions have the same text; only the executable permission differs.")
	default:
		fmt.Fprint(os.Stdout, detail.Diff)
	}
}
