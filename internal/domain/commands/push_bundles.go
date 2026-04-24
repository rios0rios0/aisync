package commands

import (
	"fmt"
	"os"
	"path/filepath"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// produceBundles walks every enabled tool's [entities.BundleSpec] list,
// scans the immediate subdirectories of each spec's source directory,
// and packages each subdirectory into one age-encrypted tarball under
// <repo>/personal/<tool>/<target>/<hash>.age. In dry-run mode the
// bundles are produced in memory and discarded so the caller can report
// the count without mutating the repo working tree.
//
// Returns the total number of bundles that were (or would be) written,
// across all tools and bundle specs.
func (c *PushCommand) produceBundles(
	config *entities.Config,
	repoPath string,
	dryRun bool,
) (int, error) {
	if c.bundleService == nil {
		return 0, nil
	}

	produced := 0
	for toolName, tool := range config.Tools {
		if !tool.Enabled || len(tool.Bundles) == 0 {
			continue
		}
		toolPath := ExpandHome(tool.Path)
		for _, spec := range tool.Bundles {
			n, err := c.produceToolBundle(repoPath, toolName, toolPath, spec, config.Encryption.Recipients, dryRun)
			if err != nil {
				return produced, fmt.Errorf("bundles for %s/%s: %w", toolName, spec.Source, err)
			}
			produced += n
		}
	}
	return produced, nil
}

// produceToolBundle handles one BundleSpec for one tool. In subdirs
// mode it walks immediate subdirectories of the source and produces
// one bundle per subdir; in whole mode it produces a single bundle
// covering the entire source tree (the right shape for flat-file dirs
// like ~/.claude/plans/ where there are no subdirs to enumerate).
func (c *PushCommand) produceToolBundle(
	repoPath, toolName, toolPath string,
	spec entities.BundleSpec,
	recipients []string,
	dryRun bool,
) (int, error) {
	if len(recipients) == 0 {
		logger.Warnf(
			"bundles for %s/%s skipped: no encryption recipients configured (add one to config.yaml)",
			toolName, spec.Source,
		)
		return 0, nil
	}

	sourceRoot := filepath.Join(toolPath, spec.Source)
	if _, statErr := os.Stat(sourceRoot); os.IsNotExist(statErr) {
		return 0, nil
	}

	targetRoot := filepath.Join(repoPath, "personal", toolName, spec.Target)
	if !dryRun {
		if mkErr := os.MkdirAll(targetRoot, 0o700); mkErr != nil {
			return 0, fmt.Errorf("create bundle target: %w", mkErr)
		}
	}

	if spec.EffectiveMode() == entities.BundleModeWhole {
		return c.produceWholeBundle(repoPath, sourceRoot, targetRoot, spec.Source, recipients, dryRun)
	}
	return c.produceSubdirBundles(repoPath, sourceRoot, targetRoot, recipients, dryRun)
}

// produceSubdirBundles is the original subdirs-mode behaviour: one
// bundle per immediate subdirectory of sourceRoot.
func (c *PushCommand) produceSubdirBundles(
	repoPath, sourceRoot, targetRoot string,
	recipients []string,
	dryRun bool,
) (int, error) {
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return 0, fmt.Errorf("read source: %w", err)
	}
	produced := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subdirPath := filepath.Join(sourceRoot, entry.Name())
		ciphertext, manifest, bundleErr := c.bundleService.Bundle(subdirPath, entry.Name(), recipients)
		if bundleErr != nil {
			logger.Warnf("skip bundle for %s: %v", entry.Name(), bundleErr)
			continue
		}
		if manifest.FileCount == 0 {
			continue
		}
		hash := c.bundleService.HashName(entry.Name())
		dest := filepath.Join(targetRoot, hash+".age")
		if writeErr := writeOrLogBundle(repoPath, dest, ciphertext, dryRun); writeErr != nil {
			return produced, writeErr
		}
		produced++
	}
	return produced, nil
}

// produceWholeBundle is the whole-mode behaviour: one bundle covering
// every file under sourceRoot. The bundle hash is derived from
// sourceName so the same logical source always lands at the same
// .age path across pushes (so git delta detection still applies).
func (c *PushCommand) produceWholeBundle(
	repoPath, sourceRoot, targetRoot, sourceName string,
	recipients []string,
	dryRun bool,
) (int, error) {
	ciphertext, manifest, bundleErr := c.bundleService.Bundle(sourceRoot, sourceName, recipients)
	if bundleErr != nil {
		logger.Warnf("skip bundle for %s: %v", sourceName, bundleErr)
		return 0, nil
	}
	if manifest.FileCount == 0 {
		return 0, nil
	}
	hash := c.bundleService.HashName(sourceName)
	dest := filepath.Join(targetRoot, hash+".age")
	if writeErr := writeOrLogBundle(repoPath, dest, ciphertext, dryRun); writeErr != nil {
		return 0, writeErr
	}
	return 1, nil
}

// writeOrLogBundle writes the bundle ciphertext to disk for a real
// push or just logs the would-be path for a dry run. Centralised so
// the two production paths cannot drift out of sync on path display.
func writeOrLogBundle(repoPath, dest string, ciphertext []byte, dryRun bool) error {
	if dryRun {
		fmt.Fprintf(os.Stdout, "  %s (bundle)\n", relRepoPath(repoPath, dest))
		return nil
	}
	if writeErr := os.WriteFile(dest, ciphertext, 0o600); writeErr != nil {
		return fmt.Errorf("write bundle %s: %w", dest, writeErr)
	}
	return nil
}

// relRepoPath returns the slash-separated path of bundlePath relative to
// repoPath so dry-run output matches the form used elsewhere in the
// summary (e.g. "personal/claude/projects/<hash>.age").
func relRepoPath(repoPath, bundlePath string) string {
	rel, err := filepath.Rel(repoPath, bundlePath)
	if err != nil {
		return bundlePath
	}
	return filepath.ToSlash(rel)
}

// bundleTargetExtraAllowlist returns the gitwildmatch-style patterns
// that the legacy-file warner should treat as syncable for a tool. Each
// configured BundleSpec adds <target>/** so .age files produced by
// produceBundles do not get reported as legacy entries on the next push.
func bundleTargetExtraAllowlist(tool entities.Tool) []string {
	if len(tool.Bundles) == 0 {
		return nil
	}
	patterns := make([]string, 0, len(tool.Bundles))
	for _, spec := range tool.Bundles {
		if spec.Target == "" {
			continue
		}
		patterns = append(patterns, filepath.ToSlash(filepath.Join(spec.Target, "**")))
	}
	return patterns
}
