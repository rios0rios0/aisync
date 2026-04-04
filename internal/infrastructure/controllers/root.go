package controllers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/rios0rios0/aisync/internal/domain/commands"
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	infraRepos "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
	"github.com/rios0rios0/aisync/internal/infrastructure/services"
	"github.com/rios0rios0/aisync/internal/infrastructure/ui"

	"github.com/rios0rios0/cliforge/pkg/selfupdate"
)

const (
	pollingWatchInterval = 30 * time.Second
	defaultDebounce      = 60 * time.Second
	deviceRenameArgs     = 2
)

// DefaultRepoPath returns the default aifiles repo location.
// On Windows it uses %APPDATA%\aisync\repo; on other platforms ~/.config/aisync/repo.
func DefaultRepoPath() string {
	return filepath.Join(defaultConfigDir(), "repo")
}

// DefaultConfigPath returns the default config.yaml location inside the repo.
func DefaultConfigPath() string {
	return filepath.Join(DefaultRepoPath(), "config.yaml")
}

// gitRepoProxy delegates all GitRepository calls to an underlying implementation
// that can be swapped at runtime (e.g., when --use-system-git is set).
type gitRepoProxy struct {
	impl repositories.GitRepository
}

func (p *gitRepoProxy) Clone(url, dir, branch string) error { return p.impl.Clone(url, dir, branch) }
func (p *gitRepoProxy) Init(dir string) error               { return p.impl.Init(dir) }
func (p *gitRepoProxy) Open(dir string) error               { return p.impl.Open(dir) }
func (p *gitRepoProxy) Pull() error                         { return p.impl.Pull() }
func (p *gitRepoProxy) CommitAll(message string) error      { return p.impl.CommitAll(message) }
func (p *gitRepoProxy) Push() error                         { return p.impl.Push() }
func (p *gitRepoProxy) IsClean() (bool, error)              { return p.impl.IsClean() }
func (p *gitRepoProxy) HasRemote() bool                     { return p.impl.HasRemote() }
func (p *gitRepoProxy) AddRemote(name, url string) error    { return p.impl.AddRemote(name, url) }
func (p *gitRepoProxy) SetConfig(key, value string) error   { return p.impl.SetConfig(key, value) }

// NewExecGitRepository re-exports the infrastructure constructor so that main.go
// can create it when --use-system-git is set.
var NewExecGitRepository = infraRepos.NewExecGitRepository //nolint:gochecknoglobals // re-exported constructor for main.go

