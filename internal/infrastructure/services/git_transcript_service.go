package services

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

const (
	// transcriptNameKeyInfoV1 namespaces the HKDF derivation of the key that
	// hides conversation paths (which contain project names) in the snapshot.
	transcriptNameKeyInfoV1 = "aisync-transcript-name-v1"
	// transcriptNameLength is the number of hex characters kept from the
	// HMAC of a conversation key to name its blob.
	transcriptNameLength = 32
	// transcriptIndexName is the encrypted table of contents at the root of
	// every snapshot.
	transcriptIndexName = "index.age"
	// transcriptIndexVersion is the schema version written into the index.
	transcriptIndexVersion = 1
	// transcriptLocalTimeout bounds git commands that never touch the network.
	transcriptLocalTimeout = time.Minute
	// transcriptNetworkTimeout bounds fetches and pushes of the snapshot.
	transcriptNetworkTimeout = 10 * time.Minute
	// expireImmediately makes git drop reflogs and unreachable objects at once.
	expireImmediately = "now"
)

// GitTranscriptService implements [repositories.TranscriptService] with the
// system git binary and a bare repository at <repo>/.aisync/transcripts.git,
// which holds the latest snapshot only. Each conversation is stored as a
// gzip-compressed, age-encrypted blob named by an HMAC of its path, next to
// an encrypted index; every publish is one parentless commit pushed with
// --force-with-lease, so the branch never accumulates history.
type GitTranscriptService struct {
	encryption repositories.EncryptionService
}

// NewGitTranscriptService creates a new GitTranscriptService.
func NewGitTranscriptService(encryption repositories.EncryptionService) *GitTranscriptService {
	return &GitTranscriptService{encryption: encryption}
}

// transcriptIndex is the encrypted table of contents of a snapshot.
type transcriptIndex struct {
	Version int                   `json:"version"`
	Files   []transcriptIndexFile `json:"files"`
}

// transcriptIndexFile describes one stored conversation file.
type transcriptIndexFile struct {
	Key      string    `json:"key"`
	Stored   string    `json:"stored"`
	Checksum string    `json:"checksum"`
	ModTime  time.Time `json:"mtime"`
	Device   string    `json:"device,omitempty"`
}

// Fetch implements [repositories.TranscriptService].
func (s *GitTranscriptService) Fetch(repoPath, branch, identityPath string) (*entities.TranscriptSnapshot, error) {
	bare, url, err := prepareTranscriptRepo(repoPath)
	if err != nil {
		return nil, err
	}

	ref := "refs/remotes/origin/" + branch
	_, stderr, err := runGit(bare, transcriptNetworkTimeout, nil,
		"fetch", "--quiet", "--no-tags", "--depth=1", url, "+refs/heads/"+branch+":"+ref)
	if err != nil {
		if strings.Contains(stderr, "couldn't find remote ref") {
			_, _, _ = runGit(bare, transcriptLocalTimeout, nil, "update-ref", "-d", ref)
			return entities.NewTranscriptSnapshot(), nil
		}
		return nil, fmt.Errorf("fetching the %s branch: %w", branch, err)
	}

	lease, _, err := runGit(bare, transcriptLocalTimeout, nil, "rev-parse", ref)
	if err != nil {
		return nil, fmt.Errorf("resolving the fetched %s branch: %w", branch, err)
	}
	snapshot, err := s.readIndex(bare, strings.TrimSpace(string(lease)), identityPath)
	if err != nil {
		return nil, err
	}
	pruneTranscriptRepo(bare)
	return snapshot, nil
}

