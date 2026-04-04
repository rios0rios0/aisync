package commands

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// DoctorCommand diagnoses common issues with the aisync setup.
type DoctorCommand struct {
	configRepo        repositories.ConfigRepository
	stateRepo         repositories.StateRepository
	encryptionService repositories.EncryptionService
	toolDetector      repositories.ToolDetector
	formatter         entities.Formatter
}

// NewDoctorCommand creates a new DoctorCommand.
func NewDoctorCommand(
	configRepo repositories.ConfigRepository,
	stateRepo repositories.StateRepository,
	encryptionService repositories.EncryptionService,
	toolDetector repositories.ToolDetector,
	formatter entities.Formatter,
) *DoctorCommand {
	return &DoctorCommand{
		configRepo:        configRepo,
		stateRepo:         stateRepo,
		encryptionService: encryptionService,
		toolDetector:      toolDetector,
		formatter:         formatter,
	}
}

// Execute runs all diagnostic checks and prints a pass/fail table.
func (c *DoctorCommand) Execute(configPath, repoPath string) error {
	fmt.Println("Running diagnostics...")
	fmt.Println()

	checks := []struct {
		name string
		fn   func() (string, bool)
	}{
		{"Config file", func() (string, bool) { return c.checkConfig(configPath) }},
		{"Sync repo", func() (string, bool) { return c.checkRepo(repoPath) }},
		{"State file", func() (string, bool) { return c.checkState(repoPath) }},
		{"Age key", func() (string, bool) { return c.checkAgeKey(configPath) }},
		{"AI tools", func() (string, bool) { return c.checkTools() }},
		{"External sources", func() (string, bool) { return c.checkSources(configPath) }},
	}

	allPassed := true
	for _, check := range checks {
		detail, passed := check.fn()
		tag := c.formatter.StatusTag(passed)
		if !passed {
			allPassed = false
		}
		fmt.Printf("  %s %-20s %s\n", tag, check.name, detail)
	}

	fmt.Println()
	if allPassed {
		fmt.Println("All checks passed.")
	} else {
		fmt.Println("Some checks failed. See details above.")
	}

	return nil
}

func (c *DoctorCommand) checkConfig(configPath string) (string, bool) {
	if !c.configRepo.Exists(configPath) {
		return "not found at " + configPath, false
	}
	_, err := c.configRepo.Load(configPath)
	if err != nil {
		return "exists but invalid: " + err.Error(), false
	}
	return configPath, true
}

func (c *DoctorCommand) checkRepo(repoPath string) (string, bool) {
	info, err := os.Stat(repoPath)
	if err != nil {
		return "not found at " + repoPath, false
	}
	if !info.IsDir() {
		return "exists but is not a directory", false
	}
	gitDir := repoPath + "/.git"
	if _, err := os.Stat(gitDir); err != nil {
		return "exists but is not a git repository", false
	}
	return repoPath, true
}

func (c *DoctorCommand) checkState(repoPath string) (string, bool) {
	if !c.stateRepo.Exists(repoPath) {
		return "not found (run 'aisync init')", false
	}
	state, err := c.stateRepo.Load(repoPath)
	if err != nil {
		return "exists but invalid: " + err.Error(), false
	}
	return fmt.Sprintf("%d device(s) registered", len(state.Devices)), true
}

func (c *DoctorCommand) checkAgeKey(configPath string) (string, bool) {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return "cannot load config", false
	}

	identityPath := ExpandHome(config.Encryption.Identity)
	if _, err := os.Stat(identityPath); err != nil {
		return "identity file not found at " + identityPath + " (run 'aisync key generate')", false
	}

	pubKey, err := c.encryptionService.ExportPublicKey(identityPath)
	if err != nil {
		return "identity file exists but is invalid: " + err.Error(), false
	}

	return pubKey[:20] + "...", true
}

func (c *DoctorCommand) checkTools() (string, bool) {
	detected := c.toolDetector.DetectInstalled(entities.DefaultTools())
	count := 0
	for _, t := range detected {
		if t.Enabled {
			count++
		}
	}
	if count == 0 {
		return "no AI tools detected", false
	}
	return fmt.Sprintf("%d tool(s) detected", count), true
}

func (c *DoctorCommand) checkSources(configPath string) (string, bool) {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return "cannot load config", false
	}

	if len(config.Sources) == 0 {
		return "no sources configured", false
	}

	client := &http.Client{Timeout: 10 * time.Second}
	reachable := 0
	for _, source := range config.Sources {
		url := source.TarballURL()
		req, _ := http.NewRequest(http.MethodHead, url, nil)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			reachable++
			resp.Body.Close()
		}
	}

	return fmt.Sprintf("%d/%d source(s) reachable", reachable, len(config.Sources)), reachable == len(config.Sources)
}
