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

	rootCmd := controllers.NewRootCommand(version)
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		quiet, _ := cmd.Flags().GetBool("quiet")
		if verbose {
			logger.SetLevel(logger.DebugLevel)
		} else if quiet {
			logger.SetLevel(logger.ErrorLevel)
		}
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
