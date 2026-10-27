package repositories

import (
	"errors"
	"fmt"
	"os"
	"sync"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// FileExcludeRepository implements ExcludeRepository as a plain text file
// outside the sync repo, one "<tool>/<path pattern>" per line. The file is
// read on every Load, so a long-running watch picks up an edit, and each
// malformed line is warned about once per process rather than on every read.
type FileExcludeRepository struct {
	path   string
	mu     sync.Mutex
	warned map[string]bool
}

// NewFileExcludeRepository creates a FileExcludeRepository reading path.
func NewFileExcludeRepository(path string) *FileExcludeRepository {
	return &FileExcludeRepository{path: path, warned: make(map[string]bool)}
}

// Path returns the file Load reads.
func (r *FileExcludeRepository) Path() string {
	return r.path
}

// Load returns the device's exclusions, or none when the file does not exist.
func (r *FileExcludeRepository) Load() (*entities.ExcludePatterns, error) {
	content, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return &entities.ExcludePatterns{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading this device's exclusions in %s: %w", r.path, err)
	}

	patterns, invalid := entities.ParseExcludePatterns(content)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, line := range invalid {
		if r.warned[line] {
			continue
		}
		r.warned[line] = true
		logger.Warnf("%s: ignoring %q, which names no tool; write it as <tool>/<path>, e.g. claude/settings.json",
			r.path, line)
	}
	return patterns, nil
}