// readIndex decrypts the index of the snapshot at lease.
func (s *GitTranscriptService) readIndex(bare, lease, identityPath string) (*entities.TranscriptSnapshot, error) {
	ciphertext, _, err := runGit(bare, transcriptLocalTimeout, nil, "cat-file", "blob", lease+":"+transcriptIndexName)
	if err != nil {
		return nil, fmt.Errorf("reading the conversation index: %w", err)
	}
	plaintext, err := s.encryption.Decrypt(ciphertext, identityPath)
	if err != nil {
		return nil, fmt.Errorf("decrypting the conversation index (is this device's age key the shared one?): %w", err)
	}

	var index transcriptIndex
	if err = json.Unmarshal(plaintext, &index); err != nil {
		return nil, fmt.Errorf("parsing the conversation index: %w", err)
	}
	if index.Version > transcriptIndexVersion {
		return nil, fmt.Errorf("the conversation index has version %d; update aisync to read it", index.Version)
	}

	snapshot := &entities.TranscriptSnapshot{
		Lease: lease,
		Files: make(map[string]entities.TranscriptFile, len(index.Files)),
	}
	for _, file := range index.Files {
		snapshot.Files[file.Key] = entities.TranscriptFile{
			Key:      file.Key,
			Checksum: file.Checksum,
			ModTime:  file.ModTime,
			Device:   file.Device,
			Stored:   file.Stored,
		}
	}
	return snapshot, nil
}