// NewRootCommand builds the root cobra command with all subcommands. It returns
// the root command and a function that swaps the git implementation to the
// system git binary (called from PersistentPreRun when --use-system-git is set).
func NewRootCommand(version string) (*cobra.Command, func(repositories.GitRepository)) {
	// Git repo wrapped in a proxy so --use-system-git can swap the implementation
	// after flag parsing but before any command runs.
	gitProxy := &gitRepoProxy{impl: infraRepos.NewGoGitRepository()}

	// Infrastructure
	configRepo := infraRepos.NewYAMLConfigRepository()
	sourceRepo := infraRepos.NewHTTPSourceRepository()
	manifestRepo := infraRepos.NewJSONManifestRepository()
	stateRepo := infraRepos.NewJSONStateRepository()
	journalRepo := infraRepos.NewJSONJournalRepository(defaultConfigDir())
	toolDetector := services.NewFSToolDetector()
	encryptionSvc := services.NewAgeEncryptionService()
	diffSvc := services.NewFSDiffService()
	secretScanner := services.NewRegexSecretScanner()
	conflictDetector := services.NewConflictDetector()

	// Watch service: fsnotify on desktop, polling on Android
	var watchSvc repositories.WatchService
	if runtime.GOOS == "android" || os.Getenv("ANDROID_ROOT") != "" {
		watchSvc = services.NewPollingWatchService(pollingWatchInterval)
	} else {
		watchSvc = services.NewFSNotifyWatchService()
	}

	// Mergers + Formatter
	hooksMerger := services.NewHooksMerger(nil) // excludes loaded per-pull from config
	settingsMerger := services.NewSettingsMerger()
	sectionMerger := services.NewSectionMerger()
	atomicApplySvc := services.NewAtomicApplyService(journalRepo, defaultConfigDir())
	formatter := ui.NewLipglossFormatter()

	// Domain commands
	initCmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitProxy, encryptionSvc)
	sourceCmd := commands.NewSourceCommand(configRepo, sourceRepo)
	promptSvc := ui.NewHuhPromptService()
	pullCmd := commands.NewPullCommand(
		configRepo, stateRepo, sourceRepo, manifestRepo,
		gitProxy, encryptionSvc, conflictDetector,
		hooksMerger, settingsMerger, sectionMerger,
		atomicApplySvc, promptSvc,
	)
	pushCmd := commands.NewPushCommand(configRepo, stateRepo, gitProxy, encryptionSvc, manifestRepo, secretScanner)
	syncCmd := commands.NewSyncCommand(pullCmd, pushCmd)
	statusCmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo)
	diffViewer := ui.NewBubbleteaDiffViewer()
	diffCmd := commands.NewDiffCommand(configRepo, sourceRepo, diffSvc, formatter, diffViewer)
	keyCmd := commands.NewKeyCommand(configRepo, encryptionSvc)
	deviceCmd := commands.NewDeviceCommand(stateRepo)
	doctorCmd := commands.NewDoctorCommand(configRepo, stateRepo, encryptionSvc, toolDetector, gitProxy, formatter)
	migrateCmd := commands.NewMigrateCommand(configRepo, manifestRepo, sourceRepo)
	watchCmd := commands.NewWatchCommand(configRepo, watchSvc, pushCmd)

	//nolint:exhaustruct // cobra command does not require all fields
	root := &cobra.Command{
		Use:   "aisync",
		Short: "Sync AI coding assistant configurations across devices",
		Long: `aisync manages AI coding assistant configurations (rules, agents, commands,
hooks, skills, memories, settings) across multiple devices and multiple AI tools.

It pulls shared rules from public external sources, syncs personal configurations
via a Git repository, and encrypts sensitive data with age.

Quick start:
  aisync init
  aisync source add guide --source-repo rios0rios0/guide --branch generated
  aisync pull`,
	}

	root.PersistentFlags().String("repo", DefaultRepoPath(), "path to the aifiles sync repository")
	root.PersistentFlags().String("config", "", "path to config.yaml (default: <repo>/config.yaml)")
	root.PersistentFlags().BoolP("verbose", "v", false, "enable verbose logging")
	root.PersistentFlags().Bool("quiet", false, "suppress non-error output")
	root.PersistentFlags().Bool("force", false, "skip confirmation prompts")
	root.PersistentFlags().Bool("use-system-git", false, "use system git binary instead of built-in go-git")

	root.AddCommand(newFilterSubcmds(encryptionSvc)...)
	root.AddCommand(
		newInitSubcmd(initCmd),
		newSourceSubcmd(sourceCmd),
		newPullSubcmd(pullCmd),
		newPushSubcmd(pushCmd),
		newSyncSubcmd(syncCmd),
		newDiffSubcmd(diffCmd),
		newWatchSubcmd(watchCmd),
		newStatusSubcmd(statusCmd),
		newKeySubcmd(keyCmd),
		newDeviceSubcmd(deviceCmd),
		newDoctorSubcmd(doctorCmd),
		newMigrateSubcmd(migrateCmd),
		newSelfUpdateSubcmd(version),
		newVersionSubcmd(version),
	)

	setGitImpl := func(impl repositories.GitRepository) {
		gitProxy.impl = impl
	}
	return root, setGitImpl
}

func defaultConfigDir() string {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "aisync")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aisync")
}

