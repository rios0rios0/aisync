package services

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

const (
	diffPathParts    = 3
	diffMinPathParts = 2

	namespaceShared   = "shared"
	namespacePersonal = "personal"
)

// FSDiffService computes diffs by comparing files on the local filesystem
// against the sync repo and incoming external source content.
type FSDiffService struct{}

// NewFSDiffService creates a new FSDiffService.
func NewFSDiffService() *FSDiffService {
	return &FSDiffService{}
}

// ComputeSharedDiff compares incoming shared files against what currently exists
// in the AI tool directories on disk.
func (s *FSDiffService) ComputeSharedDiff(
	config *entities.Config,
	_ string,
	incomingFiles map[string][]byte,
) ([]entities.FileChange, error) {
	var changes []entities.FileChange

	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		toolDir := expandHomePath(tool.Path)
		prefix := "shared/" + toolName + "/"

		for relPath, remoteContent := range incomingFiles {
			if !strings.HasPrefix(relPath, prefix) {
				continue
			}

			localRel := strings.TrimPrefix(relPath, prefix)
			localPath := filepath.Join(toolDir, localRel)

			if !entities.IsSyncable(toolName, localRel, tool.ExtraAllowlist) {
				continue
			}

			remoteChecksum := checksumData(remoteContent)

			localContent, err := os.ReadFile(localPath)
			if err != nil {
				// File does not exist locally — it's a new file
				changes = append(changes, entities.FileChange{
					Path:            localRel,
					Direction:       entities.ChangeAdded,
					Source:          sourceFromIncoming(relPath, incomingFiles),
					Namespace:       namespaceShared,
					RemoteSize:      int64(len(remoteContent)),
					RemoteContent:   remoteContent,
					RemoteTimestamp: time.Now(),
				})
				continue
			}

			localChecksum := checksumData(localContent)
			if localChecksum == remoteChecksum {
				continue
			}

			localInfo, _ := os.Stat(localPath)
			localMod := time.Time{}
			if localInfo != nil {
				localMod = localInfo.ModTime()
			}

			changes = append(changes, entities.FileChange{
				Path:            localRel,
				Direction:       entities.ChangeModified,
				Source:          sourceFromIncoming(relPath, incomingFiles),
				Namespace:       namespaceShared,
				LocalTimestamp:  localMod,
				RemoteTimestamp: time.Now(),
				LocalSize:       int64(len(localContent)),
				RemoteSize:      int64(len(remoteContent)),
				LocalContent:    localContent,
				RemoteContent:   remoteContent,
			})
		}
	}

	return changes, nil
}

func expandHomePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	if strings.Contains(path, "%") {
		return os.ExpandEnv(path)
	}
	return path
}

func checksumData(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}

func sourceFromIncoming(relPath string, _ map[string][]byte) string {
	parts := strings.SplitN(relPath, "/", diffPathParts)
	if len(parts) >= diffMinPathParts {
		return parts[1]
	}
	return "unknown"
}
