package commands

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

const (
	hoursPerDay       = 24
	statusHTTPTimeout = 3 * time.Second
)

// StatusCommand displays the current sync state.
type StatusCommand struct {
	configRepo   repositories.ConfigRepository
	stateRepo    repositories.StateRepository
	manifestRepo repositories.ManifestRepository
}

// NewStatusCommand creates a new StatusCommand.
func NewStatusCommand(
	configRepo repositories.ConfigRepository,
	stateRepo repositories.StateRepository,
	manifestRepo repositories.ManifestRepository,
) *StatusCommand {
	return &StatusCommand{
		configRepo:   configRepo,
		stateRepo:    stateRepo,
		manifestRepo: manifestRepo,
	}
}

// Execute prints the current sync status.
func (c *StatusCommand) Execute(configPath, repoPath string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state := c.loadState(repoPath)
	c.printDeviceInfo(config, state)
	c.printToolsStatus(config)
	fmt.Fprintln(os.Stdout)

	c.printPendingChanges(config, repoPath)
	fmt.Fprintln(os.Stdout)

	c.printIncomingHint(config, state)
	fmt.Fprintln(os.Stdout)

	c.printSourcesStatus(config, state)

	return nil
}

// loadState loads the state from the repo path, returning nil if unavailable.
func (c *StatusCommand) loadState(repoPath string) *entities.State {
	if !c.stateRepo.Exists(repoPath) {
		return nil
	}
	loaded, loadErr := c.stateRepo.Load(repoPath)
	if loadErr != nil {
		return nil
	}
	return loaded
}

// printDeviceInfo prints the device, remote, branch, sync timestamps, and
// encryption status.
func (c *StatusCommand) printDeviceInfo(config *entities.Config, state *entities.State) {
	deviceName, _ := os.Hostname()
	deviceID := "(unknown)"
	lastPull := "(never)"
	lastPush := "(never)"

	if state != nil {
		if device := state.FindDevice(deviceName); device != nil {
			deviceName = device.Name
			deviceID = device.ID
		}
		if !state.LastPull.IsZero() {
			lastPull = state.LastPull.Format("2006-01-02 15:04")
		}
		if !state.LastPush.IsZero() {
			lastPush = state.LastPush.Format("2006-01-02 15:04")
		}
	}

	encryption := "disabled"
	identityPath := ExpandHome(config.Encryption.Identity)
	if _, statErr := os.Stat(identityPath); statErr == nil {
		encryption = "enabled (" + filepath.Base(identityPath) + ")"
	}

	fmt.Fprintf(os.Stdout, "Device:     %s (%s)\n", deviceName, deviceID)

	remote := config.Sync.Remote
	if remote == "" {
		remote = "(not configured)"
	}
	fmt.Fprintf(os.Stdout, "Remote:     %s\n", remote)
	if config.Sync.Remote != "" && !isRemoteReachable(config.Sync.Remote) && state != nil {
		cachedAt := state.LastPull.Format("2006-01-02 15:04")
		fmt.Fprintf(os.Stdout, "            (offline, cached at %s)\n", cachedAt)
	}
	fmt.Fprintf(os.Stdout, "Branch:     %s\n", config.Sync.Branch)
	fmt.Fprintf(os.Stdout, "Last pull:  %s\n", lastPull)
	fmt.Fprintf(os.Stdout, "Last push:  %s\n", lastPush)
	fmt.Fprintf(os.Stdout, "Encryption: %s\n", encryption)
	fmt.Fprintln(os.Stdout)
}

