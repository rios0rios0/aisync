package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// InitCommand creates or clones an aifiles repository.
type InitCommand struct {
	configRepo        repositories.ConfigRepository
	stateRepo         repositories.StateRepository
	toolDetector      repositories.ToolDetector
	gitRepo           repositories.GitRepository
	encryptionService repositories.EncryptionService
}

// NewInitCommand creates a new InitCommand.
func NewInitCommand(
	configRepo repositories.ConfigRepository,
	stateRepo repositories.StateRepository,
	toolDetector repositories.ToolDetector,
	gitRepo repositories.GitRepository,
	encryptionService repositories.EncryptionService,
) *InitCommand {
	return &InitCommand{
		configRepo:        configRepo,
		stateRepo:         stateRepo,
		toolDetector:      toolDetector,
		gitRepo:           gitRepo,
		encryptionService: encryptionService,
	}
}

// Execute initializes a new aifiles repo at the given path. If a GitHub username
// is provided, it clones <user>/aifiles from GitHub. If a remoteURL is provided,
// it clones from that URL. Otherwise it creates a fresh local repo. The keyPath
// parameter allows importing an existing age identity during clone.
func (c *InitCommand) Execute(repoPath, githubUser, remoteURL, keyPath string) error {
	cloneURL := c.resolveCloneURL(githubUser, remoteURL)

	if cloneURL != "" {
		return c.executeClone(repoPath, cloneURL, keyPath)
	}

	return c.executeCreate(repoPath)
}

// resolveCloneURL determines the Git clone URL from the provided arguments.
// A --remote flag takes priority over a positional GitHub username.
func (c *InitCommand) resolveCloneURL(githubUser, remoteURL string) string {
	if remoteURL != "" {
		return remoteURL
	}
	if githubUser != "" {
		return fmt.Sprintf("git@github.com:%s/aifiles.git", githubUser)
	}
	return ""
}

