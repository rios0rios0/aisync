package main

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commandRunBy executes args against a command tree shaped like aisync's, which
// cobra completes with its own completion commands, and returns the command
// cobra ran.
func commandRunBy(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	var ran *cobra.Command
	run := func(*cobra.Command, []string) {}
	root := &cobra.Command{
		Use:              "aisync",
		PersistentPreRun: func(command *cobra.Command, _ []string) { ran = command },
		Run:              run,
	}
	root.AddCommand(&cobra.Command{Use: "pull", Run: run})
	root.AddCommand(&cobra.Command{Use: "self-update", Run: run})
	root.AddCommand(&cobra.Command{Use: "version", Run: run})
	source := &cobra.Command{Use: "source"}
	source.AddCommand(&cobra.Command{Use: "add", Run: run})
	root.AddCommand(source)
	root.SetArgs(append([]string{}, args...))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	require.NoError(t, root.Execute())
	require.NotNil(t, ran)
	return ran
}

// invocation spells args out the way a user types them.
func invocation(args []string) string {
	return strings.TrimSpace("aisync " + strings.Join(args, " "))
}

func TestChecksForUpdates(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{}, {"pull"}, {"source", "add"}} {
		t.Run("should check for updates when running `"+invocation(args)+"`", func(t *testing.T) {
			t.Parallel()
			// given
			command := commandRunBy(t, args...)

			// when
			checks := checksForUpdates(command)

			// then
			assert.True(t, checks)
		})
	}

	for _, args := range [][]string{
		{"version"},
		{"self-update"},
		{"completion", "bash"},
		{"completion", "zsh"},
		{cobra.ShellCompRequestCmd, ""},
		{cobra.ShellCompNoDescRequestCmd, ""},
	} {
		t.Run("should not check for updates when running `"+invocation(args)+"`", func(t *testing.T) {
			t.Parallel()
			// given
			command := commandRunBy(t, args...)

			// when
			checks := checksForUpdates(command)

			// then
			assert.False(t, checks)
		})
	}
}
