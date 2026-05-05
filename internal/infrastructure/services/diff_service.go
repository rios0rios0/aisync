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
		if _, err := os.Stat(toolDir); err != nil {
			continue
		}

		personalDir := filepath.Join(repoPath, "personal", toolName)
		toolChanges := s.diffLocalToolDir(toolName, toolDir, personalDir, tool.ExtraAllowlist)
		changes = append(changes, toolChanges...)
	}

	return changes, nil
}

// diffLocalToolDir walks a single tool directory and compares each file against
// the corresponding file in the personal directory of the sync repo.
func (s *FSDiffService) diffLocalToolDir(
	toolName, toolDir, personalDir string,
	extraAllowlist []string,
) []entities.FileChange {
	var changes []entities.FileChange

	_ = filepath.WalkDir(toolDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		relPath, _ := filepath.Rel(toolDir, path)
		if !entities.IsSyncable(toolName, relPath, extraAllowlist) {
			return nil
		}

		change := s.compareLocalFile(toolName, relPath, path, personalDir, d)
		if change != nil {
			changes = append(changes, *change)
		}
		return nil
	})

	return changes
}

// compareLocalFile compares a single local file against its sync repo counterpart,
// returning a FileChange if they differ, or nil if they match.
func (s *FSDiffService) compareLocalFile(
	toolName, relPath, path, personalDir string,
	d os.DirEntry,
) *entities.FileChange {
	repoFile := filepath.Join(personalDir, relPath)

	localContent, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil
	}

	repoContent, repoErr := os.ReadFile(repoFile)
	if repoErr != nil {
		info, _ := d.Info()
		mod := time.Time{}
		if info != nil {
			mod = info.ModTime()
		}
		return &entities.FileChange{
			Path:           filepath.Join(toolName, relPath),
			Direction:      entities.ChangeAdded,
			Source:         namespacePersonal,
			Namespace:      namespacePersonal,
			LocalTimestamp: mod,
			LocalSize:      int64(len(localContent)),
			LocalContent:   localContent,
		}
	}

	if checksumData(localContent) == checksumData(repoContent) {
		return nil
	}

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

	return &entities.FileChange{
		Path:            filepath.Join(toolName, relPath),
		Direction:       entities.ChangeModified,
		Source:          namespacePersonal,
		Namespace:       namespacePersonal,
		LocalTimestamp:  localMod,
		RemoteTimestamp: repoMod,
		LocalSize:       int64(len(localContent)),
		RemoteSize:      int64(len(repoContent)),
		LocalContent:    localContent,
		RemoteContent:   repoContent,
	}
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

		toolChanges := s.diffPersonalToolDir(toolName, toolDir, personalDir)
		changes = append(changes, toolChanges...)
	}

	return changes, nil
}

// diffPersonalToolDir walks the personal directory for a single tool in the sync
// repo and compares each file against the local tool directory.
func (s *FSDiffService) diffPersonalToolDir(toolName, toolDir, personalDir string) []entities.FileChange {
	var changes []entities.FileChange

	_ = filepath.WalkDir(personalDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		if strings.HasSuffix(path, ".age") {
			return nil
		}

		relPath, relErr := filepath.Rel(personalDir, path)
		if relErr != nil {
			return nil //nolint:nilerr // return nil to continue WalkDir traversal
		}

		change := s.comparePersonalFile(toolName, relPath, path, toolDir, d)
		if change != nil {
			changes = append(changes, *change)
		}
		return nil
	})

	return changes
}

// comparePersonalFile compares a single repo personal file against its local
// counterpart, returning a FileChange for added or modified files, or nil if
// unchanged.
func (s *FSDiffService) comparePersonalFile(
	toolName, relPath, repoPath, toolDir string,
	d os.DirEntry,
) *entities.FileChange {
	localPath := filepath.Join(toolDir, relPath)

	repoContent, readErr := os.ReadFile(repoPath)
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
		return &entities.FileChange{
			Path:            filepath.Join(toolName, relPath),
			Direction:       entities.ChangeAdded,
			Source:          namespacePersonal,
			Namespace:       namespacePersonal,
			RemoteTimestamp: repoMod,
			RemoteSize:      int64(len(repoContent)),
			RemoteContent:   repoContent,
		}
	}

	if checksumData(localContent) == checksumData(repoContent) {
		return nil
	}

	localInfo, _ := os.Stat(localPath)
	localMod := time.Time{}
	if localInfo != nil {
		localMod = localInfo.ModTime()
	}

	if !repoMod.After(localMod) {
		return nil
	}

	return &entities.FileChange{
		Path:            filepath.Join(toolName, relPath),
		Direction:       entities.ChangeModified,
		Source:          namespacePersonal,
		Namespace:       namespacePersonal,
		LocalTimestamp:  localMod,
		RemoteTimestamp: repoMod,
		LocalSize:       int64(len(localContent)),
		RemoteSize:      int64(len(repoContent)),
		LocalContent:    localContent,
		RemoteContent:   repoContent,
	}
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
