package repositories

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	domainRepos "github.com/rios0rios0/aisync/internal/domain/repositories"
)

// HTTPSourceRepository fetches external sources via HTTPS tarball downloads.
type HTTPSourceRepository struct {
	client *http.Client
}

// NewHTTPSourceRepository creates a new HTTPSourceRepository.
func NewHTTPSourceRepository() *HTTPSourceRepository {
	return &HTTPSourceRepository{
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Fetch downloads the tarball for the source, extracts mapped files, and returns them.
func (r *HTTPSourceRepository) Fetch(source *entities.Source, hints domainRepos.CacheHints) (*domainRepos.FetchResult, error) {
	url := source.TarballURL()
	logger.Debugf("fetching tarball from %s", url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if hints.ETag != "" {
		req.Header.Set("If-None-Match", hints.ETag)
	}
	if hints.LastModified != "" {
		req.Header.Set("If-Modified-Since", hints.LastModified)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tarball: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	etag := resp.Header.Get("ETag")
	lastModified := resp.Header.Get("Last-Modified")

	files, err := r.extractMappedFiles(resp.Body, source)
	if err != nil {
		return nil, fmt.Errorf("failed to extract tarball: %w", err)
	}

	return &domainRepos.FetchResult{
		Files:        files,
		ETag:         etag,
		LastModified: lastModified,
	}, nil
}

// extractMappedFiles reads a tar.gz stream and extracts files that match the
// source mappings.
func (r *HTTPSourceRepository) extractMappedFiles(reader io.Reader, source *entities.Source) (map[string][]byte, error) {
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	files := make(map[string][]byte)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		// GitHub tarballs have a top-level directory like "repo-branch/"
		// Strip the first component to get the actual path.
		parts := strings.SplitN(header.Name, "/", 2)
		if len(parts) < 2 {
			continue
		}
		entryPath := parts[1]

		for _, mapping := range source.Mappings {
			if !matchesMapping(entryPath, mapping.Source) {
				continue
			}

			relativePath := remapPath(entryPath, mapping.Source, mapping.Target)
			content, readErr := io.ReadAll(tr)
			if readErr != nil {
				logger.Warnf("failed to read %s: %v", header.Name, readErr)
				continue
			}

			files[relativePath] = content
			break
		}
	}

	return files, nil
}

// matchesMapping checks if a tarball entry path falls under a source mapping path.
func matchesMapping(entryPath, mappingSource string) bool {
	// Exact file match
	if entryPath == mappingSource {
		return true
	}
	// Directory prefix match
	return strings.HasPrefix(entryPath, mappingSource+"/")
}

// remapPath converts a tarball entry path to its target path in the sync repo.
func remapPath(entryPath, mappingSource, mappingTarget string) string {
	suffix := strings.TrimPrefix(entryPath, mappingSource)
	suffix = strings.TrimPrefix(suffix, "/")
	if suffix == "" {
		return filepath.Join(mappingTarget, filepath.Base(entryPath))
	}
	return filepath.Join(mappingTarget, suffix)
}