// Read implements [repositories.TranscriptService].
func (s *GitTranscriptService) Read(
	repoPath, lease string,
	file entities.TranscriptFile,
	identityPath string,
) ([]byte, error) {
	if lease == "" || file.Stored == "" {
		return nil, fmt.Errorf("%s is not stored in the snapshot", file.Key)
	}
	ciphertext, _, err := runGit(transcriptRepoDir(repoPath), transcriptLocalTimeout, nil,
		"cat-file", "blob", lease+":"+file.Stored)
	if err != nil {
		return nil, fmt.Errorf("reading %s from the snapshot: %w", file.Key, err)
	}
	compressed, err := s.encryption.Decrypt(ciphertext, identityPath)
	if err != nil {
		return nil, fmt.Errorf("decrypting %s: %w", file.Key, err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("decompressing %s: %w", file.Key, err)
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// Publish implements [repositories.TranscriptService].
func (s *GitTranscriptService) Publish(
	repoPath, branch string,
	snapshot *entities.TranscriptSnapshot,
	recipients []string,
	identityPath, message string,
) error {
	if len(recipients) == 0 {
		return errors.New("conversations are only published encrypted; no age recipients are configured")
	}
	bare, url, err := prepareTranscriptRepo(repoPath)
	if err != nil {
		return err
	}
	nameKey, err := deriveNameKey(identityPath, transcriptNameKeyInfoV1)
	if err != nil {
		return fmt.Errorf("deriving the conversation name key: %w", err)
	}

	// A newer snapshot fetched or published by another process on this
	// device moves the tracking ref (and may prune the lease's objects), so
	// the snapshot is stale before the push could even tell.
	current, _, _ := runGit(bare, transcriptLocalTimeout, nil,
		"rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	if strings.TrimSpace(string(current)) != snapshot.Lease {
		return repositories.ErrSnapshotMoved
	}

	tree, err := s.writeSnapshotTree(bare, snapshot, recipients, nameKey)
	if err != nil {
		return err
	}
	commit, _, err := runGit(bare, transcriptLocalTimeout, nil, "commit-tree", tree, "-m", message)
	if err != nil {
		return fmt.Errorf("committing the conversation snapshot: %w", err)
	}
	commitID := strings.TrimSpace(string(commit))

	lease := "--force-with-lease=refs/heads/" + branch + ":" + snapshot.Lease
	_, stderr, err := runGit(bare, transcriptNetworkTimeout, nil,
		"push", "--quiet", lease, url, commitID+":refs/heads/"+branch)
	if err != nil {
		if strings.Contains(stderr, "stale info") {
			return repositories.ErrSnapshotMoved
		}
		return fmt.Errorf("pushing the conversation snapshot: %w", err)
	}

	_, _, _ = runGit(bare, transcriptLocalTimeout, nil, "update-ref", "refs/remotes/origin/"+branch, commitID)
	snapshot.Lease = commitID
	pruneTranscriptRepo(bare)
	return nil
}

// writeSnapshotTree stores every new or changed file, reuses the blobs of
// unchanged ones, writes the encrypted index, and returns the root tree.
func (s *GitTranscriptService) writeSnapshotTree(
	bare string,
	snapshot *entities.TranscriptSnapshot,
	recipients []string,
	nameKey []byte,
) (string, error) {
	existing, err := listSnapshotBlobs(bare, snapshot.Lease)
	if err != nil {
		return "", err
	}

	keys := make([]string, 0, len(snapshot.Files))
	for key := range snapshot.Files {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	index := transcriptIndex{Version: transcriptIndexVersion, Files: make([]transcriptIndexFile, 0, len(keys))}
	dirs := make(map[string][]string)
	for _, key := range keys {
		file := snapshot.Files[key]
		blob, stored, storeErr := s.storeFile(bare, file, existing, recipients, nameKey)
		if storeErr != nil {
			return "", storeErr
		}
		dir, name := path.Split(stored)
		dir = strings.TrimSuffix(dir, "/")
		dirs[dir] = append(dirs[dir], "100644 blob "+blob+"\t"+name)
		index.Files = append(index.Files, transcriptIndexFile{
			Key: key, Stored: stored, Checksum: file.Checksum, ModTime: file.ModTime.UTC(), Device: file.Device,
		})
	}

	plainIndex, err := json.Marshal(index)
	if err != nil {
		return "", fmt.Errorf("encoding the conversation index: %w", err)
	}
	encryptedIndex, err := s.encryption.Encrypt(plainIndex, recipients)
	if err != nil {
		return "", fmt.Errorf("encrypting the conversation index: %w", err)
	}
	indexBlob, err := hashObject(bare, encryptedIndex)
	if err != nil {
		return "", err
	}

	root := []string{"100644 blob " + indexBlob + "\t" + transcriptIndexName}
	for dir, entries := range dirs {
		subtree, treeErr := makeTree(bare, entries)
		if treeErr != nil {
			return "", treeErr
		}
		root = append(root, "040000 tree "+subtree+"\t"+dir)
	}
	return makeTree(bare, root)
}

// storeFile returns the blob and stored path for one file: a fresh blob for
// content waiting to be stored, otherwise the blob already in the snapshot.
func (s *GitTranscriptService) storeFile(
	bare string,
	file entities.TranscriptFile,
	existing map[string]string,
	recipients []string,
	nameKey []byte,
) (string, string, error) {
	if file.Content == nil {
		blob, ok := existing[file.Stored]
		if !ok {
			return "", "", fmt.Errorf("the fetched snapshot no longer has %s", file.Key)
		}
		return blob, file.Stored, nil
	}

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(file.Content); err != nil {
		return "", "", fmt.Errorf("compressing %s: %w", file.Key, err)
	}
	if err := writer.Close(); err != nil {
		return "", "", fmt.Errorf("compressing %s: %w", file.Key, err)
	}
	ciphertext, err := s.encryption.Encrypt(compressed.Bytes(), recipients)
	if err != nil {
		return "", "", fmt.Errorf("encrypting %s: %w", file.Key, err)
	}
	blob, err := hashObject(bare, ciphertext)
	if err != nil {
		return "", "", err
	}
	return blob, storedTranscriptName(file.Key, nameKey), nil
}

// storedTranscriptName names a conversation's blob "<tool>/<hmac>.age", so
// the snapshot tree never shows the project names inside conversation paths.
func storedTranscriptName(key string, nameKey []byte) string {
	tool, _, _ := strings.Cut(key, "/")
	mac := hmac.New(sha256.New, nameKey)
	_, _ = mac.Write([]byte(key))
	return tool + "/" + hex.EncodeToString(mac.Sum(nil))[:transcriptNameLength] + ".age"
}

// listSnapshotBlobs maps every stored path of the snapshot at lease to its
// blob, so unchanged files are reused instead of re-encrypted and re-uploaded.
func listSnapshotBlobs(bare, lease string) (map[string]string, error) {
	blobs := make(map[string]string)
	if lease == "" {
		return blobs, nil
	}
	out, _, err := runGit(bare, transcriptLocalTimeout, nil, "ls-tree", "-r", "-z", lease)
	if err != nil {
		return nil, fmt.Errorf("listing the fetched snapshot: %w", err)
	}
	for record := range strings.SplitSeq(string(out), "\x00") {
		meta, name, found := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !found || len(fields) != 3 {
			continue
		}
		blobs[name] = fields[2]
	}
	return blobs, nil
}

// hashObject writes content as a blob and returns its id.
func hashObject(bare string, content []byte) (string, error) {
	out, _, err := runGit(bare, transcriptLocalTimeout, content, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("storing a conversation blob: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// makeTree writes a tree from ls-tree formatted entries and returns its id.
func makeTree(bare string, entries []string) (string, error) {
	out, _, err := runGit(bare, transcriptLocalTimeout, []byte(strings.Join(entries, "\n")+"\n"), "mktree")
	if err != nil {
		return "", fmt.Errorf("writing a snapshot tree: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// transcriptRepoDir is the bare repository holding the snapshot locally.
func transcriptRepoDir(repoPath string) string {
	return filepath.Join(repoPath, ".aisync", "transcripts.git")
}

// prepareTranscriptRepo creates the bare repository on first use and returns
// it with the sync repo's origin URL. Its reflogs are disabled and unreachable
// objects expire immediately, so a replaced snapshot does not linger on disk.
func prepareTranscriptRepo(repoPath string) (string, string, error) {
	out, _, err := runGit(repoPath, transcriptLocalTimeout, nil, "remote", "get-url", "origin")
	if err != nil {
		return "", "", fmt.Errorf("the sync repo has no origin remote to keep conversations in: %w", err)
	}
	url := strings.TrimSpace(string(out))

	bare := transcriptRepoDir(repoPath)
	if _, statErr := os.Stat(filepath.Join(bare, "HEAD")); statErr == nil {
		return bare, url, nil
	}
	if err = os.MkdirAll(bare, 0o700); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", bare, err)
	}
	if _, _, err = runGit(bare, transcriptLocalTimeout, nil, "init", "--bare", "--quiet"); err != nil {
		return "", "", fmt.Errorf("initializing %s: %w", bare, err)
	}
	for _, setting := range [][2]string{
		{"core.logAllRefUpdates", "false"},
		{"gc.pruneExpire", expireImmediately},
		{"gc.reflogExpire", expireImmediately},
		{"gc.reflogExpireUnreachable", expireImmediately},
	} {
		if _, _, err = runGit(bare, transcriptLocalTimeout, nil, "config", setting[0], setting[1]); err != nil {
			return "", "", fmt.Errorf("configuring %s: %w", bare, err)
		}
	}
	return bare, url, nil
}

// pruneTranscriptRepo drops objects of snapshots that were replaced. It is
// housekeeping only, so failures are ignored.
func pruneTranscriptRepo(bare string) {
	_, _, _ = runGit(bare, transcriptLocalTimeout, nil, "prune", "--expire=now")
}

// runGit runs git in dir with an optional stdin and returns stdout and stderr.
func runGit(dir string, timeout time.Duration, stdin []byte, args ...string) ([]byte, string, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, "", fmt.Errorf("conversation sync needs git on PATH: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitPath, append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=aisync", "GIT_AUTHOR_EMAIL=aisync@local",
		"GIT_COMMITTER_NAME=aisync", "GIT_COMMITTER_EMAIL=aisync@local",
	)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err = cmd.Run(); err != nil {
		return stdout.Bytes(), stderr.String(), fmt.Errorf("git %s: %w: %s",
			args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), stderr.String(), nil
}