func resolveConfigPath(cmd *cobra.Command) string {
	cfgFlag, _ := cmd.Flags().GetString("config")
	if cfgFlag != "" {
		return cfgFlag
	}
	return filepath.Join(resolveRepoPath(cmd), "config.yaml")
}

func resolveRepoPath(cmd *cobra.Command) string {
	repoFlag, _ := cmd.Flags().GetString("repo")
	if repoFlag != "" {
		return repoFlag
	}
	return DefaultRepoPath()
}

// --- Subcommands ---

func newInitSubcmd(initCmd *commands.InitCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	cmd := &cobra.Command{
		Use:   "init [github-user]",
		Short: "Initialize a new aifiles repository or clone an existing one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath := resolveRepoPath(cmd)
			remoteURL, _ := cmd.Flags().GetString("remote-url")
			githubUser := ""
			if len(args) > 0 {
				githubUser = args[0]
			}
			keyPath, _ := cmd.Flags().GetString("key")
			return initCmd.Execute(repoPath, githubUser, remoteURL, keyPath)
		},
	}
	cmd.Flags().String("remote-url", "", "full Git URL to clone (overrides github-user shorthand)")
	cmd.Flags().String("key", "", "path to age identity file")
	return cmd
}

func newSourceSubcmd(sourceCmd *commands.SourceCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	parent := &cobra.Command{
		Use:   "source",
		Short: "Manage external sources",
	}

	//nolint:exhaustruct // cobra command does not require all fields
	addCmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Add an external source",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fromURL, _ := cmd.Flags().GetString("from-url")

			if fromURL != "" {
				return sourceCmd.AddFromURL(resolveConfigPath(cmd), fromURL)
			}

			if len(args) == 0 {
				return errors.New("name is required when --from-url is not specified")
			}

			repo, _ := cmd.Flags().GetString("source-repo")
			branch, _ := cmd.Flags().GetString("branch")
			ref, _ := cmd.Flags().GetString("ref")
			pathFilter, _ := cmd.Flags().GetString("path")

			var mappings []entities.SourceMapping
			if pathFilter != "" {
				mappings = inferMappingsForPath(pathFilter)
			} else {
				mappings = inferMappings()
			}

			source := entities.Source{
				Name:     args[0],
				Repo:     repo,
				Branch:   branch,
				Ref:      ref,
				Refresh:  "168h",
				Mappings: mappings,
			}
			return sourceCmd.Add(resolveConfigPath(cmd), source)
		},
	}
	addCmd.Flags().String("source-repo", "", "repository in owner/repo format")
	addCmd.Flags().String("branch", "main", "branch to pull from")
	addCmd.Flags().String("ref", "", "pin to a specific tag or SHA")
	addCmd.Flags().String("path", "", "subdirectory within the source repo to restrict mappings")
	addCmd.Flags().String("from-url", "", "import source definition from a remote YAML URL")

	//nolint:exhaustruct // cobra command does not require all fields
	removeCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an external source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sourceCmd.Remove(resolveConfigPath(cmd), args[0])
		},
	}

	//nolint:exhaustruct // cobra command does not require all fields
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List configured external sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return sourceCmd.List(resolveConfigPath(cmd))
		},
	}

	//nolint:exhaustruct // cobra command does not require all fields
	updateCmd := &cobra.Command{
		Use:   "update [name]",
		Short: "Re-fetch external sources (ignoring refresh interval)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return sourceCmd.Update(resolveConfigPath(cmd), resolveRepoPath(cmd), name)
		},
	}

	//nolint:exhaustruct // cobra command does not require all fields
	pinCmd := &cobra.Command{
		Use:   "pin <name>",
		Short: "Pin a source to a specific tag or SHA",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, _ := cmd.Flags().GetString("ref")
			return sourceCmd.Pin(resolveConfigPath(cmd), args[0], ref)
		},
	}
	pinCmd.Flags().String("ref", "", "tag or SHA to pin to (required)")
	_ = pinCmd.MarkFlagRequired("ref")

	parent.AddCommand(addCmd, removeCmd, listCmd, updateCmd, pinCmd)
	return parent
}

