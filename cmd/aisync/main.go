package main

import (
	"os"
	"slices"

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
// any invocation that passed --quiet, and the commands checksForUpdates leaves
// out. Running after flag parsing (inside PersistentPreRun) means Cobra's own
// --help and flag parse errors never reach it; shell completion does, which is
// why checksForUpdates names it.
func runUpdateCheck(cmd *cobra.Command, quiet bool) {
	if version == "dev" || quiet || !checksForUpdates(cmd) {
		return
	}
	selfupdate.NewCommand(controllers.RepoOwner, controllers.RepoName, controllers.BinaryName, version).
		CheckForUpdates()
}

// checksForUpdates reports whether running command also checks for a newer
// release. The self-update and version subcommands skip it, and so does
// shell completion: `completion` runs from a shell's startup file every time a
// shell starts, and cobra's hidden `__complete` on every TAB press. Both exit at
// once, so a lookup started there would never be read and would only use up the
// day's update check. `completion bash` is named `bash`, so a command is judged
// by its ancestor directly under the root.
func checksForUpdates(command *cobra.Command) bool {
	for command.HasParent() && command.Parent().HasParent() {
		command = command.Parent()
	}
	return !slices.Contains([]string{
		"self-update", "version", "completion", cobra.ShellCompRequestCmd,
	}, command.Name())
}
