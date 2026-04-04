package commands

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
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

	deviceName, _ := os.Hostname()
	deviceID := "(unknown)"
	lastPull := "(never)"
	lastPush := "(never)"
	encryption := "disabled"

	var state *entities.State
	if c.stateRepo.Exists(repoPath) {
		loaded, loadErr := c.stateRepo.Load(repoPath)
		if loadErr == nil {
			state = loaded
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
	}

	identityPath := ExpandHome(config.Encryption.Identity)
	if _, statErr := os.Stat(identityPath); statErr == nil {
		encryption = "enabled (" + filepath.Base(identityPath) + ")"
	}

	fmt.Printf("Device:     %s (%s)\n", deviceName, deviceID)

	remote := config.Sync.Remote
	if remote == "" {
		remote = "(not configured)"
	}
	fmt.Printf("Remote:     %s\n", remote)
	fmt.Printf("Branch:     %s\n", config.Sync.Branch)
	fmt.Printf("Last pull:  %s\n", lastPull)
	fmt.Printf("Last push:  %s\n", lastPush)
	fmt.Printf("Encryption: %s\n", encryption)
	fmt.Println()

	// Tools status
	totalFiles := 0
	fmt.Println("AI Tools:")
	for name, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		toolDir := ExpandHome(tool.Path)
		status := "not installed"

		if _, err := os.Stat(toolDir); err == nil {
			status = "installed"
			if c.manifestRepo.Exists(toolDir) {
				manifest, loadErr := c.manifestRepo.Load(toolDir)
				if loadErr == nil {
					files := len(manifest.Files)
					totalFiles += files
					status = fmt.Sprintf("%d files managed (last sync: %s)",
						files, manifest.LastSync.Format("2006-01-02 15:04"))
				}
			}
		}

		fmt.Printf("  %-12s %s  %s\n", name, tool.Path, status)
	}
	fmt.Printf("\n  Total managed files: %d\n", totalFiles)
	fmt.Println()

	// Pending local changes
	c.printPendingChanges(config, repoPath)
	fmt.Println()

	// Incoming changes hint
	c.printIncomingHint(config, state)
	fmt.Println()

	// Sources status
	fmt.Println("External Sources:")
	if len(config.Sources) == 0 {
		fmt.Println("  (none configured)")
	}
	for _, s := range config.Sources {
		ref := s.Ref
		if ref == "" {
			ref = "latest"
		}

		freshness := c.sourceFreshness(s, state)
		fmt.Printf("  %-20s %s@%s (%s) %s\n", s.Name, s.Repo, s.Branch, ref, freshness)
	}

	return nil
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
	fmt.Println("Pending Local Changes:")
	anyPending := false

	for name, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		toolDir := ExpandHome(tool.Path)
		if _, err := os.Stat(toolDir); os.IsNotExist(err) {
			continue
		}

		personalDir := filepath.Join(repoPath, "personal", name)

		// Load manifest to determine which files are shared vs personal.
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
				return nil
			}

			relPath, err := filepath.Rel(toolDir, path)
			if err != nil {
				return nil
			}

			if entities.IsDenied(path) {
				return nil
			}

			// Skip files tracked as shared in the manifest.
			if manifest != nil {
				if entry, exists := manifest.Files[relPath]; exists && entry.Namespace == "shared" {
					return nil
				}
			}

			// Skip the manifest file itself.
			if strings.HasSuffix(relPath, ".aisync-manifest.json") {
				return nil
			}

			localContent, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			repoFilePath := filepath.Join(personalDir, relPath)
			repoContent, repoErr := os.ReadFile(repoFilePath)
			if repoErr != nil {
				// File exists locally but not in the sync repo.
				pending++
				return nil
			}

			localSum := sha256.Sum256(localContent)
			repoSum := sha256.Sum256(repoContent)
			if localSum != repoSum {
				pending++
			}

			return nil
		})

		if pending > 0 {
			anyPending = true
			fmt.Printf("  %-12s %d file(s) not yet pushed\n", name, pending)
		}
	}

	if !anyPending {
		fmt.Println("  (none)")
	}
}

// printIncomingHint prints a hint about incoming changes from other devices.
// A full check would require a git fetch, so for offline mode we use heuristics
// based on the state timestamps.
func (c *StatusCommand) printIncomingHint(config *entities.Config, state *entities.State) {
	fmt.Println("Incoming Changes:")

	if config.Sync.Remote == "" {
		fmt.Println("  (no remote configured)")
		return
	}

	if state == nil || state.LastPull.IsZero() {
		fmt.Println("  (never pulled -- run 'aisync pull' to check for incoming changes)")
		return
	}

	// If LastPush from any other device is newer than this device's LastPull,
	// there may be incoming changes. Since we only store global timestamps,
	// check if LastPush > LastPull as a proxy.
	if !state.LastPush.IsZero() && state.LastPush.After(state.LastPull) {
		fmt.Printf("  Last push (%s) is newer than last pull (%s)\n",
			state.LastPush.Format("2006-01-02 15:04"),
			state.LastPull.Format("2006-01-02 15:04"))
		fmt.Println("  (run 'aisync pull' to check for incoming changes)")
		return
	}

	// Check if the pull is stale (older than 24 hours).
	if time.Since(state.LastPull) > 24*time.Hour {
		fmt.Printf("  Last pull was %s ago\n", formatDuration(time.Since(state.LastPull)))
		fmt.Println("  (run 'aisync pull' to check for incoming changes)")
		return
	}

	fmt.Printf("  Up to date (last pull %s ago)\n", formatDuration(time.Since(state.LastPull)))
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
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
