package main

import (
	"os"

	logger "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/rios0rios0/aisync/internal/infrastructure/controllers"
)

// version is set at build time via ldflags.
var version = "dev"

func main() {
	//nolint:exhaustruct
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
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