// printToolsStatus prints the status of each enabled AI tool and an aggregate
// file count with provenance breakdown.
func (c *StatusCommand) printToolsStatus(config *entities.Config) {
	totalFiles := 0
	provenance := make(map[string]int)

	fmt.Fprintln(os.Stdout, "AI Tools:")
	for name, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		toolDir := ExpandHome(tool.Path)
		status := "not installed"

		if _, err := os.Stat(toolDir); err == nil {
			status = c.toolManifestStatus(toolDir, &totalFiles, provenance)
		}

		fmt.Fprintf(os.Stdout, "  %-12s %s  %s\n", name, tool.Path, status)
	}
	fmt.Fprintf(os.Stdout, "\n  Total managed files: %d\n", totalFiles)
	if len(provenance) > 0 {
		parts := make([]string, 0, len(provenance))
		for source, count := range provenance {
			parts = append(parts, fmt.Sprintf("%d from %s", count, source))
		}
		fmt.Fprintf(os.Stdout, "  Provenance: %s\n", strings.Join(parts, ", "))
	}
}

// toolManifestStatus returns a human-readable status for a tool directory based on
// its manifest. It also updates totalFiles and provenance accumulators.
func (c *StatusCommand) toolManifestStatus(
	toolDir string,
	totalFiles *int,
	provenance map[string]int,
) string {
	if !c.manifestRepo.Exists(toolDir) {
		return "installed"
	}

	manifest, loadErr := c.manifestRepo.Load(toolDir)
	if loadErr != nil {
		return "installed"
	}

	files := len(manifest.Files)
	*totalFiles += files
	for _, mf := range manifest.Files {
		provenance[mf.Source]++
	}

	return fmt.Sprintf("%d files managed (last sync: %s)",
		files, manifest.LastSync.Format("2006-01-02 15:04"))
}

// printSourcesStatus prints the status of each configured external source.
func (c *StatusCommand) printSourcesStatus(config *entities.Config, state *entities.State) {
	fmt.Fprintln(os.Stdout, "External Sources:")
	if len(config.Sources) == 0 {
		fmt.Fprintln(os.Stdout, "  (none configured)")
		return
	}
	for _, s := range config.Sources {
		ref := s.Ref
		if ref == "" {
			ref = "latest"
		}

		freshness := c.sourceFreshness(s, state)
		fmt.Fprintf(os.Stdout, "  %-20s %s@%s (%s) %s\n", s.Name, s.Repo, s.Branch, ref, freshness)
	}
}

// sourceFreshness returns a human-readable freshness indicator for a source
// based on the ETag presence in state and the configured refresh interval.
func (c *StatusCommand) sourceFreshness(source entities.Source, state *entities.State) string {
	if state == nil {
		return "(never fetched)"
	}

	etag := state.GetETag(source.Name)
	if etag == "" {
		return "(never fetched)"
	}

	refresh := source.Refresh
	if refresh == "" {
		refresh = "168h" // default: 7 days
	}

	duration, parseErr := time.ParseDuration(refresh)
	if parseErr != nil {
		return "(fetched)"
	}

	// Use LastPull as an approximation of when sources were last fetched.
	// If LastPull + refresh interval has elapsed, the source is stale.
	if !state.LastPull.IsZero() && time.Since(state.LastPull) > duration {
		return fmt.Sprintf("(stale, last pull %s ago)", formatDuration(time.Since(state.LastPull)))
	}

	if !state.LastPull.IsZero() {
		return fmt.Sprintf("(fetched %s ago)", formatDuration(time.Since(state.LastPull)))
	}

	return "(fetched)"
}

// printPendingChanges counts local files that differ from or are absent in the
// sync repo's personal/<tool>/ directory for each enabled tool.
func (c *StatusCommand) printPendingChanges(config *entities.Config, repoPath string) {
	fmt.Fprintln(os.Stdout, "Pending Local Changes:")
	anyPending := false

	for name, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		pending := c.countPendingForTool(name, tool, repoPath)
		if pending > 0 {
			anyPending = true
			fmt.Fprintf(os.Stdout, "  %-12s %d file(s) not yet pushed\n", name, pending)
		}
	}

	if !anyPending {
		fmt.Fprintln(os.Stdout, "  (none)")
	}
}

