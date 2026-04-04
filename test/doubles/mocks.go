//go:build unit

package doubles

import (
	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// MockConfigRepository is a manual stub for repositories.ConfigRepository.
type MockConfigRepository struct {
	Config      *entities.Config
	SavedPath   string
	SavedConfig *entities.Config
	LoadErr     error
	SaveErr     error
	ExistsVal   bool
	LoadCalls   int
	SaveCalls   int
	ExistsCalls int
}

func (m *MockConfigRepository) Load(path string) (*entities.Config, error) {
	m.LoadCalls++
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	return m.Config, nil
}

func (m *MockConfigRepository) Save(path string, config *entities.Config) error {
	m.SaveCalls++
	m.SavedPath = path
	m.SavedConfig = config
	if m.SaveErr != nil {
		return m.SaveErr
	}
	return nil
}

func (m *MockConfigRepository) Exists(path string) bool {
	m.ExistsCalls++
	return m.ExistsVal
}

// MockStateRepository is a manual stub for repositories.StateRepository.
type MockStateRepository struct {
	State      *entities.State
	SavedPath  string
	SavedState *entities.State
	LoadErr    error
	SaveErr    error
	ExistsVal  bool
	LoadCalls  int
	SaveCalls  int
}

func (m *MockStateRepository) Load(repoPath string) (*entities.State, error) {
	m.LoadCalls++
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	return m.State, nil
}

func (m *MockStateRepository) Save(repoPath string, state *entities.State) error {
	m.SaveCalls++
	m.SavedPath = repoPath
	m.SavedState = state
	if m.SaveErr != nil {
		return m.SaveErr
	}
	return nil
}

func (m *MockStateRepository) Exists(repoPath string) bool {
	return m.ExistsVal
}

// MockSourceRepository is a manual stub for repositories.SourceRepository.
type MockSourceRepository struct {
	Result    *repositories.FetchResult
	FetchErr  error
	FetchedSources []*entities.Source
	FetchCalls int
}

func (m *MockSourceRepository) Fetch(source *entities.Source, cachedETag string) (*repositories.FetchResult, error) {
	m.FetchCalls++
	m.FetchedSources = append(m.FetchedSources, source)
	if m.FetchErr != nil {
		return nil, m.FetchErr
	}
	return m.Result, nil
}

// MockManifestRepository is a manual stub for repositories.ManifestRepository.
type MockManifestRepository struct {
	Manifest      *entities.Manifest
	SavedDir      string
	SavedManifest *entities.Manifest
	LoadErr       error
	SaveErr       error
	ExistsVal     bool
	LoadCalls     int
	SaveCalls     int
}

func (m *MockManifestRepository) Load(toolDir string) (*entities.Manifest, error) {
	m.LoadCalls++
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	return m.Manifest, nil
}

func (m *MockManifestRepository) Save(toolDir string, manifest *entities.Manifest) error {
	m.SaveCalls++
	m.SavedDir = toolDir
	m.SavedManifest = manifest
	if m.SaveErr != nil {
		return m.SaveErr
	}
	return nil
}

func (m *MockManifestRepository) Exists(toolDir string) bool {
	return m.ExistsVal
}

// MockGitRepository is a manual stub for repositories.GitRepository.
type MockGitRepository struct {
	CloneURL       string
	CloneDir       string
	CloneBranch    string
	CloneErr       error
	InitDir        string
	InitErr        error
	OpenDir        string
	OpenErr        error
	PullErr        error
	CommitMsg      string
	CommitErr      error
	PushErr        error
	IsCleanVal     bool
	IsCleanErr     error
	HasRemoteVal   bool
	CloneCalls     int
	InitCalls      int
	OpenCalls      int
	PullCalls      int
	CommitAllCalls int
	PushCalls      int
}

func (m *MockGitRepository) Clone(url, dir, branch string) error {
	m.CloneCalls++
	m.CloneURL = url
	m.CloneDir = dir
	m.CloneBranch = branch
	return m.CloneErr
}

func (m *MockGitRepository) Init(dir string) error {
	m.InitCalls++
	m.InitDir = dir
	return m.InitErr
}

func (m *MockGitRepository) Open(dir string) error {
	m.OpenCalls++
	m.OpenDir = dir
	return m.OpenErr
}

func (m *MockGitRepository) Pull() error {
	m.PullCalls++
	return m.PullErr
}

func (m *MockGitRepository) CommitAll(message string) error {
	m.CommitAllCalls++
	m.CommitMsg = message
	return m.CommitErr
}

func (m *MockGitRepository) Push() error {
	m.PushCalls++
	return m.PushErr
}

func (m *MockGitRepository) IsClean() (bool, error) {
	return m.IsCleanVal, m.IsCleanErr
}

func (m *MockGitRepository) HasRemote() bool {
	return m.HasRemoteVal
}

// MockEncryptionService is a manual stub for repositories.EncryptionService.
type MockEncryptionService struct {
	GeneratedPublicKey string
	GenerateOutputPath string
	GenerateErr        error
	ImportSourcePath   string
	ImportDestPath     string
	ImportErr          error
	ExportedPublicKey  string
	ExportIdentityPath string
	ExportErr          error
	EncryptedData      []byte
	EncryptErr         error
	DecryptedData      []byte
	DecryptErr         error
	GenerateCalls      int
	ImportCalls        int
	ExportCalls        int
	EncryptCalls       int
	DecryptCalls       int
}

func (m *MockEncryptionService) GenerateKey(outputPath string) (string, error) {
	m.GenerateCalls++
	m.GenerateOutputPath = outputPath
	if m.GenerateErr != nil {
		return "", m.GenerateErr
	}
	return m.GeneratedPublicKey, nil
}

func (m *MockEncryptionService) ImportKey(sourcePath, destPath string) error {
	m.ImportCalls++
	m.ImportSourcePath = sourcePath
	m.ImportDestPath = destPath
	return m.ImportErr
}

func (m *MockEncryptionService) ExportPublicKey(identityPath string) (string, error) {
	m.ExportCalls++
	m.ExportIdentityPath = identityPath
	if m.ExportErr != nil {
		return "", m.ExportErr
	}
	return m.ExportedPublicKey, nil
}

func (m *MockEncryptionService) Encrypt(plaintext []byte, recipients []string) ([]byte, error) {
	m.EncryptCalls++
	if m.EncryptErr != nil {
		return nil, m.EncryptErr
	}
	return m.EncryptedData, nil
}

func (m *MockEncryptionService) Decrypt(ciphertext []byte, identityPath string) ([]byte, error) {
	m.DecryptCalls++
	if m.DecryptErr != nil {
		return nil, m.DecryptErr
	}
	return m.DecryptedData, nil
}

// MockToolDetector is a manual stub for repositories.ToolDetector.
type MockToolDetector struct {
	DetectedTools map[string]entities.Tool
	DetectCalls   int
}

func (m *MockToolDetector) DetectInstalled(defaults map[string]entities.Tool) map[string]entities.Tool {
	m.DetectCalls++
	if m.DetectedTools != nil {
		return m.DetectedTools
	}
	return defaults
}

// MockSecretScanner is a manual stub for repositories.SecretScanner.
type MockSecretScanner struct {
	Findings  []repositories.SecretFinding
	ScanCalls int
}

func (m *MockSecretScanner) Scan(files map[string][]byte) []repositories.SecretFinding {
	m.ScanCalls++
	return m.Findings
}

// MockDiffService is a manual stub for repositories.DiffService.
type MockDiffService struct {
	SharedDiff   []entities.FileChange
	SharedErr    error
	LocalDiff    []entities.FileChange
	LocalErr     error
	PersonalDiff []entities.FileChange
	PersonalErr  error
}

func (m *MockDiffService) ComputeSharedDiff(
	config *entities.Config,
	repoPath string,
	incomingFiles map[string][]byte,
) ([]entities.FileChange, error) {
	return m.SharedDiff, m.SharedErr
}

func (m *MockDiffService) ComputeLocalDiff(
	config *entities.Config,
	repoPath string,
) ([]entities.FileChange, error) {
	return m.LocalDiff, m.LocalErr
}

func (m *MockDiffService) ComputePersonalDiff(
	config *entities.Config,
	repoPath string,
) ([]entities.FileChange, error) {
	return m.PersonalDiff, m.PersonalErr
}

// MockWatchService is a manual stub for repositories.WatchService.
type MockWatchService struct {
	WatchDirs    []string
	WatchErr     error
	StopCalls    int
	WatchCalls   int
	IgnorePatterns *entities.IgnorePatterns
}

func (m *MockWatchService) Watch(dirs []string, callback func(event repositories.FileEvent)) error {
	m.WatchCalls++
	m.WatchDirs = dirs
	return m.WatchErr
}

func (m *MockWatchService) Stop() {
	m.StopCalls++
}

func (m *MockWatchService) SetIgnorePatterns(patterns *entities.IgnorePatterns) {
	m.IgnorePatterns = patterns
}

// MockMerger is a manual stub for repositories.Merger.
type MockMerger struct {
	MergedData []byte
	MergeErr   error
	MergeCalls int
}

func (m *MockMerger) Merge(sharedSources [][]byte, personal []byte) ([]byte, error) {
	m.MergeCalls++
	if m.MergeErr != nil {
		return nil, m.MergeErr
	}
	return m.MergedData, nil
}

// MockApplyService is a manual stub for repositories.ApplyService.
type MockApplyService struct {
	StagedJournal *entities.Journal
	StageErr      error
	ApplyErr      error
	RecoverErr    error
	StageCalls    int
	ApplyCalls    int
	RecoverCalls  int
}

func (m *MockApplyService) Stage(files map[string][]byte) (*entities.Journal, error) {
	m.StageCalls++
	if m.StageErr != nil {
		return nil, m.StageErr
	}
	if m.StagedJournal != nil {
		return m.StagedJournal, nil
	}
	j := entities.NewJournal("/tmp/staging")
	return j, nil
}

func (m *MockApplyService) Apply(journal *entities.Journal) error {
	m.ApplyCalls++
	return m.ApplyErr
}

func (m *MockApplyService) Recover() error {
	m.RecoverCalls++
	return m.RecoverErr
}

// MockConflictDetector is a manual stub for repositories.ConflictDetector.
type MockConflictDetector struct {
	Conflicts  []entities.Conflict
	DetectErr  error
	ResolveErr error
	DetectCalls  int
	ResolveCalls int
}

func (m *MockConflictDetector) DetectConflicts(
	toolDir string,
	incomingFiles map[string][]byte,
	manifest *entities.Manifest,
	deviceName string,
) ([]entities.Conflict, error) {
	m.DetectCalls++
	return m.Conflicts, m.DetectErr
}

func (m *MockConflictDetector) ResolveConflict(toolDir string, conflict entities.Conflict, choice string) error {
	m.ResolveCalls++
	return m.ResolveErr
}

// MockJournalRepository is a manual stub for repositories.JournalRepository.
type MockJournalRepository struct {
	Journal    *entities.Journal
	LoadErr    error
	SaveErr    error
	ExistsVal  bool
	ClearErr   error
	LoadCalls  int
	SaveCalls  int
	ClearCalls int
}

func (m *MockJournalRepository) Load() (*entities.Journal, error) {
	m.LoadCalls++
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	return m.Journal, nil
}

func (m *MockJournalRepository) Save(journal *entities.Journal) error {
	m.SaveCalls++
	return m.SaveErr
}

func (m *MockJournalRepository) Exists() bool {
	return m.ExistsVal
}

func (m *MockJournalRepository) Clear() error {
	m.ClearCalls++
	return m.ClearErr
}
