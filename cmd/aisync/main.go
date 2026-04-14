package main

import (
	"os"

	"github.com/rios0rios0/cliforge/pkg/selfupdate"
	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/rios0rios0/aisync/internal/infrastructure/controllers"
)

// version is set at build time via ldflags.
var version = "dev"

func main() {
	//nolint:exhaustruct // cobra command does not require all fields
	logger.SetFormatter(&logger.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
	if os.Getenv("DEBUG") == "true" {
		logger.SetLevel(logger.DebugLevel)
	}

	rootCmd, setGitImpl := controllers.NewRootCommand(version)
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		quiet, _ := cmd.Flags().GetBool("quiet")
		if verbose {
			logger.SetLevel(logger.DebugLevel)
		} else if quiet {
			logger.SetLevel(logger.ErrorLevel)
		}

		useSystemGit, _ := cmd.Flags().GetBool("use-system-git")
		if useSystemGit {
			repo, err := controllers.NewExecGitRepository()
			if err != nil {
				logger.Fatalf("--use-system-git: %v", err)
			}
			setGitImpl(repo)
		}

		runUpdateCheck(cmd, quiet)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// runUpdateCheck performs the cliforge update check, skipping local dev builds,
// the self-update / version subcommands, and any invocation that passed --quiet.
// Running after flag parsing (inside PersistentPreRun) means Cobra's own --help,
// flag parse errors, and shell completion paths are bypassed automatically.
func runUpdateCheck(cmd *cobra.Command, quiet bool) {
	if version == "dev" || quiet {
		return
	}
	switch cmd.Name() {
	case "self-update", "version":
		return
	}
	selfupdate.NewCommand(controllers.RepoOwner, controllers.RepoName, controllers.BinaryName, version).
		CheckForUpdates()
}
