package services

import (
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// DefaultSeparator exposes defaultSeparator for black-box testing.
const DefaultSeparator = defaultSeparator

// ChecksumContent exposes checksumContent for black-box testing.
var ChecksumContent = checksumContent //nolint:gochecknoglobals // test export for black-box testing

// MoveFile exposes moveFile for black-box testing.
var MoveFile = moveFile //nolint:gochecknoglobals // test export for black-box testing

// ComputeChecksum exposes computeChecksum for black-box testing.
var ComputeChecksum = computeChecksum //nolint:gochecknoglobals // test export for black-box testing

// ReadExistingChecksum exposes readExistingChecksum for black-box testing.
var ReadExistingChecksum = readExistingChecksum //nolint:gochecknoglobals // test export for black-box testing

// NormalizeLineEndings exposes normalizeLineEndings for black-box testing.
var NormalizeLineEndings = normalizeLineEndings //nolint:gochecknoglobals // test export for black-box testing

// IsBinaryContent exposes isBinaryContent for black-box testing.
var IsBinaryContent = isBinaryContent //nolint:gochecknoglobals // test export for black-box testing

// ExpandHomePath exposes expandHomePath for black-box testing.
var ExpandHomePath = expandHomePath //nolint:gochecknoglobals // test export for black-box testing

// SourceFromIncoming exposes sourceFromIncoming for black-box testing.
var SourceFromIncoming = sourceFromIncoming //nolint:gochecknoglobals // test export for black-box testing

// ChecksumData exposes checksumData for black-box testing.
var ChecksumData = checksumData //nolint:gochecknoglobals // test export for black-box testing

// MapOp exposes mapOp for black-box testing.
func MapOp(op fsnotify.Op) string {
	return mapOp(op)
}

// PollingInterval returns the interval field from a PollingWatchService.
func PollingInterval(s *PollingWatchService) time.Duration {
	return s.interval
}

// PollingState returns the state field from a PollingWatchService.
func PollingState(s *PollingWatchService) map[string]time.Time {
	return s.state
}

// PollingStopCh returns the stopCh field from a PollingWatchService.
func PollingStopCh(s *PollingWatchService) chan struct{} {
	return s.stopCh
}

// PollingIgnorePatterns returns the ignorePatterns field from a PollingWatchService.
func PollingIgnorePatterns(s *PollingWatchService) *entities.IgnorePatterns {
	return s.ignorePatterns
}

// PollingStopped returns the stopped field from a PollingWatchService.
func PollingStopped(s *PollingWatchService) bool {
	return s.stopped
}

// PollingScanDir exposes scanDir on a PollingWatchService for black-box testing.
func PollingScanDir(s *PollingWatchService, dir string) {
	s.scanDir(dir)
}

// PollingPollDir exposes pollDir on a PollingWatchService for black-box testing.
func PollingPollDir(s *PollingWatchService, dir string, callback func(event repositories.FileEvent)) {
	s.pollDir(dir, callback)
}

// FSNotifyStopCh returns the stopCh field from an FSNotifyWatchService.
func FSNotifyStopCh(s *FSNotifyWatchService) chan struct{} {
	return s.stopCh
}

// FSNotifyIgnorePatterns returns the ignorePatterns field from an FSNotifyWatchService.
func FSNotifyIgnorePatterns(s *FSNotifyWatchService) *entities.IgnorePatterns {
	return s.ignorePatterns
}

// FSNotifyStopped returns the stopped field from an FSNotifyWatchService.
func FSNotifyStopped(s *FSNotifyWatchService) bool {
	return s.stopped
}