// executeClone clones an existing aifiles repository and ensures the local state
// is initialised for this device. If keyPath is provided, the age identity is
// imported from that path into the configured identity location. Falls back to
// the AISYNC_KEY_FILE environment variable when keyPath is empty.
func (c *InitCommand) executeClone(repoPath, cloneURL, keyPath string) error {
	// Fall back to AISYNC_KEY_FILE env var when no explicit key path is given.
	if keyPath == "" {
		if envKey := os.Getenv("AISYNC_KEY_FILE"); envKey != "" {
			logger.Infof("using key file from AISYNC_KEY_FILE environment variable: %s", envKey)
			keyPath = envKey
		}
	}

	logger.Infof("cloning aifiles repo from %s into %s", cloneURL, repoPath)

	if err := os.MkdirAll(filepath.Dir(repoPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if err := c.gitRepo.Clone(cloneURL, repoPath, "main"); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Import age identity if a key path was provided
	if keyPath != "" {
		configPath := filepath.Join(repoPath, "config.yaml")
		config, loadErr := c.configRepo.Load(configPath)
		if loadErr != nil {
			logger.Warnf("failed to load config for key import: %v", loadErr)
		} else {
			destPath := ExpandHome(config.Encryption.Identity)
			if err := os.MkdirAll(filepath.Dir(destPath), 0700); err != nil {
				return fmt.Errorf("failed to create identity directory: %w", err)
			}
			if err := c.encryptionService.ImportKey(keyPath, destPath); err != nil {
				return fmt.Errorf("failed to import age identity: %w", err)
			}
			logger.Infof("imported age identity from %s to %s", keyPath, destPath)
		}
	}

	// Detect installed AI tools and update config for this device
	configPath := filepath.Join(repoPath, "config.yaml")
	if config, loadErr := c.configRepo.Load(configPath); loadErr == nil {
		detected := c.toolDetector.DetectInstalled(entities.DefaultTools())
		if config.Tools == nil {
			config.Tools = make(map[string]entities.Tool)
		}
		for name, tool := range detected {
			if _, exists := config.Tools[name]; !exists {
				config.Tools[name] = tool
			}
		}
		if saveErr := c.configRepo.Save(configPath, config); saveErr != nil {
			logger.Warnf("failed to update config with detected tools: %v", saveErr)
		}
	}

	// Ensure state file exists for this device
	hostname, _ := os.Hostname()
	if !c.stateRepo.Exists(repoPath) {
		state := entities.NewState(hostname)
		if err := c.stateRepo.Save(repoPath, state); err != nil {
			return fmt.Errorf("failed to write state: %w", err)
		}
	}

	logger.Info("aifiles repo cloned successfully")
	fmt.Printf("\nSync repo cloned to %s\n\n", repoPath)
	fmt.Println("Next steps:")
	fmt.Println("  aisync pull")
	fmt.Println()

	return nil
}

// executeCreate scaffolds a brand-new aifiles repository with default config,
// directory structure, and an initialised Git repo.
func (c *InitCommand) executeCreate(repoPath string) error {
	configPath := filepath.Join(repoPath, "config.yaml")
	if c.configRepo.Exists(configPath) {
		return fmt.Errorf("aifiles repo already exists at %s", repoPath)
	}

	logger.Infof("initializing aifiles repo at %s", repoPath)

	dirs := []string{
		"shared/claude/rules", "shared/claude/commands", "shared/claude/agents",
		"shared/claude/hooks", "shared/claude/skills",
		"shared/cursor/rules", "shared/cursor/skills",
		"shared/copilot/instructions",
		"shared/codex/rules",
		"personal/claude/rules", "personal/claude/commands", "personal/claude/agents",
		"personal/claude/hooks", "personal/claude/memories",
		"personal/cursor/rules",
		"personal/copilot/instructions",
		"personal/codex/rules",
		".aisync",
	}

	for _, dir := range dirs {
		fullPath := filepath.Join(repoPath, dir)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", fullPath, err)
		}
		gitkeep := filepath.Join(fullPath, ".gitkeep")
		if err := os.WriteFile(gitkeep, []byte{}, 0644); err != nil {
			return fmt.Errorf("failed to create .gitkeep in %s: %w", fullPath, err)
		}
	}

	detected := c.toolDetector.DetectInstalled(entities.DefaultTools())
	config := &entities.Config{
		Sync: entities.SyncConfig{
			Remote:       "",
			Branch:       "main",
			AutoPush:     false,
			Debounce:     "60s",
			CommitPrefix: "sync",
		},
		Encryption: entities.EncryptionConfig{
			Identity:   "~/.config/aisync/key.txt",
			Recipients: []string{},
		},
		Tools:   detected,
		Sources: []entities.Source{},
		Watch: entities.WatchConfig{
			PollingInterval: "30s",
			IgnoredPatterns: []string{"*.tmp", "*.swp"},
		},
	}

	if err := c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Auto-generate age key pair if the identity file does not exist
	identityPath := ExpandHome(config.Encryption.Identity)
	if _, statErr := os.Stat(identityPath); os.IsNotExist(statErr) {
		if err := os.MkdirAll(filepath.Dir(identityPath), 0700); err != nil {
			return fmt.Errorf("failed to create identity directory: %w", err)
		}

		publicKey, genErr := c.encryptionService.GenerateKey(identityPath)
		if genErr != nil {
			return fmt.Errorf("failed to generate age key: %w", genErr)
		}

		config.Encryption.Recipients = []string{publicKey}
		if err := c.configRepo.Save(configPath, config); err != nil {
			return fmt.Errorf("failed to update config with recipient: %w", err)
		}
		logger.Infof("generated age key pair at %s", identityPath)
	}

	hostname, _ := os.Hostname()
	state := entities.NewState(hostname)
	if err := c.stateRepo.Save(repoPath, state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	// Create .gitattributes: enforce LF line endings and configure encryption filter.
	gitattributesContent := "* text=auto eol=lf\npersonal/*/memories/** filter=aisync-crypt diff=aisync-crypt\npersonal/*/settings.local.json filter=aisync-crypt diff=aisync-crypt\n"
	gitattributesPath := filepath.Join(repoPath, ".gitattributes")
	if err := os.WriteFile(gitattributesPath, []byte(gitattributesContent), 0644); err != nil {
		return fmt.Errorf("failed to create .gitattributes: %w", err)
	}

	// Initialize as a Git repository
	if err := c.gitRepo.Init(repoPath); err != nil {
		return fmt.Errorf("failed to initialize git repository: %w", err)
	}

	// Configure clean/smudge filter for transparent encryption.
	filterConfigs := map[string]string{
		"filter.aisync-crypt.clean":    "aisync _clean",
		"filter.aisync-crypt.smudge":   "aisync _smudge",
		"filter.aisync-crypt.required": "true",
	}
	for key, value := range filterConfigs {
		if err := c.gitRepo.SetConfig(key, value); err != nil {
			logger.Warnf("failed to set %s: %v", key, err)
		}
	}

	// Offer to set up a remote for cross-device sync
	fmt.Println("Tip: create a repo with \"gh repo create aifiles --private\" then paste the URL below")
	fmt.Print("Remote Git URL (leave empty to skip): ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		if remoteURL := strings.TrimSpace(scanner.Text()); remoteURL != "" {
			if err := c.gitRepo.AddRemote("origin", remoteURL); err != nil {
				logger.Warnf("failed to add remote: %v", err)
			} else {
				logger.Infof("added remote 'origin' -> %s", remoteURL)
			}
		}
	}

	logger.Info("aifiles repo initialized successfully")
	fmt.Printf("\nSync repo initialized at %s\n\n", repoPath)
	fmt.Println("Next steps:")
	fmt.Println("  aisync source add <name> --repo <owner/repo> --branch <branch>")
	fmt.Println("  aisync pull")
	fmt.Println()
	fmt.Println("Browse curated sources at https://github.com/rios0rios0/guide")

	return nil
}