func newPullSubcmd(pullCmd *commands.PullCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull shared rules from external sources and apply to AI tool directories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			force, _ := cmd.Flags().GetBool("force")
			source, _ := cmd.Flags().GetString("source")
			opts := commands.PullOptions{
				DryRun:       dryRun,
				Force:        force,
				SourceFilter: source,
			}
			return pullCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd), opts)
		},
	}
	cmd.Flags().Bool("dry-run", false, "preview changes without applying")
	cmd.Flags().String("source", "", "pull only from a specific source")
	return cmd
}

func newPushSubcmd(pushCmd *commands.PushCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push personal file changes to the sync repo",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			msg, _ := cmd.Flags().GetString("message")
			skipSecretScan, _ := cmd.Flags().GetBool("skip-secret-scan")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			return pushCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd), msg, skipSecretScan, dryRun)
		},
	}
	cmd.Flags().StringP("message", "m", "", "custom commit message")
	cmd.Flags().Bool("skip-secret-scan", false, "skip secret scanning (not recommended)")
	cmd.Flags().Bool("dry-run", false, "preview files that would be pushed without modifying anything")
	return cmd
}

func newSyncSubcmd(syncCmd *commands.SyncCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Pull then push (daily workflow)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			return syncCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd), "", dryRun)
		},
	}
	cmd.Flags().Bool("dry-run", false, "preview changes without applying")
	return cmd
}

func newDiffSubcmd(diffCmd *commands.DiffCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Preview what would change on pull",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			source, _ := cmd.Flags().GetString("source")
			personal, _ := cmd.Flags().GetBool("personal")
			shared, _ := cmd.Flags().GetBool("shared")
			summary, _ := cmd.Flags().GetBool("summary")
			reverse, _ := cmd.Flags().GetBool("reverse")
			tool, _ := cmd.Flags().GetString("tool")

			opts := commands.DiffOptions{
				SourceFilter: source,
				Personal:     personal,
				Shared:       shared,
				Summary:      summary,
				Reverse:      reverse,
				Tool:         tool,
			}
			return diffCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd), opts)
		},
	}
	cmd.Flags().String("source", "", "show diff only for a specific source")
	cmd.Flags().Bool("personal", false, "show only personal file changes")
	cmd.Flags().Bool("shared", false, "show only shared file changes")
	cmd.Flags().Bool("summary", false, "show only file names, no content diff")
	cmd.Flags().Bool("reverse", false, "show what remote would look like after push")
	cmd.Flags().String("tool", "", "use an external diff tool")
	return cmd
}

func newWatchSubcmd(watchCmd *commands.WatchCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Monitor file changes in real-time",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			autoPush, _ := cmd.Flags().GetBool("auto-push")
			interval, _ := cmd.Flags().GetString("interval")
			pollingStr, _ := cmd.Flags().GetString("polling-interval")
			debounce := defaultDebounce
			if d, err := time.ParseDuration(interval); err == nil {
				debounce = d
			}
			var pollingInterval time.Duration
			if d, err := time.ParseDuration(pollingStr); err == nil {
				pollingInterval = d
			}
			return watchCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd), autoPush, debounce, pollingInterval)
		},
	}
	cmd.Flags().Bool("auto-push", false, "automatically push after debounce window")
	cmd.Flags().String("interval", "60s", "debounce interval for auto-push")
	cmd.Flags().String("polling-interval", "30s", "polling interval for file change detection (Android/Termux)")
	return cmd
}

func newStatusSubcmd(statusCmd *commands.StatusCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync state, managed files, and source freshness",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return statusCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd))
		},
	}
	return cmd
}

