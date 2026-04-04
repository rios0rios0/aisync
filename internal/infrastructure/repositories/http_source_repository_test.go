//go:build unit

package repositories_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	repositories "github.com/rios0rios0/aisync/internal/infrastructure/repositories"
)

// buildTarGz creates a tar.gz archive in memory with the given files.
// fileMap keys are full paths inside the tarball (including the top-level directory prefix).
func buildTarGz(t *testing.T, fileMap map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range fileMap {
		header := &tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		err := tw.WriteHeader(header)
		assert.NoError(t, err)
		_, err = tw.Write([]byte(content))
		assert.NoError(t, err)
	}

	err := tw.Close()
	assert.NoError(t, err)
	err = gw.Close()
	assert.NoError(t, err)

	return buf.Bytes()
}

func TestHTTPSourceRepository_Fetch_ExtractsFiles(t *testing.T) {
	// given
	tarball := buildTarGz(t, map[string]string{
		"repo-main/claude/rules/architecture.md": "# Architecture\nClean arch rules",
		"repo-main/claude/rules/git-flow.md":     "# Git Flow\nBranch conventions",
		"repo-main/claude/agents/reviewer.md":    "# Reviewer agent",
		"repo-main/README.md":                    "# Repo README",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "\"etag-test-123\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "guide",
		Repo:   "rios0rios0/guide",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "claude/rules", Target: "rules"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, server.Client())

	// Override the source's TarballURL by using a source whose generated URL
	// won't match the server. Instead, we call extractMappedFiles directly
	// or we build a server that handles any path.

	// Alternatively, serve the tarball at the exact expected path.
	// Since the repo constructs the URL from source fields, we need to intercept.
	// The simplest approach: replace the client transport to redirect to our server.
	originalFetch := source.TarballURL()
	_ = originalFetch

	// Use a custom server that responds to any request
	serverAny := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "\"etag-test-123\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer serverAny.Close()

	// Override source Repo so TarballURL points to our test server
	// We need to make the HTTP client follow the server URL.
	// The cleanest approach: create a transport that redirects.
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: serverAny.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "\"etag-test-123\"", result.ETag)
	assert.Equal(t, 2, len(result.Files))

	archContent, ok := result.Files["rules/architecture.md"]
	assert.True(t, ok)
	assert.Equal(t, "# Architecture\nClean arch rules", string(archContent))

	gitFlowContent, ok := result.Files["rules/git-flow.md"]
	assert.True(t, ok)
	assert.Equal(t, "# Git Flow\nBranch conventions", string(gitFlowContent))

	// README should not be in result since it doesn't match the mapping
	_, ok = result.Files["README.md"]
	assert.False(t, ok)
}

