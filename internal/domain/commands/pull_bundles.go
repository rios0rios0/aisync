package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// applyBundles is the post-file-apply step that decrypts every bundle
// the remote shipped, merges the contained files into the matching
// local directory, and prompts the user to remove projects whose bundle
// has been deleted upstream. The full bundle-state cache is rewritten
// at the end so the next pull computes the correct deletion set.
func (c *PullCommand) applyBundles(config *entities.Config, repoPath string) {
	if c.bundleService == nil || c.bundleStateRepo == nil {
		return
	}
	if config.Encryption.Identity == "" {
		logger.Debug("bundle apply skipped: no encryption identity configured")
		return
	}

	cached, err := c.bundleStateRepo.Load()
	if err != nil {
		logger.Warnf("bundle state load failed: %v", err)
		cached = entities.NewBundleState()
	}

	current := entities.NewBundleState()
	for toolName, tool := range config.Tools {
		if !tool.Enabled || len(tool.Bundles) == 0 {
			continue
		}
		toolPath := ExpandHome(tool.Path)
		identityPath := ExpandHome(config.Encryption.Identity)
		for _, spec := range tool.Bundles {
			c.applyToolBundles(repoPath, toolName, toolPath, identityPath, spec, current)
		}
	}

	c.promptForRemovedBundles(cached, current, config)

	if saveErr := c.bundleStateRepo.Save(current); saveErr != nil {
		logger.Warnf("bundle state save failed: %v", saveErr)
	}
}

// applyToolBundles processes every <hash>.age file under one tool's
// bundle target directory: decrypts each, asks the bundle service to
// merge it into the matching local source directory, and records the
// hash → original-name mapping in `current` for deletion detection.
func (c *PullCommand) applyToolBundles(
	repoPath, toolName, toolPath, identityPath string,
	spec entities.BundleSpec,
	current *entities.BundleState,
) {
	targetDir := filepath.Join(repoPath, "personal", toolName, spec.Target)
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warnf("bundle scan %s: %v", targetDir, err)
		}
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".age") {
			continue
		}
		hash := strings.TrimSuffix(entry.Name(), ".age")
		bundlePath := filepath.Join(targetDir, entry.Name())

		ciphertext, readErr := os.ReadFile(bundlePath)
		if readErr != nil {
			logger.Warnf("read bundle %s: %v", bundlePath, readErr)
			continue
		}
		manifest, files, extractErr := c.bundleService.Extract(ciphertext, identityPath)
		if extractErr != nil {
			logger.Warnf("extract bundle %s: %v", bundlePath, extractErr)
			continue
		}

		localDir := filepath.Join(toolPath, spec.Source, manifest.OriginalName)
		report, mergeErr := c.bundleService.MergeIntoLocal(files, localDir, spec.EffectiveMergeStrategy())
		if mergeErr != nil {
			logger.Warnf("merge bundle %s: %v", bundlePath, mergeErr)
			continue
		}

		current.Bundles[hash] = entities.BundleStateEntry{
			OriginalName: manifest.OriginalName,
			Tool:         toolName,
			Target:       spec.Target,
			LastSeen:     time.Now().UTC(),
		}

		if total := len(report.Added) + len(report.Overwrote); total > 0 {
			fmt.Fprintf(os.Stdout,
				"  bundle %s/%s: %d added, %d updated, %d preserved\n",
				toolName, manifest.OriginalName,
				len(report.Added), len(report.Overwrote), len(report.SkippedNew),
			)
		}
	}
}

// promptForRemovedBundles compares the previous-pull cache against the
// freshly-pulled bundle set. Anything in the cache but missing from
// `current` was deleted upstream — for each one whose local source
// directory still exists, ask the user whether to remove it locally
// too. Auto-removal is intentionally avoided so a remote `prune`
// mistake cannot wipe local state without confirmation.
func (c *PullCommand) promptForRemovedBundles(
	cached, current *entities.BundleState,
	config *entities.Config,
) {
	for hash, entry := range cached.Bundles {
		if _, stillPresent := current.Bundles[hash]; stillPresent {
			continue
		}
		c.handleRemovedBundle(entry, config)
	}
}

// handleRemovedBundle resolves the local source directory that
// corresponds to a removed cache entry, asks the user whether to
// delete it, and performs the deletion if they confirm. Extracted from
// promptForRemovedBundles to keep cognitive complexity in check.
func (c *PullCommand) handleRemovedBundle(
	entry entities.BundleStateEntry,
	config *entities.Config,
) {
	tool, ok := config.Tools[entry.Tool]
	if !ok || !tool.Enabled {
		return
	}
	sourceRel := bundleSourceRel(tool, entry.Target)
	if sourceRel == "" {
		return
	}
	localDir := filepath.Join(ExpandHome(tool.Path), sourceRel, entry.OriginalName)
	if _, statErr := os.Stat(localDir); os.IsNotExist(statErr) {
		return
	}
	prompt := fmt.Sprintf(
		"Project %q (under %s) was removed on another device. Remove locally too?",
		entry.OriginalName, filepath.Join(tool.Path, sourceRel),
	)
	if c.promptService == nil || !c.promptService.PromptConfirmation(prompt) {
		return
	}
	if rmErr := os.RemoveAll(localDir); rmErr != nil {
		logger.Warnf("remove %s: %v", localDir, rmErr)
		return
	}
	fmt.Fprintf(os.Stdout, "  removed %s\n", localDir)
}

// bundleSourceRel returns the BundleSpec.Source whose Target matches
// the recorded target of a removed cache entry, or "" if the user
// removed the spec from config.yaml between syncs.
func bundleSourceRel(tool entities.Tool, target string) string {
	for _, spec := range tool.Bundles {
		if spec.Target == target {
			return spec.Source
		}
	}
	return ""
}
