//go:build unit

package services_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
	services "github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func TestMapOp_ShouldReturnCreateForCreateOp(t *testing.T) {
	// given
	op := fsnotify.Create

	// when
	result := services.MapOp(op)

	// then
	assert.Equal(t, "create", result)
}

func TestMapOp_ShouldReturnWriteForWriteOp(t *testing.T) {
	// given
	op := fsnotify.Write

	// when
	result := services.MapOp(op)

	// then
	assert.Equal(t, "write", result)
}

func TestMapOp_ShouldReturnRemoveForRemoveOp(t *testing.T) {
	// given
	op := fsnotify.Remove

	// when
	result := services.MapOp(op)

	// then
	assert.Equal(t, "remove", result)
}

func TestMapOp_ShouldReturnRenameForRenameOp(t *testing.T) {
	// given
	op := fsnotify.Rename

	// when
	result := services.MapOp(op)

	// then
	assert.Equal(t, "rename", result)
}

func TestMapOp_ShouldReturnUnknownForChmodOp(t *testing.T) {
	// given
	op := fsnotify.Chmod

	// when
	result := services.MapOp(op)

	// then
	assert.Equal(t, "unknown", result)
}

func TestNewPollingWatchService_ShouldCreateWithInterval(t *testing.T) {
	// given
	interval := 5 * time.Second

	// when
	svc := services.NewPollingWatchService(interval)

	// then
	assert.NotNil(t, svc)
	assert.Equal(t, interval, services.PollingInterval(svc))
	assert.NotNil(t, services.PollingState(svc))
	assert.NotNil(t, services.PollingStopCh(svc))
}

func TestPollingWatchService_SetIgnorePatterns_ShouldStorePatterns(t *testing.T) {
	// given
	svc := services.NewPollingWatchService(1 * time.Second)
	patterns := entities.ParseIgnorePatterns([]byte("*.tmp\n*.log"))

	// when
	svc.SetIgnorePatterns(patterns)

	// then
	assert.Equal(t, patterns, services.PollingIgnorePatterns(svc))
}

func TestPollingWatchService_Stop_ShouldBeIdempotent(t *testing.T) {
	// given
	svc := services.NewPollingWatchService(1 * time.Second)

	// when -- stop twice
	svc.Stop()
	svc.Stop()

	// then -- should not panic
	assert.True(t, services.PollingStopped(svc))
}

func TestNewFSNotifyWatchService_ShouldCreateWithStopChannel(t *testing.T) {
	// given / when
	svc := services.NewFSNotifyWatchService()

	// then
	assert.NotNil(t, svc)
	assert.NotNil(t, services.FSNotifyStopCh(svc))
}

func TestFSNotifyWatchService_SetIgnorePatterns_ShouldStorePatterns(t *testing.T) {
	// given
	svc := services.NewFSNotifyWatchService()
	patterns := entities.ParseIgnorePatterns([]byte("*.bak\n.DS_Store"))

	// when
	svc.SetIgnorePatterns(patterns)

	// then
	assert.Equal(t, patterns, services.FSNotifyIgnorePatterns(svc))
}

func TestFSNotifyWatchService_Stop_ShouldBeIdempotent(t *testing.T) {
	// given
	svc := services.NewFSNotifyWatchService()

	// when -- stop twice
	svc.Stop()
	svc.Stop()

	// then -- should not panic
	assert.True(t, services.FSNotifyStopped(svc))
}

func TestPollingWatchService_ScanDir_ShouldPopulateState(t *testing.T) {
	// given — files live under rules/ so they match the default allowlist.
	tmpDir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "rules"), 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "rules", "file1.md"), []byte("a"), 0600))
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "rules", "file2.md"), []byte("b"), 0600))

	svc := services.NewPollingWatchService(1 * time.Second)

	// when
	services.PollingScanDir(svc, tmpDir)

	// then
	assert.Len(t, services.PollingState(svc), 2)
}

func TestPollingWatchService_PollDir_ShouldDetectNewFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "rules"), 0700))
	svc := services.NewPollingWatchService(1 * time.Second)
	services.PollingScanDir(svc, tmpDir) // empty state initially

	// Create a new file — under rules/ so it matches the default allowlist.
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "rules", "new.md"), []byte("new"), 0600))

	var mu sync.Mutex
	var events []repositories.FileEvent

	// when
	services.PollingPollDir(svc, tmpDir, func(event repositories.FileEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// then
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, events, 1)
	assert.Equal(t, "create", events[0].Op)
	assert.Contains(t, events[0].Path, "new.md")
}

func TestPollingWatchService_PollDir_ShouldDetectRemovedFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "rules"), 0700))
	filePath := filepath.Join(tmpDir, "rules", "remove-me.md")
	assert.NoError(t, os.WriteFile(filePath, []byte("data"), 0600))

	svc := services.NewPollingWatchService(1 * time.Second)
	services.PollingScanDir(svc, tmpDir)
	assert.Len(t, services.PollingState(svc), 1)

	// Remove the file
	assert.NoError(t, os.Remove(filePath))

	var mu sync.Mutex
	var events []repositories.FileEvent

	// when
	services.PollingPollDir(svc,tmpDir, func(event repositories.FileEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// then
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, events, 1)
	assert.Equal(t, "remove", events[0].Op)
	assert.Contains(t, events[0].Path, "remove-me.md")
}

func TestPollingWatchService_PollDir_ShouldDetectModifiedFile(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "rules"), 0700))
	filePath := filepath.Join(tmpDir, "rules", "modify-me.md")
	assert.NoError(t, os.WriteFile(filePath, []byte("original"), 0600))

	svc := services.NewPollingWatchService(1 * time.Second)
	services.PollingScanDir(svc, tmpDir)

	// Modify with a future mtime
	assert.NoError(t, os.WriteFile(filePath, []byte("modified content"), 0600))
	future := time.Now().Add(10 * time.Second)
	assert.NoError(t, os.Chtimes(filePath, future, future))

	var mu sync.Mutex
	var events []repositories.FileEvent

	// when
	services.PollingPollDir(svc, tmpDir, func(event repositories.FileEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// then
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, events, 1)
	assert.Equal(t, "write", events[0].Op)
	assert.Contains(t, events[0].Path, "modify-me.md")
}

func TestPollingWatchService_Watch_ShouldStartAndStop(t *testing.T) {
	// given
	tmpDir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "rules"), 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "rules", "file.md"), []byte("data"), 0600))

	svc := services.NewPollingWatchService(50 * time.Millisecond)

	// when
	err := svc.Watch([]repositories.WatchedTree{{ToolName: "claude", Dir: tmpDir}}, func(event repositories.FileEvent) {})

	// then
	assert.NoError(t, err)

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	svc.Stop()
	assert.True(t, services.PollingStopped(svc))
}