// countPendingForTool walks a single tool directory and counts personal files
// that differ from or are absent in the sync repo.
func (c *StatusCommand) countPendingForTool(name string, tool entities.Tool, repoPath string) int {
	toolDir := ExpandHome(tool.Path)
	if _, err := os.Stat(toolDir); os.IsNotExist(err) {
		return 0
	}

	personalDir := filepath.Join(repoPath, "personal", name)

	var manifest *entities.Manifest
	if c.manifestRepo.Exists(toolDir) {
		loaded, loadErr := c.manifestRepo.Load(toolDir)
		if loadErr == nil {
			manifest = loaded
		}
	}

	pending := 0
	_ = filepath.Walk(toolDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil //nolint:nilerr // return nil to continue Walk traversal
		}

		if c.isFilePending(path, toolDir, personalDir, manifest) {
			pending++
		}
		return nil
	})

	return pending
}

// isFilePending checks whether a local file is new or differs from its sync repo
// counterpart.
func (c *StatusCommand) isFilePending(
	path, toolDir, personalDir string,
	manifest *entities.Manifest,
) bool {
	relPath, err := filepath.Rel(toolDir, path)
	if err != nil {
		return false
	}

	if entities.IsDenied(path) {
		return false
	}

	if manifest != nil {
		if entry, exists := manifest.Files[relPath]; exists && entry.Namespace == "shared" {
			return false
		}
	}

	if strings.HasSuffix(relPath, ".aisync-manifest.json") {
		return false
	}

	localContent, readErr := os.ReadFile(path)
	if readErr != nil {
		return false
	}

	repoFilePath := filepath.Join(personalDir, relPath)
	repoContent, repoErr := os.ReadFile(repoFilePath)
	if repoErr != nil {
		return true
	}

	localSum := sha256.Sum256(localContent)
	repoSum := sha256.Sum256(repoContent)
	return localSum != repoSum
}

// printIncomingHint prints a hint about incoming changes from other devices.
// A full check would require a git fetch, so for offline mode we use heuristics
// based on the state timestamps.
func (c *StatusCommand) printIncomingHint(config *entities.Config, state *entities.State) {
	fmt.Fprintln(os.Stdout, "Incoming Changes:")

	if config.Sync.Remote == "" {
		fmt.Fprintln(os.Stdout, "  (no remote configured)")
		return
	}

	if state == nil || state.LastPull.IsZero() {
		fmt.Fprintln(os.Stdout, "  (never pulled -- run 'aisync pull' to check for incoming changes)")
		return
	}

	// If LastPush from any other device is newer than this device's LastPull,
	// there may be incoming changes. Since we only store global timestamps,
	// check if LastPush > LastPull as a proxy.
	if !state.LastPush.IsZero() && state.LastPush.After(state.LastPull) {
		fmt.Fprintf(os.Stdout, "  Last push (%s) is newer than last pull (%s)\n",
			state.LastPush.Format("2006-01-02 15:04"),
			state.LastPull.Format("2006-01-02 15:04"))
		fmt.Fprintln(os.Stdout, "  (run 'aisync pull' to check for incoming changes)")
		return
	}

	// Check if the pull is stale (older than 24 hours).
	if time.Since(state.LastPull) > 24*time.Hour {
		fmt.Fprintf(os.Stdout, "  Last pull was %s ago\n", formatDuration(time.Since(state.LastPull)))
		fmt.Fprintln(os.Stdout, "  (run 'aisync pull' to check for incoming changes)")
		return
	}

	fmt.Fprintf(os.Stdout, "  Up to date (last pull %s ago)\n", formatDuration(time.Since(state.LastPull)))
}

// formatDuration returns a human-friendly duration string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/hoursPerDay))
}

// isRemoteReachable performs a quick connectivity check for HTTPS remotes.
// SSH remotes are assumed reachable (no lightweight check available).
func isRemoteReachable(remoteURL string) bool {
	if strings.HasPrefix(remoteURL, "git@") || !strings.HasPrefix(remoteURL, "http") {
		return true // Can't cheaply check SSH; assume reachable
	}
	client := &http.Client{Timeout: statusHTTPTimeout}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, remoteURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}
