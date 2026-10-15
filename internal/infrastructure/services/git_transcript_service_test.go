package services_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	"github.com/rios0rios0/aisync/internal/infrastructure/services"
)

// transcriptRemote is a sync repo whose origin is a local bare repository,
// with a real age identity, for exercising GitTranscriptService end to end.
type transcriptRemote struct {
	repoPath   string
	remotePath string
	identity   string
	recipients []string
	service    *services.GitTranscriptService
}

func newTranscriptRemote(t *testing.T) *transcriptRemote {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found, skipping transcript snapshot tests")
	}
	r := &transcriptRemote{
		repoPath:   t.TempDir(),
		remotePath: filepath.Join(t.TempDir(), "aifiles.git"),
		identity:   filepath.Join(t.TempDir(), "key.txt"),
	}
	runTestGit(t, "", "init", "--bare", "--quiet", r.remotePath)
	runTestGit(t, r.repoPath, "init", "--quiet")
	runTestGit(t, r.repoPath, "remote", "add", "origin", r.remotePath)

	encryption := services.NewAgeEncryptionService()
	recipient, err := encryption.GenerateKey(r.identity)
	require.NoError(t, err)
	r.recipients = []string{recipient}
	r.service = services.NewGitTranscriptService(encryption)
	return r
}

func runTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func (r *transcriptRemote) publish(t *testing.T, snapshot *entities.TranscriptSnapshot) error {
	t.Helper()
	return r.service.Publish(
		r.repoPath,
		"transcripts",
		snapshot,
		r.recipients,
		r.identity,
		"sync(test): updated conversations",
	)
}

func (r *transcriptRemote) fetch(t *testing.T) *entities.TranscriptSnapshot {
	t.Helper()
	snapshot, err := r.service.Fetch(r.repoPath, "transcripts", r.identity)
	require.NoError(t, err)
	return snapshot
}

func newFile(key, content string, modTime time.Time) entities.TranscriptFile {
	return entities.TranscriptFile{
		Key:      key,
		Checksum: "sha256:" + key,
		ModTime:  modTime,
		Device:   "laptop",
		Content:  []byte(content),
	}
}

