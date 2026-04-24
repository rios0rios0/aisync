package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// PruneBundlesCommand removes bundle ciphertext files from the sync
// repo whose corresponding source directory no longer exists locally.
// It is intentionally interactive: an `rm -rf ~/.claude/projects`
// accident must not turn into a one-shot remote nuke. Each orphan is
// confirmed individually so the user can keep some and prune others.
type PruneBundlesCommand struct {
	configRepo    repositories.ConfigRepository
	bundleService repositories.BundleService
	promptService repositories.PromptService
}

// NewPruneBundlesCommand wires the prune command. Both bundleService
// and promptService are required: HashName is needed to translate a
// project name to its expected on-disk filename, and the prompt is the
// confirmation gate.
func NewPruneBundlesCommand(
	configRepo repositories.ConfigRepository,
	bundleService repositories.BundleService,
	promptService repositories.PromptService,
) *PruneBundlesCommand {
	return &PruneBundlesCommand{
		configRepo:    configRepo,
		bundleService: bundleService,
		promptService: promptService,
	}
}

// PruneResult summarises one prune run for the caller (typically the
// CLI controller) so it can print a single-line outcome.
type PruneResult struct {
	Scanned int
	Removed int
}

// Execute walks each enabled tool's BundleSpec list, computes the set
// of bundle hashes that *should* exist (one per local source subdir),
// and inspects the sync repo for any .age file under the same target
// whose hash is no longer in that set. Each orphan is confirmed
// individually before the file is deleted.
func (c *PruneBundlesCommand) Execute(configPath, repoPath string) (*PruneResult, error) {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if c.bundleService == nil {
		return &PruneResult{}, nil
	}

	result := &PruneResult{}
	for toolName, tool := range config.Tools {
		if !tool.Enabled || len(tool.Bundles) == 0 {
			continue
		}
		toolPath := ExpandHome(tool.Path)
		for _, spec := range tool.Bundles {
			scanned, removed, scanErr := c.pruneOneSpec(repoPath, toolName, toolPath, spec)
			if scanErr != nil {
				logger.Warnf("prune %s/%s: %v", toolName, spec.Source, scanErr)
				continue
			}
			result.Scanned += scanned
			result.Removed += removed
		}
	}
	return result, nil
}

// pruneOneSpec scans the bundle target directory of a single
// BundleSpec, asks for confirmation on every orphan, and deletes the
// confirmed ones. Returns (scanned, removed, error) so the caller can
// build a meaningful summary line.
func (c *PruneBundlesCommand) pruneOneSpec(
	repoPath, toolName, toolPath string,
	spec entities.BundleSpec,
) (int, int, error) {
	targetDir := filepath.Join(repoPath, "personal", toolName, spec.Target)
	bundles, err := os.ReadDir(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("read %s: %w", targetDir, err)
	}

	expected := c.expectedHashes(toolPath, spec)

	scanned, removed := 0, 0
	for _, entry := range bundles {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".age") {
			continue
		}
		scanned++
		hash := strings.TrimSuffix(entry.Name(), ".age")
		if _, alive := expected[hash]; alive {
			continue
		}
		bundlePath := filepath.Join(targetDir, entry.Name())
		prompt := fmt.Sprintf(
			"Bundle %s has no matching local source dir under %s. Remove it from the sync repo?",
			filepath.Join("personal", toolName, spec.Target, entry.Name()),
			filepath.Join(toolPath, spec.Source),
		)
		if c.promptService == nil {
			logger.Warnf("skip deleting %s: prompt service is not configured", bundlePath)
			continue
		}
		if !c.promptService.PromptConfirmation(prompt) {
			continue
		}
		if rmErr := os.Remove(bundlePath); rmErr != nil {
			logger.Warnf("delete %s: %v", bundlePath, rmErr)
			continue
		}
		removed++
		fmt.Fprintf(os.Stdout, "  removed %s\n", bundlePath)
	}
	return scanned, removed, nil
}

// expectedHashes returns the set of bundle hashes that the current
// local source directories would produce on the next push. Any bundle
// in the sync repo whose hash is NOT in this set is an orphan. The
// shape of the set depends on the spec's mode: subdirs mode produces
// one hash per immediate subdirectory; whole mode produces exactly
// one hash (HashName of the source label) iff the source dir exists.
func (c *PruneBundlesCommand) expectedHashes(toolPath string, spec entities.BundleSpec) map[string]struct{} {
	sourceRoot := filepath.Join(toolPath, spec.Source)
	if spec.EffectiveMode() == entities.BundleModeWhole {
		expected := map[string]struct{}{}
		if _, statErr := os.Stat(sourceRoot); statErr == nil {
			expected[c.bundleService.HashName(spec.Source)] = struct{}{}
		}
		return expected
	}
	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return map[string]struct{}{}
	}
	expected := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		expected[c.bundleService.HashName(entry.Name())] = struct{}{}
	}
	return expected
}
