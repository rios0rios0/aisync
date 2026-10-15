package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// transcriptBaseFileName is the per-device record of synced conversation
// checksums, under the sync repo's never-committed .aisync/ directory.
const transcriptBaseFileName = "transcripts-base.json"

// JSONTranscriptBaseRepository implements TranscriptBaseRepository as a JSON
// file at <repo>/.aisync/transcripts-base.json.
type JSONTranscriptBaseRepository struct{}

// NewJSONTranscriptBaseRepository creates a new JSONTranscriptBaseRepository.
func NewJSONTranscriptBaseRepository() *JSONTranscriptBaseRepository {
	return &JSONTranscriptBaseRepository{}
}

// transcriptBaseFile is the on-disk shape of the base.
type transcriptBaseFile struct {
	Files map[string]string `json:"files"`
}

// Load returns the base, or an empty one before the first conversation sync.
func (r *JSONTranscriptBaseRepository) Load(repoPath string) (map[string]string, error) {
	content, err := os.ReadFile(filepath.Join(repoPath, ".aisync", transcriptBaseFileName))
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", transcriptBaseFileName, err)
	}

	var file transcriptBaseFile
	if err = json.Unmarshal(content, &file); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", transcriptBaseFileName, err)
	}
	if file.Files == nil {
		file.Files = make(map[string]string)
	}
	return file.Files, nil
}

// Save writes the base atomically: a crash leaves the old or the new file,
// never a truncated one.
func (r *JSONTranscriptBaseRepository) Save(repoPath string, base map[string]string) error {
	dir := filepath.Join(repoPath, ".aisync")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	content, err := json.MarshalIndent(transcriptBaseFile{Files: base}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", transcriptBaseFileName, err)
	}

	target := filepath.Join(dir, transcriptBaseFileName)
	temporary := target + ".tmp"
	if err = os.WriteFile(temporary, content, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", temporary, err)
	}
	if err = os.Rename(temporary, target); err != nil {
		return fmt.Errorf("replacing %s: %w", target, err)
	}
	return nil
}
