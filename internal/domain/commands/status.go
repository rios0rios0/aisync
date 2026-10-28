package commands

import (
	"context"
	"fmt"
	"io"
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
	configRepo     repositories.ConfigRepository
	stateRepo      repositories.StateRepository
	manifestRepo   repositories.ManifestRepository
	excludes       repositories.ExcludeRepository
	personalSyncer *PersonalSyncer
	out            io.Writer
}

// NewStatusCommand creates a new StatusCommand. The PersonalSyncer is the one
// sync uses, so the pending counts are what a sync would actually do.
func NewStatusCommand(
	configRepo repositories.ConfigRepository,
	stateRepo repositories.StateRepository,
	manifestRepo repositories.ManifestRepository,
	personalSyncer *PersonalSyncer,
) *StatusCommand {
	return &StatusCommand{
		configRepo:     configRepo,
		stateRepo:      stateRepo,
		manifestRepo:   manifestRepo,
		personalSyncer: personalSyncer,
		out:            os.Stdout,
	}
}

// WithExcludes sets where the device's exclusions come from: status names the
// file when it holds any, and leaves the paths it excludes out of the pending
// changes. Without it, nothing is excluded.
func (c *StatusCommand) WithExcludes(excludes repositories.ExcludeRepository) *StatusCommand {
	c.excludes = excludes
	return c
}

// Execute prints the current sync status.
func (c *StatusCommand) Execute(configPath, repoPath string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	state := c.loadState(repoPath)
	excludes, excludeErr := loadExcludes(c.excludes)
	c.printDeviceInfo(config, state)
	c.printExcludes(excludes, excludeErr)
	fmt.Fprintln(c.out)
	c.printToolsStatus(config)
	fmt.Fprintln(c.out)

	pending := c.personalSyncer.Pending(config, repoPath)
	c.printPendingChanges(pending)
	fmt.Fprintln(c.out)

	c.printIncomingHint(config, state, pending)
	fmt.Fprintln(c.out)

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

	fmt.Fprintf(c.out, "Device:     %s (%s)\n", deviceName, deviceID)

	remote := config.Sync.Remote
	if remote == "" {
		remote = "(not configured)"
	}
	fmt.Fprintf(c.out, "Remote:     %s\n", remote)
	if config.Sync.Remote != "" && !isRemoteReachable(config.Sync.Remote) && state != nil {
		cachedAt := state.LastPull.Format("2006-01-02 15:04")
		fmt.Fprintf(c.out, "            (offline, cached at %s)\n", cachedAt)
	}
	fmt.Fprintf(c.out, "Branch:     %s\n", config.Sync.Branch)
	fmt.Fprintf(c.out, "Last pull:  %s\n", lastPull)
	fmt.Fprintf(c.out, "Last push:  %s\n", lastPush)
	fmt.Fprintf(c.out, "Encryption: %s\n", encryption)
}

// printExcludes names the file holding the device's exclusions when it holds
// any, or says that it could not be read.
func (c *StatusCommand) printExcludes(excludes *entities.ExcludePatterns, err error) {
	switch {
	case c.excludes == nil:
	case err != nil:
		fmt.Fprintf(c.out, "Excludes:   unreadable (%v)\n", err)
	case excludes.Len() > 0:
		fmt.Fprintf(c.out, "Excludes:   %s (%d pattern(s), never synced on this device)\n",
			c.excludes.Path(), excludes.Len())
	}
}