func TestGitTranscriptService(t *testing.T) {
	t.Parallel()

	modTime := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	session := "claude/-home-user-Development-acme-backend/0e1f.jsonl"
	memory := "claude/-home-user-Development-acme-backend/memory/MEMORY.md"

	t.Run("should return an empty snapshot before anything was published", func(t *testing.T) {
		t.Parallel()
		// given
		remote := newTranscriptRemote(t)

		// when
		snapshot := remote.fetch(t)

		// then
		assert.Empty(t, snapshot.Lease)
		assert.Empty(t, snapshot.Files)
	})

	t.Run("should round-trip published conversations with their metadata", func(t *testing.T) {
		t.Parallel()
		// given
		remote := newTranscriptRemote(t)
		snapshot := entities.NewTranscriptSnapshot()
		snapshot.Files[session] = newFile(session, "{\"turn\":1}\n", modTime)
		snapshot.Files[memory] = newFile(memory, "- remembered\n", modTime)

		// when
		require.NoError(t, remote.publish(t, snapshot))
		fetched := remote.fetch(t)

		// then
		require.Len(t, fetched.Files, 2)
		assert.NotEmpty(t, fetched.Lease)
		file := fetched.Files[session]
		assert.Equal(t, "sha256:"+session, file.Checksum)
		assert.True(t, modTime.Equal(file.ModTime))
		assert.Equal(t, "laptop", file.Device)
		content, err := remote.service.Read(remote.repoPath, fetched.Lease, file, remote.identity)
		require.NoError(t, err)
		assert.Equal(t, "{\"turn\":1}\n", string(content))
	})

	t.Run("should keep a single commit on the branch however often it is published", func(t *testing.T) {
		t.Parallel()
		// given
		remote := newTranscriptRemote(t)
		snapshot := entities.NewTranscriptSnapshot()
		snapshot.Files[session] = newFile(session, "v1\n", modTime)
		require.NoError(t, remote.publish(t, snapshot))

		// when
		for _, content := range []string{"v1\nv2\n", "v1\nv2\nv3\n"} {
			next := remote.fetch(t)
			next.Files[session] = newFile(session, content, modTime)
			require.NoError(t, remote.publish(t, next))
		}

		// then
		assert.Equal(t, "1", runTestGit(t, remote.remotePath, "rev-list", "--count", "transcripts"))
		final := remote.fetch(t)
		content, err := remote.service.Read(remote.repoPath, final.Lease, final.Files[session], remote.identity)
		require.NoError(t, err)
		assert.Equal(t, "v1\nv2\nv3\n", string(content))
	})

	t.Run("should reuse the stored blob of an unchanged conversation", func(t *testing.T) {
		t.Parallel()
		// given
		remote := newTranscriptRemote(t)
		snapshot := entities.NewTranscriptSnapshot()
		snapshot.Files[session] = newFile(session, "unchanged\n", modTime)
		snapshot.Files[memory] = newFile(memory, "v1\n", modTime)
		require.NoError(t, remote.publish(t, snapshot))
		first := remote.fetch(t)
		firstBlob := runTestGit(t, remote.remotePath, "rev-parse", "transcripts:"+first.Files[session].Stored)

		// when
		next := remote.fetch(t)
		next.Files[memory] = newFile(memory, "v2\n", modTime)
		require.NoError(t, remote.publish(t, next))

		// then
		second := remote.fetch(t)
		secondBlob := runTestGit(t, remote.remotePath, "rev-parse", "transcripts:"+second.Files[session].Stored)
		assert.Equal(t, firstBlob, secondBlob, "an unchanged conversation is not re-encrypted or re-uploaded")
		content, err := remote.service.Read(remote.repoPath, second.Lease, second.Files[memory], remote.identity)
		require.NoError(t, err)
		assert.Equal(t, "v2\n", string(content))
	})

	t.Run("should report a moved branch instead of overwriting another device's publish", func(t *testing.T) {
		t.Parallel()
		// given
		remote := newTranscriptRemote(t)
		first := entities.NewTranscriptSnapshot()
		first.Files[session] = newFile(session, "v1\n", modTime)
		require.NoError(t, remote.publish(t, first))
		stale := remote.fetch(t)
		other := remote.fetch(t)
		other.Files[memory] = newFile(memory, "from the other device\n", modTime)
		require.NoError(t, remote.publish(t, other))

		// when
		stale.Files[session] = newFile(session, "v2\n", modTime)
		err := remote.publish(t, stale)

		// then
		require.ErrorIs(t, err, repositories.ErrSnapshotMoved)
		assert.Contains(t, remote.fetch(t).Files, memory, "the other device's publish survives")
	})

	t.Run("should report a moved branch when another device published since the fetch", func(t *testing.T) {
		t.Parallel()
		// given
		laptop := newTranscriptRemote(t)
		first := entities.NewTranscriptSnapshot()
		first.Files[session] = newFile(session, "v1\n", modTime)
		require.NoError(t, laptop.publish(t, first))
		fetchedOnLaptop := laptop.fetch(t)

		desktopRepo := t.TempDir()
		runTestGit(t, desktopRepo, "init", "--quiet")
		runTestGit(t, desktopRepo, "remote", "add", "origin", laptop.remotePath)
		desktop := &transcriptRemote{
			repoPath: desktopRepo, remotePath: laptop.remotePath, identity: laptop.identity,
			recipients: laptop.recipients, service: laptop.service,
		}
		onDesktop := desktop.fetch(t)
		onDesktop.Files[memory] = newFile(memory, "written on the desktop\n", modTime)
		require.NoError(t, desktop.publish(t, onDesktop))

		// when
		fetchedOnLaptop.Files[session] = newFile(session, "v2 from the laptop\n", modTime)
		err := laptop.publish(t, fetchedOnLaptop)

		// then
		require.ErrorIs(t, err, repositories.ErrSnapshotMoved)
		assert.Contains(t, laptop.fetch(t).Files, memory, "the desktop's publish survives")
	})

	t.Run("should never show conversation paths in the snapshot tree", func(t *testing.T) {
		t.Parallel()
		// given
		remote := newTranscriptRemote(t)
		snapshot := entities.NewTranscriptSnapshot()
		snapshot.Files[session] = newFile(session, "secret project talk\n", modTime)

		// when
		require.NoError(t, remote.publish(t, snapshot))

		// then
		tree := runTestGit(t, remote.remotePath, "ls-tree", "-r", "--name-only", "transcripts")
		assert.NotContains(t, tree, "acme")
		assert.NotContains(t, tree, ".jsonl")
		assert.Contains(t, tree, "index.age")
		raw := runTestGit(t, remote.remotePath, "cat-file", "-p", "transcripts:index.age")
		assert.NotContains(t, raw, "acme", "the index is encrypted too")
	})

	t.Run("should refuse to publish without recipients", func(t *testing.T) {
		t.Parallel()
		// given
		remote := newTranscriptRemote(t)
		snapshot := entities.NewTranscriptSnapshot()
		snapshot.Files[session] = newFile(session, "x\n", modTime)

		// when
		err := remote.service.Publish(remote.repoPath, "transcripts", snapshot, nil, remote.identity, "message")

		// then
		require.Error(t, err)
		_, statErr := os.Stat(filepath.Join(remote.remotePath, "refs", "heads", "transcripts"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("should return an error when the sync repo has no origin", func(t *testing.T) {
		t.Parallel()
		// given
		remote := newTranscriptRemote(t)
		runTestGit(t, remote.repoPath, "remote", "remove", "origin")

		// when
		_, err := remote.service.Fetch(remote.repoPath, "transcripts", remote.identity)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "origin")
	})
}
