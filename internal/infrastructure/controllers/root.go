package controllers

import (
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

// DefaultRepoPath returns the default aifiles repo location.
func DefaultRepoPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "aisync", "repo")
	}
	return filepath.Join(home, ".config", "aisync", "repo")
}

// DefaultConfigPath returns the default config.yaml location inside the repo.
func DefaultConfigPath() string {
	return filepath.Join(DefaultRepoPath(), "config.yaml")
}

// NewRootCommand builds the root cobra command with all subcommands.
func NewRootCommand(version string) *cobra.Command {
	// Infrastructure
	configRepo := infraRepos.NewYAMLConfigRepository()
	sourceRepo := infraRepos.NewHTTPSourceRepository()
	manifestRepo := infraRepos.NewJSONManifestRepository()
	stateRepo := infraRepos.NewJSONStateRepository()
	gitRepo := infraRepos.NewGoGitRepository()
	journalRepo := infraRepos.NewJSONJournalRepository(defaultConfigDir())
	toolDetector := services.NewFSToolDetector()
	encryptionSvc := services.NewAgeEncryptionService()
	diffSvc := services.NewFSDiffService()
	secretScanner := services.NewRegexSecretScanner()
	conflictDetector := services.NewConflictDetector()

	// Watch service: fsnotify on desktop, polling on Android
	var watchSvc repositories.WatchService
	if runtime.GOOS == "android" || os.Getenv("ANDROID_ROOT") != "" {
		watchSvc = services.NewPollingWatchService(30 * time.Second)
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
	initCmd := commands.NewInitCommand(configRepo, stateRepo, toolDetector, gitRepo, encryptionSvc)
	sourceCmd := commands.NewSourceCommand(configRepo, sourceRepo)
	pullCmd := commands.NewPullCommand(
		configRepo, stateRepo, sourceRepo, manifestRepo,
		gitRepo, encryptionSvc, conflictDetector,
		hooksMerger, settingsMerger, sectionMerger,
		atomicApplySvc,
	)
	pushCmd := commands.NewPushCommand(configRepo, stateRepo, gitRepo, encryptionSvc, manifestRepo, secretScanner)
	syncCmd := commands.NewSyncCommand(pullCmd, pushCmd)
	statusCmd := commands.NewStatusCommand(configRepo, stateRepo, manifestRepo)
	diffCmd := commands.NewDiffCommand(configRepo, sourceRepo, diffSvc, formatter)
	keyCmd := commands.NewKeyCommand(configRepo, encryptionSvc)
	deviceCmd := commands.NewDeviceCommand(stateRepo)
	doctorCmd := commands.NewDoctorCommand(configRepo, stateRepo, encryptionSvc, toolDetector, formatter)
	migrateCmd := commands.NewMigrateCommand(configRepo, manifestRepo, sourceRepo)
	watchCmd := commands.NewWatchCommand(configRepo, watchSvc, pushCmd)

	//nolint:exhaustruct
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

	return root
}

func defaultConfigDir() string {
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
	//nolint:exhaustruct
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
	//nolint:exhaustruct
	parent := &cobra.Command{
		Use:   "source",
		Short: "Manage external sources",
	}

	//nolint:exhaustruct
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
				return fmt.Errorf("name is required when --from-url is not specified")
			}

			repo, _ := cmd.Flags().GetString("source-repo")
			branch, _ := cmd.Flags().GetString("branch")
			ref, _ := cmd.Flags().GetString("ref")

			source := entities.Source{
				Name:     args[0],
				Repo:     repo,
				Branch:   branch,
				Ref:      ref,
				Refresh:  "168h",
				Mappings: inferMappings(),
			}
			return sourceCmd.Add(resolveConfigPath(cmd), source)
		},
	}
	addCmd.Flags().String("source-repo", "", "repository in owner/repo format")
	addCmd.Flags().String("branch", "main", "branch to pull from")
	addCmd.Flags().String("ref", "", "pin to a specific tag or SHA")
	addCmd.Flags().String("from-url", "", "import source definition from a remote YAML URL")

	//nolint:exhaustruct
	removeCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an external source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sourceCmd.Remove(resolveConfigPath(cmd), args[0])
		},
	}

	//nolint:exhaustruct
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List configured external sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return sourceCmd.List(resolveConfigPath(cmd))
		},
	}

	//nolint:exhaustruct
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

	//nolint:exhaustruct
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
	//nolint:exhaustruct
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
	//nolint:exhaustruct
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
	//nolint:exhaustruct
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
	//nolint:exhaustruct
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
	//nolint:exhaustruct
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Monitor file changes in real-time",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			autoPush, _ := cmd.Flags().GetBool("auto-push")
			interval, _ := cmd.Flags().GetString("interval")
			debounce := 60 * time.Second
			if d, err := time.ParseDuration(interval); err == nil {
				debounce = d
			}
			return watchCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd), autoPush, debounce)
		},
	}
	cmd.Flags().Bool("auto-push", false, "automatically push after debounce window")
	cmd.Flags().String("interval", "60s", "debounce interval for auto-push")
	return cmd
}

func newStatusSubcmd(statusCmd *commands.StatusCommand) *cobra.Command {
	//nolint:exhaustruct
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
	//nolint:exhaustruct
	parent := &cobra.Command{Use: "key", Short: "Manage age encryption keys"}

	//nolint:exhaustruct
	parent.AddCommand(
		&cobra.Command{Use: "generate", Short: "Generate a new age key pair", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return keyCmd.Generate(resolveConfigPath(cmd)) }},
		&cobra.Command{Use: "import <path>", Short: "Import an existing age key", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error { return keyCmd.Import(resolveConfigPath(cmd), args[0]) }},
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
	//nolint:exhaustruct
	parent := &cobra.Command{Use: "device", Short: "Manage registered devices"}

	//nolint:exhaustruct
	parent.AddCommand(
		&cobra.Command{Use: "list", Short: "List registered devices", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return deviceCmd.List(resolveRepoPath(cmd)) }},
		&cobra.Command{Use: "rename <old> <new>", Short: "Rename a device", Args: cobra.ExactArgs(2),
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
	//nolint:exhaustruct
	return &cobra.Command{
		Use: "doctor", Short: "Diagnose common issues", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return doctorCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd))
		},
	}
}

func newMigrateSubcmd(migrateCmd *commands.MigrateCommand) *cobra.Command {
	//nolint:exhaustruct
	return &cobra.Command{
		Use: "migrate", Short: "Migrate from legacy setups", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return migrateCmd.Execute(resolveConfigPath(cmd), resolveRepoPath(cmd))
		},
	}
}

func newSelfUpdateSubcmd(version string) *cobra.Command {
	updateCmd := selfupdate.NewCommand("rios0rios0", "aisync", "aisync", version)
	//nolint:exhaustruct
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
	//nolint:exhaustruct
	return &cobra.Command{
		Use: "version", Short: "Print aisync version", Args: cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) { println("aisync " + version) },
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
