package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

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
			huh.NewOption("Apply", "apply"),
			huh.NewOption("Skip", "skip"),
			huh.NewOption("Show diff", "diff"),
			huh.NewOption("Abort", "abort"),
		).
		Value(&choice).
		Run()

	if err != nil {
		return "abort"
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
		return "skip"
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
			huh.NewOption("Apply", "apply"),
			huh.NewOption("Skip", "skip"),
		).
		Value(&choice).
		Run()

	if err != nil {
		return "apply"
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
		fmt.Printf("Apply changes to %s? [y/n/d(iff)/s(kip)]: ", toolName)
		if !scanner.Scan() {
			return "abort"
		}
		switch strings.TrimSpace(strings.ToLower(scanner.Text())) {
		case "y", "yes":
			return "apply"
		case "n", "no":
			return "abort"
		case "d", "diff":
			return "diff"
		case "s", "skip":
			return "skip"
		default:
			fmt.Println("Please enter y, n, d, or s.")
		}
	}
}

func promptConfirmationFallback(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

func promptFileActionFallback(path, direction string) string {
	fmt.Printf("  [%s] %s — apply? [Y/n]: ", direction, path)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "n" || answer == "no" {
			return "skip"
		}
	}
	return "apply"
}

func promptConflictFallback(path, remoteDevice string) string {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("\nConflict: %s\n", path)
	fmt.Printf("  Local version differs from incoming (from device '%s')\n", remoteDevice)
	for {
		fmt.Print("  [l]ocal / [r]emote / [s]kip: ")
		if !scanner.Scan() {
			return "skip"
		}
		switch strings.TrimSpace(strings.ToLower(scanner.Text())) {
		case "l", "local":
			return "local"
		case "r", "remote":
			return "remote"
		case "s", "skip":
			return "skip"
		default:
			fmt.Println("  Please enter l, r, or s.")
		}
	}
}
