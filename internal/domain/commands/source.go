package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

const sourceHTTPTimeout = 30 * time.Second

// SourceCommand manages external sources in the config.
type SourceCommand struct {
	configRepo repositories.ConfigRepository
	sourceRepo repositories.SourceRepository
}

// NewSourceCommand creates a new SourceCommand.
func NewSourceCommand(
	configRepo repositories.ConfigRepository,
	sourceRepo repositories.SourceRepository,
) *SourceCommand {
	return &SourceCommand{configRepo: configRepo, sourceRepo: sourceRepo}
}

// Add adds a new external source to the config.
func (c *SourceCommand) Add(configPath string, source entities.Source) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	for _, existing := range config.Sources {
		if existing.Name == source.Name {
			return fmt.Errorf("source '%s' already exists", source.Name)
		}
	}

	if source.Refresh == "" {
		source.Refresh = "168h"
	}

	config.Sources = append(config.Sources, source)

	if err = c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	logger.Infof("added source '%s' (%s@%s)", source.Name, source.Repo, source.Branch)
	return nil
}

// Remove removes an external source from the config.
func (c *SourceCommand) Remove(configPath, name string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	found := false
	filtered := make([]entities.Source, 0, len(config.Sources))
	for _, s := range config.Sources {
		if s.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, s)
	}

	if !found {
		return fmt.Errorf("source '%s' not found", name)
	}

	config.Sources = filtered

	if err = c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	logger.Infof("removed source '%s'", name)
	return nil
}

// List prints all configured external sources.
func (c *SourceCommand) List(configPath string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(config.Sources) == 0 {
		fmt.Fprintln(os.Stdout, "No external sources configured.")
		fmt.Fprintln(os.Stdout, "Add one with: aisync source add <name> --repo <owner/repo> --branch <branch>")
		return nil
	}

	fmt.Fprintf(os.Stdout, "%-20s %-35s %-15s %-10s %s\n", "NAME", "REPOSITORY", "BRANCH", "REF", "MAPPINGS")
	for _, s := range config.Sources {
		ref := s.Ref
		if ref == "" {
			ref = "latest"
		}
		fmt.Fprintf(os.Stdout, "%-20s %-35s %-15s %-10s %d\n", s.Name, s.Repo, s.Branch, ref, len(s.Mappings))
	}

	return nil
}

// Update re-fetches one or all external sources, ignoring the refresh interval.
func (c *SourceCommand) Update(configPath, repoPath, name string) error {
	if c.sourceRepo == nil {
		return errors.New("source repository not configured")
	}

	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	updated := 0
	fileOwnership := make(map[string]string)

	for _, source := range config.Sources {
		if name != "" && source.Name != name {
			continue
		}

		if c.fetchAndWriteSource(source, repoPath, fileOwnership) {
			updated++
		}
	}

	if updated == 0 && name != "" {
		return fmt.Errorf("source '%s' not found", name)
	}

	return nil
}

// fetchAndWriteSource fetches a single source and writes its files to the sync
// repo. Returns true if the source was successfully updated.
func (c *SourceCommand) fetchAndWriteSource(
	source entities.Source,
	repoPath string,
	fileOwnership map[string]string,
) bool {
	logger.Infof("updating source '%s' (%s@%s)", source.Name, source.Repo, source.Branch)

	result, fetchErr := c.sourceRepo.Fetch(&source, repositories.CacheHints{})
	if fetchErr != nil {
		logger.Warnf("failed to fetch source '%s': %v", source.Name, fetchErr)
		return false
	}
	if result == nil {
		logger.Infof("source '%s' returned no files", source.Name)
		return false
	}

	written := 0
	for relPath, content := range result.Files {
		if existingSource, ok := fileOwnership[relPath]; ok {
			logger.Warnf(
				"file '%s' provided by both '%s' and '%s' (last source wins)",
				relPath, existingSource, source.Name,
			)
		}
		fileOwnership[relPath] = source.Name

		if c.writeSourceFile(repoPath, relPath, content) {
			written++
		}
	}

	logger.Infof("source '%s': wrote %d files", source.Name, written)
	fmt.Fprintf(os.Stdout, "Updated source '%s' (%d files)\n", source.Name, written)
	return true
}

// writeSourceFile writes a single file from an external source to the sync repo.
func (c *SourceCommand) writeSourceFile(repoPath, relPath string, content []byte) bool {
	destPath := filepath.Join(repoPath, relPath)
	destDir := filepath.Dir(destPath)

	if err := os.MkdirAll(destDir, 0700); err != nil {
		logger.Warnf("failed to create directory %s: %v", destDir, err)
		return false
	}

	if err := os.WriteFile(destPath, content, 0600); err != nil {
		logger.Warnf("failed to write %s: %v", destPath, err)
		return false
	}
	return true
}

// Pin sets a specific tag or SHA for a source.
func (c *SourceCommand) Pin(configPath, name, ref string) error {
	config, err := c.configRepo.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	found := false
	for i, s := range config.Sources {
		if s.Name == name {
			config.Sources[i].Ref = ref
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("source '%s' not found", name)
	}

	if err = c.configRepo.Save(configPath, config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	logger.Infof("pinned source '%s' to ref '%s'", name, ref)
	return nil
}

// AddFromURL fetches a source definition YAML from a URL and adds it to the config.
// Only HTTPS URLs are accepted for security.
func (c *SourceCommand) AddFromURL(configPath, url string) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("only https:// URLs are supported for security, got: %s", url)
	}

	client := &http.Client{Timeout: sourceHTTPTimeout}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch source definition from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var source entities.Source
	if err = yaml.Unmarshal(body, &source); err != nil {
		return fmt.Errorf("failed to parse source definition: %w", err)
	}

	return c.Add(configPath, source)
}
