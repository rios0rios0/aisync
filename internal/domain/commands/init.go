package commands

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// defaultAisyncIgnore is the starter content written to .aisyncignore by
// `aisync init`. These are basename and simple-glob patterns for files that
// users almost never want to sync across devices. Structural/directory-level
// blocking (transcripts, runtime state, backups) lives in the compiled-in
// deny-list in entities/denylist.go — that deny-list cannot be overridden,
// whereas this file can.
const defaultAisyncIgnore = `# aisync default ignore — user-overridable basename patterns.
# For structural directory exclusions (transcripts, runtime state, caches,
# backups) see the compiled-in deny-list in aisync. That list cannot be
# overridden and ships updated with every aisync release.

# editor / OS junk
*.tmp
*.swp
*.bak
*.old
*.orig
*.log
*.backup
*.backup.*
.DS_Store
Thumbs.db

# AI-assistant planning documents.
# Plans are generated from conversation context and frequently contain
# internal repo paths, customer/project names, and other NDA-sensitive
# strings. Comment out the lines below if you want to sync them anyway.
plans/
`

// defaultAisyncEncrypt is the starter content written to .aisyncencrypt by
// `aisync init`. Patterns are matched against the repo-relative path
// personal/<tool>/<file> so they agree with .gitattributes and the secret
// scanner. Anything matched here is age-encrypted before being committed.
const defaultAisyncEncrypt = `# aisync default encrypt patterns.
# Patterns are matched against the repo-relative path under personal/<tool>/
# (same form used by .gitattributes). Anything matched here is age-encrypted
# before being committed. Add your own patterns for any file that may contain
# secrets, credentials, or NDA-protected content.

# Claude — user memories and local overrides.
personal/claude/memories/**
personal/claude/settings.local.json
personal/claude/*.local.json
personal/claude/keys/**

# Cursor — user memories and local overrides.
personal/cursor/memories/**
personal/cursor/settings.local.json

# Codex — user memories.
personal/codex/memories/**

# Generic — anything the user explicitly marks private.
personal/**/secrets/**
personal/**/*.secret
personal/**/*.private
personal/**/.env
personal/**/.env.*
`

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
	keyPath = c.resolveKeyPath(keyPath)

	logger.Infof("cloning aifiles repo from %s into %s", cloneURL, repoPath)

	if err := os.MkdirAll(filepath.Dir(repoPath), 0700); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if err := c.gitRepo.Clone(cloneURL, repoPath, "main"); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	if keyPath != "" {
		if err := c.importAgeKey(repoPath, keyPath); err != nil {
			return err
		}
	}

	c.detectAndUpdateTools(repoPath)
	if err := c.ensureStateExists(repoPath); err != nil {
		return err
	}

	// Upgrade legacy cloned repos that predate default ignore/encrypt files.
	// Never overwrite user-customised content; just fill in missing files.
	if err := c.writeDefaultIgnoreIfMissing(repoPath); err != nil {
		logger.Warnf("failed to write default .aisyncignore: %v", err)
	}
	if err := c.writeDefaultEncryptIfMissing(repoPath); err != nil {
		logger.Warnf("failed to write default .aisyncencrypt: %v", err)
	}

	logger.Info("aifiles repo cloned successfully")
	fmt.Fprintf(os.Stdout, "\nSync repo cloned to %s\n\n", repoPath)
	fmt.Fprintln(os.Stdout, "Next steps:")
	fmt.Fprintln(os.Stdout, "  aisync pull")
	fmt.Fprintln(os.Stdout)

	return nil
}

// resolveKeyPath falls back to the AISYNC_KEY_FILE env var when no explicit key
// path is given.
func (c *InitCommand) resolveKeyPath(keyPath string) string {
	if keyPath == "" {
		if envKey := os.Getenv("AISYNC_KEY_FILE"); envKey != "" {
			logger.Infof("using key file from AISYNC_KEY_FILE environment variable: %s", envKey)
			return envKey
		}
	}
	return keyPath
}

// importAgeKey loads the config to determine the identity destination and imports
// the age key from keyPath.
func (c *InitCommand) importAgeKey(repoPath, keyPath string) error {
	configPath := filepath.Join(repoPath, "config.yaml")
	config, loadErr := c.configRepo.Load(configPath)
	if loadErr != nil {
		logger.Warnf("failed to load config for key import: %v", loadErr)
		return nil
	}

	destPath := ExpandHome(config.Encryption.Identity)
	if err := os.MkdirAll(filepath.Dir(destPath), 0700); err != nil {
		return fmt.Errorf("failed to create identity directory: %w", err)
	}
	if err := c.encryptionService.ImportKey(keyPath, destPath); err != nil {
		return fmt.Errorf("failed to import age identity: %w", err)
	}
	logger.Infof("imported age identity from %s to %s", keyPath, destPath)
	return nil
}

