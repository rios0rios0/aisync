package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
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

// PromptConflictResolution asks how to resolve a file conflict.
func (s *HuhPromptService) PromptConflictResolution(path, remoteDevice string) string {
	if !isInteractive() {
		return promptConflictFallback(path, remoteDevice)
	}

	var choice string
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Conflict: %s (from device '%s')", path, remoteDevice)).
		Options(
			huh.NewOption("Keep local", "local"),
			huh.NewOption("Accept remote", "remote"),
			huh.NewOption("Skip (preserve conflict file)", "skip"),
		).
		Value(&choice).
		Run()

	if err != nil {
		return actionSkipValue
	}
	return choice
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

func promptConflictFallback(path, remoteDevice string) string {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprintf(os.Stdout, "\nConflict: %s\n", path)
	fmt.Fprintf(os.Stdout, "  Local version differs from incoming (from device '%s')\n", remoteDevice)
	for {
		fmt.Fprint(os.Stdout, "  [l]ocal / [r]emote / [s]kip: ")
		if !scanner.Scan() {
			return actionSkipValue
		}
		switch strings.TrimSpace(strings.ToLower(scanner.Text())) {
		case "l", "local":
			return "local"
		case "r", "remote":
			return "remote"
		case "s", "skip":
			return actionSkipValue
		default:
			fmt.Fprintln(os.Stdout, "  Please enter l, r, or s.")
		}
	}
}