func newKeySubcmd(keyCmd *commands.KeyCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	parent := &cobra.Command{Use: "key", Short: "Manage age encryption keys"}

	//nolint:exhaustruct // cobra command does not require all fields
	parent.AddCommand(
		&cobra.Command{Use: "generate", Short: "Generate a new age key pair", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return keyCmd.Generate(resolveConfigPath(cmd)) }},
		&cobra.Command{
			Use:   "import <path>",
			Short: "Import an existing age key",
			Args:  cobra.ExactArgs(1),
			RunE:  func(cmd *cobra.Command, args []string) error { return keyCmd.Import(resolveConfigPath(cmd), args[0]) },
		},
		&cobra.Command{Use: "export", Short: "Export the public key", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return keyCmd.Export(resolveConfigPath(cmd)) }},
		&cobra.Command{Use: "add-recipient <public-key>", Short: "Add an age recipient", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return keyCmd.AddRecipient(resolveConfigPath(cmd), args[0])
			}},
	)
	return parent
}

func newDeviceSubcmd(deviceCmd *commands.DeviceCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	parent := &cobra.Command{Use: "device", Short: "Manage registered devices"}

	//nolint:exhaustruct // cobra command does not require all fields
	parent.AddCommand(
		&cobra.Command{Use: "list", Short: "List registered devices", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return deviceCmd.List(resolveRepoPath(cmd)) }},
		&cobra.Command{Use: "rename <old> <new>", Short: "Rename a device", Args: cobra.ExactArgs(deviceRenameArgs),
			RunE: func(cmd *cobra.Command, args []string) error {
				return deviceCmd.Rename(resolveRepoPath(cmd), args[0], args[1])
			}},
		&cobra.Command{Use: "remove <name>", Short: "Remove a device", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return deviceCmd.Remove(resolveRepoPath(cmd), args[0])
			}},
	)
	return parent
}

func newDoctorSubcmd(doctorCmd *commands.DoctorCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	return &cobra.Command{
		Use: "doctor", Short: "Diagnose common issues", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return doctorCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd))
		},
	}
}

func newMigrateSubcmd(migrateCmd *commands.MigrateCommand) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	cmd := &cobra.Command{
		Use: "migrate", Short: "Migrate from legacy setups", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			return migrateCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd), dryRun)
		},
	}
	cmd.Flags().Bool("dry-run", false, "preview migration without modifying files")
	return cmd
}

func newSelfUpdateSubcmd(version string) *cobra.Command {
	updateCmd := selfupdate.NewCommand("rios0rios0", "aisync", "aisync", version)
	//nolint:exhaustruct // cobra command does not require all fields
	return &cobra.Command{
		Use: "self-update", Short: "Update aisync to the latest version", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			force, _ := cmd.Flags().GetBool("force")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			return updateCmd.Execute(dryRun, force)
		},
	}
}

func newVersionSubcmd(version string) *cobra.Command {
	//nolint:exhaustruct // cobra command does not require all fields
	return &cobra.Command{
		Use: "version", Short: "Print aisync version", Args: cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) { fmt.Fprintln(os.Stdout, "aisync "+version) },
	}
}

// inferMappingsForPath generates a single source mapping for a specific
// subdirectory within the source repo.
func inferMappingsForPath(subpath string) []entities.SourceMapping {
	return []entities.SourceMapping{
		{Source: subpath, Target: "shared/" + subpath},
	}
}

// inferMappings generates default source mappings for common directory structures.
func inferMappings() []entities.SourceMapping {
	return []entities.SourceMapping{
		{Source: "claude/rules", Target: "shared/claude/rules"},
		{Source: "claude/commands", Target: "shared/claude/commands"},
		{Source: "claude/agents", Target: "shared/claude/agents"},
		{Source: "claude/hooks", Target: "shared/claude/hooks"},
		{Source: "claude/skills", Target: "shared/claude/skills"},
		{Source: "cursor/rules", Target: "shared/cursor/rules"},
		{Source: "cursor/skills", Target: "shared/cursor/skills"},
		{Source: "copilot/instructions", Target: "shared/copilot/instructions"},
		{Source: "codex/rules", Target: "shared/codex/rules"},
	}
}