// detectAndUpdateTools detects installed AI tools and merges them into the config.
func (c *InitCommand) detectAndUpdateTools(repoPath string) {
	configPath := filepath.Join(repoPath, "config.yaml")
	config, loadErr := c.configRepo.Load(configPath)
	if loadErr != nil {
		return
	}

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

// ensureStateExists creates the state file for this device if it does not exist.
func (c *InitCommand) ensureStateExists(repoPath string) error {
	if c.stateRepo.Exists(repoPath) {
		return nil
	}

	hostname, _ := os.Hostname()
	state := entities.NewState(hostname)
	if err := c.stateRepo.Save(repoPath, state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}
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

	if err := c.scaffoldDirectories(repoPath); err != nil {
		return err
	}

	if err := c.writeDefaultIgnoreIfMissing(repoPath); err != nil {
		return fmt.Errorf("failed to write default .aisyncignore: %w", err)
	}
	if err := c.writeDefaultEncryptIfMissing(repoPath); err != nil {
		return fmt.Errorf("failed to write default .aisyncencrypt: %w", err)
	}

	config := c.buildDefaultConfig()
	if err := c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	if err := c.generateAgeKeyIfMissing(configPath, config); err != nil {
		return err
	}

	hostname, _ := os.Hostname()
	state := entities.NewState(hostname)
	if err := c.stateRepo.Save(repoPath, state); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	if err := c.initGitRepo(repoPath); err != nil {
		return err
	}

	c.promptRemoteSetup()

	logger.Info("aifiles repo initialized successfully")
	fmt.Fprintf(os.Stdout, "\nSync repo initialized at %s\n\n", repoPath)
	fmt.Fprintln(os.Stdout, "Next steps:")
	fmt.Fprintln(os.Stdout, "  aisync source add <name> --repo <owner/repo> --branch <branch>")
	fmt.Fprintln(os.Stdout, "  aisync pull")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Browse curated sources at https://github.com/rios0rios0/guide")

	return nil
}

// writeDefaultIgnoreIfMissing writes the default .aisyncignore content to the
// repo root if no .aisyncignore file exists there. Existing files are left
// untouched so user customisations survive re-runs of init.
func (c *InitCommand) writeDefaultIgnoreIfMissing(repoPath string) error {
	return writeFileIfMissing(filepath.Join(repoPath, ".aisyncignore"), []byte(defaultAisyncIgnore))
}

// writeDefaultEncryptIfMissing writes the default .aisyncencrypt content to
// the repo root if no .aisyncencrypt file exists there. Existing files are
// left untouched so user customisations survive re-runs of init.
func (c *InitCommand) writeDefaultEncryptIfMissing(repoPath string) error {
	return writeFileIfMissing(filepath.Join(repoPath, ".aisyncencrypt"), []byte(defaultAisyncEncrypt))
}

// writeFileIfMissing writes content to path only if path does not already
// exist. If the file exists, nothing is changed and nil is returned.
func writeFileIfMissing(path string, content []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", path, err)
	}
	return os.WriteFile(path, content, 0600)
}

// scaffoldDirectories creates the minimal aifiles directory layout: the two
// namespace roots (personal/ and shared/) plus the .aisync/ device-state
// directory. Tool-specific subdirectories (e.g. personal/claude/rules/) are
// deliberately NOT pre-created — they emerge organically when push/pull
// calls [os.MkdirAll] for the tools that are actually installed on each
// device. This keeps fresh repos from being polluted with empty placeholder
// directories for AI tools the user does not use.
func (c *InitCommand) scaffoldDirectories(repoPath string) error {
	dirs := []string{
		"personal",
		"shared",
		".aisync",
	}

	for _, dir := range dirs {
		fullPath := filepath.Join(repoPath, dir)
		if err := os.MkdirAll(fullPath, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", fullPath, err)
		}
		gitkeep := filepath.Join(fullPath, ".gitkeep")
		if err := os.WriteFile(gitkeep, []byte{}, 0600); err != nil {
			return fmt.Errorf("failed to create .gitkeep in %s: %w", fullPath, err)
		}
	}
	return nil
}

// buildDefaultConfig creates the default aifiles config with detected tools.
func (c *InitCommand) buildDefaultConfig() *entities.Config {
	detected := c.toolDetector.DetectInstalled(entities.DefaultTools())
	return &entities.Config{
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
}

// generateAgeKeyIfMissing auto-generates an age key pair when the identity file
// does not exist and updates the config with the public key as a recipient.
func (c *InitCommand) generateAgeKeyIfMissing(configPath string, config *entities.Config) error {
	identityPath := ExpandHome(config.Encryption.Identity)
	if _, statErr := os.Stat(identityPath); !os.IsNotExist(statErr) {
		return nil
	}

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
	return nil
}

// initGitRepo initializes the Git repository, creates .gitattributes, and
// configures the clean/smudge encryption filter.
func (c *InitCommand) initGitRepo(repoPath string) error {
	gitattributesContent := "* text=auto eol=lf\npersonal/*/memories/** filter=aisync-crypt diff=aisync-crypt\npersonal/*/settings.local.json filter=aisync-crypt diff=aisync-crypt\n"
	gitattributesPath := filepath.Join(repoPath, ".gitattributes")
	if err := os.WriteFile(gitattributesPath, []byte(gitattributesContent), 0600); err != nil {
		return fmt.Errorf("failed to create .gitattributes: %w", err)
	}

	if err := c.gitRepo.Init(repoPath); err != nil {
		return fmt.Errorf("failed to initialize git repository: %w", err)
	}

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
	return nil
}

// promptRemoteSetup offers the user to configure a remote for cross-device sync.
func (c *InitCommand) promptRemoteSetup() {
	fmt.Fprintln(os.Stdout, "Tip: create a repo with \"gh repo create aifiles --private\" then paste the URL below")
	fmt.Fprint(os.Stdout, "Remote Git URL (leave empty to skip): ")
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
}