func TestHTTPSourceRepository_Fetch_NotModified(t *testing.T) {
	// given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == "\"cached-etag\"" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "guide",
		Repo:   "rios0rios0/guide",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "claude/rules", Target: "rules"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "\"cached-etag\"")

	// then
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestHTTPSourceRepository_Fetch_NonOKStatus(t *testing.T) {
	// given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "guide",
		Repo:   "rios0rios0/guide",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "claude/rules", Target: "rules"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestHTTPSourceRepository_Fetch_ETagCaptured(t *testing.T) {
	// given
	tarball := buildTarGz(t, map[string]string{
		"repo-main/docs/readme.md": "hello",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "\"new-etag-value\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "docs",
		Repo:   "org/docs",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "docs", Target: "output"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "\"new-etag-value\"", result.ETag)
}

func TestHTTPSourceRepository_Fetch_MultipleMappings(t *testing.T) {
	// given
	tarball := buildTarGz(t, map[string]string{
		"repo-main/claude/rules/arch.md":    "architecture",
		"repo-main/claude/agents/review.md": "reviewer",
		"repo-main/cursor/skills/lint.md":   "linting",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "\"multi-etag\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "guide",
		Repo:   "org/guide",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "claude/rules", Target: "rules"},
			{Source: "claude/agents", Target: "agents"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Files))

	_, ok := result.Files["rules/arch.md"]
	assert.True(t, ok)

	_, ok = result.Files["agents/review.md"]
	assert.True(t, ok)

	// cursor/skills should not be extracted (no matching mapping)
	_, ok = result.Files["cursor/skills/lint.md"]
	assert.False(t, ok)
}

func TestHTTPSourceRepository_Fetch_ShouldSendIfNoneMatchHeader(t *testing.T) {
	// given
	var receivedETag string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedETag = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "test",
		Repo:   "org/test",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "src", Target: "dst"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	_, _ = repo.Fetch(source, "\"my-cached-etag\"")

	// then
	assert.Equal(t, "\"my-cached-etag\"", receivedETag)
}

func TestHTTPSourceRepository_Fetch_ShouldReturnErrorForInvalidGzip(t *testing.T) {
	// given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("this is not valid gzip content"))
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "test",
		Repo:   "org/test",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "src", Target: "dst"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to extract tarball")
}

func TestHTTPSourceRepository_Fetch_ShouldSkipTarEntriesWithNoMatchingMapping(t *testing.T) {
	// given
	tarball := buildTarGz(t, map[string]string{
		"repo-main/unrelated/file.txt":      "unrelated content",
		"repo-main/other/nested/data.json":  "nested data",
		"repo-main/claude/rules/matched.md": "matched content",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "test",
		Repo:   "org/test",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "claude/rules", Target: "rules"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Files, 1)

	_, ok := result.Files["rules/matched.md"]
	assert.True(t, ok)

	// Unrelated files should not be in the result
	_, ok = result.Files["unrelated/file.txt"]
	assert.False(t, ok)
	_, ok = result.Files["other/nested/data.json"]
	assert.False(t, ok)
}

func TestMatchesMapping_ShouldMatchExactFile(t *testing.T) {
	// given
	entryPath := "claude/rules/arch.md"
	mappingSource := "claude/rules/arch.md"

	// when
	result := repositories.MatchesMapping(entryPath, mappingSource)

	// then
	assert.True(t, result)
}

func TestMatchesMapping_ShouldMatchDirectoryPrefix(t *testing.T) {
	// given
	entryPath := "claude/rules/arch.md"
	mappingSource := "claude/rules"

	// when
	result := repositories.MatchesMapping(entryPath, mappingSource)

	// then
	assert.True(t, result)
}

func TestMatchesMapping_ShouldNotMatchUnrelatedPath(t *testing.T) {
	// given
	entryPath := "cursor/skills/lint.md"
	mappingSource := "claude/rules"

	// when
	result := repositories.MatchesMapping(entryPath, mappingSource)

	// then
	assert.False(t, result)
}

func TestRemapPath_ShouldRemapExactFileToTarget(t *testing.T) {
	// given
	entryPath := "claude/rules/arch.md"
	mappingSource := "claude/rules/arch.md"
	mappingTarget := "rules"

	// when
	result := repositories.RemapPath(entryPath, mappingSource, mappingTarget)

	// then
	assert.Equal(t, "rules/arch.md", result)
}

func TestRemapPath_ShouldRemapDirectoryPrefixToTarget(t *testing.T) {
	// given
	entryPath := "claude/rules/arch.md"
	mappingSource := "claude/rules"
	mappingTarget := "output"

	// when
	result := repositories.RemapPath(entryPath, mappingSource, mappingTarget)

	// then
	assert.Equal(t, "output/arch.md", result)
}

func TestHTTPSourceRepository_Fetch_ShouldSkipTopLevelOnlyEntries(t *testing.T) {
	// given
	tarball := buildTarGz(t, map[string]string{
		"repo-main/claude/rules/arch.md": "architecture",
		"toplevel-no-slash":              "should be skipped",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "test",
		Repo:   "org/test",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "claude/rules", Target: "rules"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Only the mapped file should be present
	assert.Len(t, result.Files, 1)
}

func TestHTTPSourceRepository_Fetch_ShouldSkipDirectoryEntries(t *testing.T) {
	// given -- create a tarball with both file and directory entries
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add a directory entry
	dirHeader := &tar.Header{
		Name:     "repo-main/claude/rules/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	}
	assert.NoError(t, tw.WriteHeader(dirHeader))

	// Add a file entry
	fileContent := "rule content"
	fileHeader := &tar.Header{
		Name:     "repo-main/claude/rules/arch.md",
		Mode:     0644,
		Size:     int64(len(fileContent)),
		Typeflag: tar.TypeReg,
	}
	assert.NoError(t, tw.WriteHeader(fileHeader))
	_, err := tw.Write([]byte(fileContent))
	assert.NoError(t, err)

	assert.NoError(t, tw.Close())
	assert.NoError(t, gw.Close())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "test",
		Repo:   "org/test",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "claude/rules", Target: "rules"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Files, 1)
	assert.Equal(t, fileContent, string(result.Files["rules/arch.md"]))
}

func TestHTTPSourceRepository_Fetch_ShouldHandleEmptyTarball(t *testing.T) {
	// given
	tarball := buildTarGz(t, map[string]string{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "test",
		Repo:   "org/test",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "claude/rules", Target: "rules"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Files, 0)
}

func TestHTTPSourceRepository_Fetch_ShouldHandleExactFileMapping(t *testing.T) {
	// given
	tarball := buildTarGz(t, map[string]string{
		"repo-main/CLAUDE.md":   "top level claude doc",
		"repo-main/README.md":  "readme content",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	source := &entities.Source{
		Name:   "test",
		Repo:   "org/test",
		Branch: "main",
		Mappings: []entities.SourceMapping{
			{Source: "CLAUDE.md", Target: "docs"},
		},
	}

	repo := repositories.NewHTTPSourceRepository()
	repositories.SetHTTPSourceClient(repo, &http.Client{
		Transport: &redirectTransport{serverURL: server.URL},
	})

	// when
	result, err := repo.Fetch(source, "")

	// then
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Files, 1)

	content, ok := result.Files["docs/CLAUDE.md"]
	assert.True(t, ok)
	assert.Equal(t, "top level claude doc", string(content))
}

// redirectTransport rewrites all requests to target the test server URL.
type redirectTransport struct {
	serverURL string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the URL to point to our test server, preserving headers
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = "http"
	newReq.URL.Host = t.serverURL[len("http://"):]
	return http.DefaultTransport.RoundTrip(newReq)
}