// printToolsStatus prints the status of each enabled AI tool and an aggregate
// file count with provenance breakdown.
func (c *StatusCommand) printToolsStatus(config *entities.Config) {
	totalFiles := 0
	provenance := make(map[string]int)

	fmt.Fprintln(c.out, "AI Tools:")
	for name, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		toolDir := ExpandHome(tool.Path)
		status := "not installed"

		if _, err := os.Stat(toolDir); err == nil {
			status = c.toolManifestStatus(toolDir, &totalFiles, provenance)
		}

		fmt.Fprintf(c.out, "  %-12s %s  %s\n", name, tool.Path, status)
	}
	fmt.Fprintf(c.out, "\n  Total managed files: %d\n", totalFiles)
	if len(provenance) > 0 {
		parts := make([]string, 0, len(provenance))
		for source, count := range provenance {
			parts = append(parts, fmt.Sprintf("%d from %s", count, source))
		}
		fmt.Fprintf(c.out, "  Provenance: %s\n", strings.Join(parts, ", "))
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
	fmt.Fprintln(c.out, "External Sources:")
	if len(config.Sources) == 0 {
		fmt.Fprintln(c.out, "  (none configured)")
		return
	}
	for _, s := range config.Sources {
		ref := s.Ref
		if ref == "" {
			ref = "latest"
		}

		freshness := c.sourceFreshness(s, state)
		fmt.Fprintf(c.out, "  %-20s %s@%s (%s) %s\n", s.Name, s.Repo, s.Branch, ref, freshness)
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

// printPendingChanges reports, per tool, the personal files a push would
// send (new, changed, or deleted here) and the conflicts a sync would ask
// about. The counts come from the plan sync itself executes, so right after
// a sync there is nothing left to report.
func (c *StatusCommand) printPendingChanges(pending []PersonalPending) {
	fmt.Fprintln(c.out, "Pending Local Changes:")
	anyPending := false

	for _, tool := range pending {
		var parts []string
		if tool.Outgoing > 0 {
			parts = append(parts, fmt.Sprintf("%d file(s) not yet pushed", tool.Outgoing))
		}
		if tool.Conflicts > 0 {
			parts = append(parts, fmt.Sprintf("%d conflict(s) to resolve", tool.Conflicts))
		}
		if len(parts) == 0 {
			continue
		}
		anyPending = true
		fmt.Fprintf(c.out, "  %-12s %s\n", tool.Tool, strings.Join(parts, ", "))
	}

	if !anyPending {
		fmt.Fprintln(c.out, "  (none)")
	}
}

// printIncomingHint prints the personal files the sync repo already holds
// that this device has not applied, or, when there are none, a hint about
// changes other devices may have pushed since. A full check would require a
// git fetch, so for offline mode that hint uses heuristics based on the
// state timestamps.
func (c *StatusCommand) printIncomingHint(
	config *entities.Config,
	state *entities.State,
	pending []PersonalPending,
) {
	fmt.Fprintln(c.out, "Incoming Changes:")

	if c.printUnappliedChanges(pending) {
		fmt.Fprintln(c.out, "  (run 'aisync pull' to apply them)")
		return
	}

	if config.Sync.Remote == "" {
		fmt.Fprintln(c.out, "  (no remote configured)")
		return
	}

	if state == nil || state.LastPull.IsZero() {
		fmt.Fprintln(c.out, "  (never pulled -- run 'aisync pull' to check for incoming changes)")
		return
	}

	// If LastPush from any other device is newer than this device's LastPull,
	// there may be incoming changes. Since we only store global timestamps,
	// check if LastPush > LastPull as a proxy.
	if !state.LastPush.IsZero() && state.LastPush.After(state.LastPull) {
		fmt.Fprintf(c.out, "  Last push (%s) is newer than last pull (%s)\n",
			state.LastPush.Format("2006-01-02 15:04"),
			state.LastPull.Format("2006-01-02 15:04"))
		fmt.Fprintln(c.out, "  (run 'aisync pull' to check for incoming changes)")
		return
	}

	// Check if the pull is stale (older than 24 hours).
	if time.Since(state.LastPull) > 24*time.Hour {
		fmt.Fprintf(c.out, "  Last pull was %s ago\n", formatDuration(time.Since(state.LastPull)))
		fmt.Fprintln(c.out, "  (run 'aisync pull' to check for incoming changes)")
		return
	}

	fmt.Fprintf(c.out, "  Up to date (last pull %s ago)\n", formatDuration(time.Since(state.LastPull)))
}

// printUnappliedChanges lists, per tool, the personal files a pull would
// apply here (new, changed, or deleted in the sync repo), and reports
// whether there were any.
func (c *StatusCommand) printUnappliedChanges(pending []PersonalPending) bool {
	anyIncoming := false
	for _, tool := range pending {
		if tool.Incoming == 0 {
			continue
		}
		anyIncoming = true
		fmt.Fprintf(c.out, "  %-12s %d file(s) in the sync repo not yet applied here\n", tool.Tool, tool.Incoming)
	}
	return anyIncoming
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
