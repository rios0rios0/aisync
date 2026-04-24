//go:build unit

package commands

import "github.com/rios0rios0/aisync/internal/domain/entities"

// ExposeApplyBundles is a test-only re-export of the unexported
// [PullCommand.applyBundles] method so external test packages can
// exercise the post-file-apply bundle pipeline (extract, merge,
// deletion-prompt) without needing to set up the full Execute call.
func ExposeApplyBundles(c *PullCommand, config *entities.Config, repoPath string) {
	c.applyBundles(config, repoPath)
}
