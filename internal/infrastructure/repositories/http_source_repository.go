package repositories

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

const (
	httpSourceTimeout = 120 * time.Second
	tarSplitParts     = 2
)

// NewHTTPSourceRepository creates a new HTTPSourceRepository.
func NewHTTPSourceRepository() *HTTPSourceRepository {
	return &HTTPSourceRepository{
		client: &http.Client{Timeout: httpSourceTimeout},
	}
}

// Fetch downloads the tarball for the source, extracts mapped files, and returns them.
func (r *HTTPSourceRepository) Fetch(
	source *entities.Source,
	hints domainRepos.CacheHints,
) (*domainRepos.FetchResult, error) {
	url := source.TarballURL()
	logger.Debugf("fetching tarball from %s", url)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
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
		return nil, nil //nolint:nilnil // nil result signals 304 Not Modified (cache hit)
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
func (r *HTTPSourceRepository) extractMappedFiles(
	reader io.Reader,
	source *entities.Source,
) (map[string][]byte, error) {
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	files := make(map[string][]byte)

	for {
		header, tarErr := tr.Next()
		if tarErr == io.EOF {
			break
		}
		if tarErr != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", tarErr)
		}

		r.processTarEntry(header, tr, source, files)
	}

	return files, nil
}

// processTarEntry handles a single tar entry, extracting it if it matches a
// source mapping.
func (r *HTTPSourceRepository) processTarEntry(
	header *tar.Header,
	tr *tar.Reader,
	source *entities.Source,
	files map[string][]byte,
) {
	if header.Typeflag != tar.TypeReg {
		return
	}

	parts := strings.SplitN(header.Name, "/", tarSplitParts)
	if len(parts) < tarSplitParts {
		return
	}
	entryPath := filepath.Clean(parts[1])

	if !filepath.IsLocal(entryPath) {
		logger.Warnf("skipping potentially unsafe tar entry: %s", header.Name)
		return
	}

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
