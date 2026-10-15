package commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// gitkeepName is the placeholder that keeps an empty directory in the repo.
const gitkeepName = ".gitkeep"

// EncryptStoredPlaintext stores as ciphertext every plaintext file under
// personal/ that matches .aisyncencrypt. A file is encrypted when it is
// written, so without this pass a pattern added later would leave the copies
// already committed in plaintext, including those of tools not installed on
// this device. A dry run only lists the files. It returns how many files it
// encrypted, or would encrypt.
func (s *PersonalSyncer) EncryptStoredPlaintext(config *entities.Config, repoPath string, dryRun bool) int {
	if len(config.Encryption.Recipients) == 0 {
		return 0
	}

	patterns := readEncryptPatterns(repoPath)
	personalDir := filepath.Join(repoPath, "personal")
	var candidates []string
	walkRoot(personalDir, func(_ fs.FS, rel string, d fs.DirEntry) {
		if d.Type().IsRegular() && !strings.HasSuffix(rel, ageSuffix) && path.Base(rel) != gitkeepName &&
			patterns.Matches(path.Join("personal", rel)) {
			candidates = append(candidates, rel)
		}
	})

	if dryRun {
		if len(candidates) > 0 {
			fmt.Fprintln(os.Stdout, "encryption:")
		}
		for _, rel := range candidates {
			fmt.Fprintf(os.Stdout, "  %-8s %s  %s\n", "→ repo", rel, "plaintext copy matches .aisyncencrypt")
		}
		return len(candidates)
	}

	root, err := os.OpenRoot(personalDir)
	if err != nil {
		return 0
	}
	defer root.Close()

	encrypted := 0
	for _, rel := range candidates {
		if encErr := s.encryptRepoFile(root, filepath.FromSlash(rel), config.Encryption.Recipients); encErr != nil {
			logger.Warnf("failed to encrypt personal/%s in the sync repo: %v", rel, encErr)
			continue
		}
		encrypted++
	}
	return encrypted
}

// encryptRepoFile replaces a plaintext file under root with "<name>.age",
// keeping the owner-execute bit so an encrypted script is restored runnable.
// Every access goes through root and the file must still be a regular file,
// so a path swapped for a symlink after the walk is refused.
func (s *PersonalSyncer) encryptRepoFile(root *os.Root, name string, recipients []string) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	plaintext, err := root.ReadFile(name)
	if err != nil {
		return err
	}
	ciphertext, err := s.encryption.Encrypt(plaintext, recipients)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}

	mode := privateFileMode
	if info.Mode().Perm()&ownerExecuteBit != 0 {
		mode = privateExecutableMode
	}
	target := name + ageSuffix
	if err = root.WriteFile(target, ciphertext, mode); err != nil {
		return err
	}
	if err = root.Chmod(target, mode); err != nil {
		return err
	}
	return root.Remove(name)
}
