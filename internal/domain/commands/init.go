package commands

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
// `aisync init`. Patterns match repo-relative paths under personal/<tool>/
// (same form used by .gitattributes). Anything matched here is age-encrypted
// before commit. The default set is deliberately over-cautious: we'd rather
// encrypt an innocuous file than silently commit a credential in plaintext.
// Users can tighten or remove patterns by editing this file.
const defaultAisyncEncrypt = `# aisync default encrypt patterns.
# Matched against repo-relative paths under personal/<tool>/. Anything matched
# is age-encrypted before being committed. The list is deliberately broad —
# it is safer to encrypt an innocuous file than to miss a credential.
# Remove or tighten patterns here as needed; the compiled deny-list still
# blocks outright-dangerous files regardless of this list.

# ---------- user memories (all supported AI tools) ----------
personal/**/memories/**

# ---------- local / device-specific settings ----------
personal/**/settings.local.json
personal/**/*.local.json
personal/**/local.settings.json

# ---------- MCP / agent-server configs (often carry API tokens) ----------
personal/**/mcp.json
personal/**/mcp.local.json

# ---------- environment files ----------
personal/**/.env
personal/**/.env.*
personal/**/.envrc

# ---------- private keys (any format) ----------
personal/**/*.key
personal/**/*.pem
personal/**/*.p12
personal/**/*.pfx
personal/**/*.jks
personal/**/*.keystore
personal/**/*.asc
personal/**/*.gpg
personal/**/secring.*
personal/**/id_rsa
personal/**/id_dsa
personal/**/id_ecdsa
personal/**/id_ed25519
personal/**/keys/**
personal/**/private_keys/**

# ---------- generic credentials / tokens ----------
personal/**/credentials
personal/**/credentials.*
personal/**/*.credentials
personal/**/*.token
personal/**/*.secret
personal/**/*.private
personal/**/secrets/**
personal/**/auth.json
personal/**/.netrc
personal/**/.pypirc
personal/**/.npmrc
personal/**/.dockerconfigjson

# ---------- session / cookie state ----------
personal/**/*.session
personal/**/*.cookies
personal/**/sessions/**
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

	// Populate the age identity and recipient list in memory BEFORE writing
	// config.yaml. Writing the recipient list and the config file in a single
	// Save eliminates the interrupt window where the repo could otherwise
	// land with `recipients: []` on disk and silently push plaintext secrets
	// on the next `aisync push`.
	if err := c.ensureAgeKeyAndRecipient(config); err != nil {
		return err
	}

	if err := c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
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

// buildDefaultConfig creates the default aifiles config with ONLY the tools
// that are actually installed on this device. Tools that are not detected
// are left out of the fresh config entirely (rather than included with
// `enabled: false`). This keeps the file small and focused on what the user
// actually uses. To enable an additional tool later, add it to config.yaml
// by hand or re-run `aisync init` on a machine where the tool is installed.
func (c *InitCommand) buildDefaultConfig() *entities.Config {
	detected := c.toolDetector.DetectInstalled(entities.DefaultTools())
	enabled := make(map[string]entities.Tool, len(detected))
	for name, tool := range detected {
		if tool.Enabled {
			enabled[name] = tool
		}
	}

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
		Tools:   enabled,
		Sources: []entities.Source{},
		Watch: entities.WatchConfig{
			PollingInterval: "30s",
			IgnoredPatterns: []string{"*.tmp", "*.swp"},
		},
	}
}

// ensureAgeKeyAndRecipient makes sure the fresh repo has a working age
// identity AND that its public key is registered as a recipient on the
// given config struct. It mutates config in memory only — callers are
// expected to persist the updated config themselves, which is how
// [InitCommand.executeCreate] keeps the config write atomic.
//
// There are two cases:
//
//  1. The identity file does not exist — generate a new keypair and set
//     config.Encryption.Recipients to the new public key.
//  2. The identity file already exists (e.g. carried over from a previous
//     aisync install, imported from 1Password, or shared across repos) —
//     derive the public key via [repositories.EncryptionService.ExportPublicKey]
//     and append it to the recipient list if it is not already there.
//
// Before this change, case 2 silently left Recipients empty, which caused
// `aisync push` to skip encryption entirely and commit memories /
// settings.local.json as plaintext.
func (c *InitCommand) ensureAgeKeyAndRecipient(config *entities.Config) error {
	identityPath := ExpandHome(config.Encryption.Identity)

	if _, statErr := os.Stat(identityPath); os.IsNotExist(statErr) {
		return c.generateAndRegisterNewKey(identityPath, config)
	} else if statErr != nil {
		return fmt.Errorf("failed to stat identity file %s: %w", identityPath, statErr)
	}

	return c.registerExistingKey(identityPath, config)
}

// generateAndRegisterNewKey creates a fresh age keypair at identityPath and
// sets the new public key as the sole recipient on the given config struct.
// It mutates config in memory only; the caller is expected to persist it.
func (c *InitCommand) generateAndRegisterNewKey(identityPath string, config *entities.Config) error {
	if err := os.MkdirAll(filepath.Dir(identityPath), 0700); err != nil {
		return fmt.Errorf("failed to create identity directory: %w", err)
	}

	publicKey, genErr := c.encryptionService.GenerateKey(identityPath)
	if genErr != nil {
		return fmt.Errorf("failed to generate age key: %w", genErr)
	}

	config.Encryption.Recipients = []string{publicKey}
	logger.Infof("generated age key pair at %s", identityPath)
	return nil
}

// registerExistingKey derives the public key from an existing identity file
// and appends it to the recipient list on the given config struct if it is
// not already present. It mutates config in memory only; the caller is
// expected to persist it.
func (c *InitCommand) registerExistingKey(identityPath string, config *entities.Config) error {
	publicKey, err := c.encryptionService.ExportPublicKey(identityPath)
	if err != nil {
		return fmt.Errorf("failed to derive public key from existing identity %s: %w", identityPath, err)
	}

	if slices.Contains(config.Encryption.Recipients, publicKey) {
		logger.Infof("reused existing age identity at %s (recipient already registered)", identityPath)
		return nil
	}

	config.Encryption.Recipients = append(config.Encryption.Recipients, publicKey)
	logger.Infof("reused existing age identity at %s and registered its public key as a recipient", identityPath)
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
