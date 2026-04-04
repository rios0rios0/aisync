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
	repoPath string,
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

			if entities.IsDenied(localPath) {
				continue
			}

			remoteChecksum := checksumData(remoteContent)

			localContent, err := os.ReadFile(localPath)
			if err != nil {
				// File does not exist locally — it's a new file
				changes = append(changes, entities.FileChange{
					Path:           localRel,
					Direction:      entities.ChangeAdded,
					Source:         sourceFromIncoming(relPath, incomingFiles),
					Namespace:      "shared",
					RemoteSize:     int64(len(remoteContent)),
					RemoteContent:  remoteContent,
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
				Namespace:       "shared",
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

// ComputeLocalDiff compares files in tool directories against the personal/
// namespace in the sync repo to find uncommitted local changes.
func (s *FSDiffService) ComputeLocalDiff(
	config *entities.Config,
	repoPath string,
) ([]entities.FileChange, error) {
	var changes []entities.FileChange

	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		toolDir := expandHomePath(tool.Path)
		personalDir := filepath.Join(repoPath, "personal", toolName)

		if _, err := os.Stat(toolDir); err != nil {
			continue
		}

		err := filepath.WalkDir(toolDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}

			if entities.IsDenied(path) {
				return nil
			}

			relPath, _ := filepath.Rel(toolDir, path)
			repoFile := filepath.Join(personalDir, relPath)

			localContent, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			repoContent, repoErr := os.ReadFile(repoFile)
			if repoErr != nil {
				// File exists locally but not in sync repo — untracked
				info, _ := d.Info()
				mod := time.Time{}
				if info != nil {
					mod = info.ModTime()
				}
				changes = append(changes, entities.FileChange{
					Path:           filepath.Join(toolName, relPath),
					Direction:      entities.ChangeAdded,
					Source:         "personal",
					Namespace:      "personal",
					LocalTimestamp: mod,
					LocalSize:      int64(len(localContent)),
					LocalContent:   localContent,
				})
				return nil
			}

			if checksumData(localContent) != checksumData(repoContent) {
				info, _ := d.Info()
				localMod := time.Time{}
				if info != nil {
					localMod = info.ModTime()
				}
				repoInfo, _ := os.Stat(repoFile)
				repoMod := time.Time{}
				if repoInfo != nil {
					repoMod = repoInfo.ModTime()
				}
				changes = append(changes, entities.FileChange{
					Path:            filepath.Join(toolName, relPath),
					Direction:       entities.ChangeModified,
					Source:          "personal",
					Namespace:       "personal",
					LocalTimestamp:  localMod,
					RemoteTimestamp: repoMod,
					LocalSize:       int64(len(localContent)),
					RemoteSize:      int64(len(repoContent)),
					LocalContent:    localContent,
					RemoteContent:   repoContent,
				})
			}

			return nil
		})
		if err != nil {
			continue
		}
	}

	return changes, nil
}

// ComputePersonalDiff detects incoming personal changes from other devices by
// walking the sync repo's personal/<tool>/ directories and comparing against
// the local tool directories. Files that exist in the repo but not locally are
// reported as ChangeAdded; files that exist in both but differ are reported as
// ChangeModified when the repo version is newer.
func (s *FSDiffService) ComputePersonalDiff(
	config *entities.Config,
	repoPath string,
) ([]entities.FileChange, error) {
	var changes []entities.FileChange

	for toolName, tool := range config.Tools {
		if !tool.Enabled {
			continue
		}

		toolDir := expandHomePath(tool.Path)
		personalDir := filepath.Join(repoPath, "personal", toolName)

		if _, err := os.Stat(personalDir); err != nil {
			continue
		}

		err := filepath.WalkDir(personalDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}

			// Skip .age files; they are encrypted counterparts handled separately.
			if strings.HasSuffix(path, ".age") {
				return nil
			}

			relPath, relErr := filepath.Rel(personalDir, path)
			if relErr != nil {
				return nil
			}

			localPath := filepath.Join(toolDir, relPath)

			repoContent, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			repoInfo, _ := d.Info()
			repoMod := time.Time{}
			if repoInfo != nil {
				repoMod = repoInfo.ModTime()
			}

			localContent, localErr := os.ReadFile(localPath)
			if localErr != nil {
				// File exists in repo but not locally -- incoming from another device.
				changes = append(changes, entities.FileChange{
					Path:            filepath.Join(toolName, relPath),
					Direction:       entities.ChangeAdded,
					Source:          "personal",
					Namespace:       "personal",
					RemoteTimestamp: repoMod,
					RemoteSize:      int64(len(repoContent)),
					RemoteContent:   repoContent,
				})
				return nil
			}

			if checksumData(localContent) != checksumData(repoContent) {
				localInfo, _ := os.Stat(localPath)
				localMod := time.Time{}
				if localInfo != nil {
					localMod = localInfo.ModTime()
				}

				// Only report as incoming change if the repo version is newer.
				if repoMod.After(localMod) {
					changes = append(changes, entities.FileChange{
						Path:            filepath.Join(toolName, relPath),
						Direction:       entities.ChangeModified,
						Source:          "personal",
						Namespace:       "personal",
						LocalTimestamp:  localMod,
						RemoteTimestamp: repoMod,
						LocalSize:       int64(len(localContent)),
						RemoteSize:      int64(len(repoContent)),
						LocalContent:    localContent,
						RemoteContent:   repoContent,
					})
				}
			}

			return nil
		})
		if err != nil {
			continue
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
	return path
}

func checksumData(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}

func sourceFromIncoming(relPath string, _ map[string][]byte) string {
	parts := strings.SplitN(relPath, "/", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}
