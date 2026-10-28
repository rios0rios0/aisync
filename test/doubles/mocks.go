package doubles

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

func (m *MockConfigRepository) Load(_ string) (*entities.Config, error) {
	m.LoadCalls++
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	if m.Config == nil {
		// Test-double convenience: return a fresh zero-value Config
		// rather than (nil, nil) so a test that forgets to set Config
		// gets a clean default instead of a nil-pointer-deref panic in
		// the caller. This is NOT a mirror of production semantics —
		// the production YAMLConfigRepository.Load always wraps the
		// underlying os.ReadFile error and returns (nil, wrapped-err)
		// for missing files. Tests that need to exercise the real
		// missing-file error path should set LoadErr explicitly.
		return &entities.Config{}, nil
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

func (m *MockConfigRepository) Exists(_ string) bool {
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

func (m *MockStateRepository) Load(_ string) (*entities.State, error) {
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

func (m *MockStateRepository) Exists(_ string) bool {
	return m.ExistsVal
}

// MockSourceRepository is a manual stub for repositories.SourceRepository.
type MockSourceRepository struct {
	Result          *repositories.FetchResult
	ResultsBySource map[string]*repositories.FetchResult // keyed by source name
	FetchErr        error
	FetchedSources  []*entities.Source
	FetchCalls      int
}

func (m *MockSourceRepository) Fetch(
	source *entities.Source,
	_ repositories.CacheHints,
) (*repositories.FetchResult, error) {
	m.FetchCalls++
	m.FetchedSources = append(m.FetchedSources, source)
	if m.FetchErr != nil {
		return nil, m.FetchErr
	}
	if m.ResultsBySource != nil {
		if result, ok := m.ResultsBySource[source.Name]; ok {
			return result, nil
		}
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

func (m *MockManifestRepository) Load(_ string) (*entities.Manifest, error) {
	m.LoadCalls++
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	return m.Manifest, nil
}

// Save records the manifest and, like the real repository, makes it what
// later Load and Exists calls see.
func (m *MockManifestRepository) Save(toolDir string, manifest *entities.Manifest) error {
	m.SaveCalls++
	m.SavedDir = toolDir
	m.SavedManifest = manifest
	if m.SaveErr != nil {
		return m.SaveErr
	}
	m.Manifest = manifest
	m.ExistsVal = true
	return nil
}

func (m *MockManifestRepository) Exists(_ string) bool {
	return m.ExistsVal
}

// MockGitRepository is a manual stub for repositories.GitRepository.
type MockGitRepository struct {
	CloneURL      string
	CloneDir      string
	CloneBranch   string
	CloneErr      error
	CloneErrByURL map[string]error // per-URL errors; falls back to CloneErr
	CloneAttempts []string         // all URLs passed to Clone in order
	// SimulatePartialClone, when true, makes Clone create a `.git/HEAD`
	// stub at dir on every invocation. This mirrors go-git's PlainClone,
	// which leaves a partial .git/ skeleton behind on failure and breaks
	// any naive retry that does not first wipe the destination.
	SimulatePartialClone bool
	// RefuseIfNotEmpty, when true, makes Clone return an error if dir is
	// non-empty before the call (mirrors go-git's
	// `ErrRepositoryAlreadyExists` and system git's "destination path
	// already exists" failure). Combined with SimulatePartialClone, this
	// reproduces the real-world breakage that the SSH alias retry must
	// guard against.
	RefuseIfNotEmpty bool
	InitDir          string
	InitErr          error
	OpenDir          string
	OpenErr          error
	FetchErr         error
	FetchCalls       int
	PullErr          error
	CommitMsg        string
	CommitErr        error
	PushErr          error
	IsCleanVal       bool
	IsCleanErr       error
	ChangedFilesVal  []string
	ChangedFilesErr  error
	HasRemoteVal     bool
	AddRemoteName    string
	AddRemoteURL     string
	AddRemoteErr     error
	AddRemoteCalls   int
	SetConfigKey     string
	SetConfigValue   string
	SetConfigErr     error
	SetConfigCalls   int
	CloneCalls       int
	InitCalls        int
	OpenCalls        int
	PullCalls        int
	CommitAllCalls   int
	PushCalls        int
	// NothingToPush makes Push report that the remote already had every commit.
	NothingToPush bool
}

func (m *MockGitRepository) Clone(url, dir, branch string) error {
	m.CloneCalls++
	m.CloneAttempts = append(m.CloneAttempts, url)
	m.CloneURL = url
	m.CloneDir = dir
	m.CloneBranch = branch
	if m.RefuseIfNotEmpty {
		if entries, _ := os.ReadDir(dir); len(entries) > 0 {
			return errors.New("destination path already exists and is not an empty directory")
		}
	}
	if m.SimulatePartialClone {
		_ = os.MkdirAll(filepath.Join(dir, ".git"), 0700)
		_ = os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0600)
	}
	if m.CloneErrByURL != nil {
		if err, ok := m.CloneErrByURL[url]; ok {
			return err
		}
	}
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

func (m *MockGitRepository) ChangedFiles() ([]string, error) {
	return m.ChangedFilesVal, m.ChangedFilesErr
}

func (m *MockGitRepository) Fetch() error {
	m.FetchCalls++
	return m.FetchErr
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

func (m *MockGitRepository) Push() (bool, error) {
	m.PushCalls++
	if m.PushErr != nil {
		return false, m.PushErr
	}
	return !m.NothingToPush, nil
}

func (m *MockGitRepository) IsClean() (bool, error) {
	return m.IsCleanVal, m.IsCleanErr
}

func (m *MockGitRepository) HasRemote() bool {
	return m.HasRemoteVal
}

func (m *MockGitRepository) AddRemote(name, url string) error {
	m.AddRemoteCalls++
	m.AddRemoteName = name
	m.AddRemoteURL = url
	return m.AddRemoteErr
}

func (m *MockGitRepository) SetConfig(key, value string) error {
	m.SetConfigCalls++
	m.SetConfigKey = key
	m.SetConfigValue = value
	return m.SetConfigErr
}

// MockEncryptionService is a manual stub for repositories.EncryptionService.
type MockEncryptionService struct {
	GeneratedPublicKey string
	GenerateOutputPath string
	GenerateErr        error
	ImportSourcePath   string
	ImportDestPath     string
	ImportErr          error
	ImportContent      []byte
	ImportContentDest  string
	ImportContentErr   error
	ExportedPublicKey  string
	ExportIdentityPath string
	ExportErr          error
	EncryptedData      []byte
	EncryptPlaintext   []byte // captures the last plaintext passed to Encrypt for round-trip tests
	EncryptErr         error
	DecryptedData      []byte
	DecryptErr         error
	GenerateCalls      int
	ImportCalls        int
	ImportContentCalls int
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

func (m *MockEncryptionService) ImportKeyContent(content []byte, destPath string) error {
	m.ImportContentCalls++
	m.ImportContent = content
	m.ImportContentDest = destPath
	return m.ImportContentErr
}

func (m *MockEncryptionService) ExportPublicKey(identityPath string) (string, error) {
	m.ExportCalls++
	m.ExportIdentityPath = identityPath
	if m.ExportErr != nil {
		return "", m.ExportErr
	}
	return m.ExportedPublicKey, nil
}

func (m *MockEncryptionService) Encrypt(plaintext []byte, _ []string) ([]byte, error) {
	m.EncryptCalls++
	m.EncryptPlaintext = plaintext
	if m.EncryptErr != nil {
		return nil, m.EncryptErr
	}
	return m.EncryptedData, nil
}

func (m *MockEncryptionService) Decrypt(_ []byte, _ string) ([]byte, error) {
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

func (m *MockSecretScanner) Scan(_ map[string][]byte) []repositories.SecretFinding {
	m.ScanCalls++
	return m.Findings
}

// MockDiffService is a manual stub for repositories.DiffService.
type MockDiffService struct {
	SharedDiff []entities.FileChange
	SharedErr  error
	// Incoming captures the files passed to the last ComputeSharedDiff call.
	Incoming map[string][]byte
}

func (m *MockDiffService) ComputeSharedDiff(
	_ *entities.Config,
	_ string,
	incoming map[string][]byte,
) ([]entities.FileChange, error) {
	m.Incoming = incoming
	return m.SharedDiff, m.SharedErr
}

// MockDiffViewer is a manual stub for commands.DiffViewer. It records every
// content it is asked to page and returns Err (nil means "shown").
type MockDiffViewer struct {
	Shown []string
	Err   error
}

func (m *MockDiffViewer) ShowText(content string) error {
	m.Shown = append(m.Shown, content)
	return m.Err
}

// MockNDAContentChecker is a manual stub for repositories.NDAContentChecker.
// Tests set Findings or Err to simulate a clean/blocked scan.
type MockNDAContentChecker struct {
	Findings   []entities.NDAFinding
	Err        error
	CheckCalls int
	LastRepo   string
}

func (m *MockNDAContentChecker) Check(
	repoPath string,
	_ *entities.Config,
	_ map[string][]byte,
) ([]entities.NDAFinding, error) {
	m.CheckCalls++
	m.LastRepo = repoPath
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Findings, nil
}

// MockWatchService is a manual stub for repositories.WatchService.
type MockWatchService struct {
	WatchTrees     []repositories.WatchedTree
	WatchErr       error
	StopCalls      int
	WatchCalls     int
	IgnorePatterns *entities.IgnorePatterns
}

func (m *MockWatchService) Watch(trees []repositories.WatchedTree, _ func(event repositories.FileEvent)) error {
	m.WatchCalls++
	m.WatchTrees = trees
	return m.WatchErr
}

func (m *MockWatchService) Stop() {
	m.StopCalls++
}

func (m *MockWatchService) SetIgnorePatterns(patterns *entities.IgnorePatterns) {
	m.IgnorePatterns = patterns
}

func (m *MockWatchService) SetInterval(_ time.Duration) {}

// MockMerger is a manual stub for repositories.Merger.
type MockMerger struct {
	MergedData []byte
	MergeErr   error
	MergeCalls int
}

func (m *MockMerger) Merge(_ [][]byte, _ []byte) ([]byte, error) {
	m.MergeCalls++
	if m.MergeErr != nil {
		return nil, m.MergeErr
	}
	return m.MergedData, nil
}

// MockApplyService is a manual stub for repositories.ApplyService.
type MockApplyService struct {
	// StagedFiles captures the files passed to the last Stage call.
	StagedFiles map[string][]byte
	// WriteOnApply makes Apply write the staged files to disk, so tests can
	// assert on the resulting tool directory.
	WriteOnApply  bool
	StagedJournal *entities.Journal
	StageErr      error
	ApplyErr      error
	RecoverErr    error
	StageCalls    int
	// StageVerbatimCalls counts StageVerbatim calls, which are also counted
	// in StageCalls.
	StageVerbatimCalls int
	ApplyCalls         int
	RecoverCalls       int
}

func (m *MockApplyService) Stage(files map[string][]byte) (*entities.Journal, error) {
	m.StageCalls++
	m.StagedFiles = files
	if m.StageErr != nil {
		return nil, m.StageErr
	}
	if m.StagedJournal != nil {
		return m.StagedJournal, nil
	}
	j := entities.NewJournal("/tmp/staging")
	return j, nil
}

// StageVerbatim records the call and the files like Stage.
func (m *MockApplyService) StageVerbatim(files map[string][]byte) (*entities.Journal, error) {
	m.StageVerbatimCalls++
	return m.Stage(files)
}

func (m *MockApplyService) Apply(_ *entities.Journal) error {
	m.ApplyCalls++
	if m.ApplyErr != nil {
		return m.ApplyErr
	}
	if m.WriteOnApply {
		for path, content := range m.StagedFiles {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(path, content, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *MockApplyService) Recover() error {
	m.RecoverCalls++
	return m.RecoverErr
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

func (m *MockJournalRepository) Save(_ *entities.Journal) error {
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

// MockPromptService is a manual stub for repositories.PromptService.
type MockPromptService struct {
	ToolAction        string // preconfigured return for PromptToolAction
	Confirmation      bool   // preconfigured return for PromptConfirmation
	FileAction        string // preconfigured return for PromptFileAction
	ToolActionCalls   int
	ConfirmationCalls int
	FileActionCalls   int

	// ConflictChoices are returned by PromptSyncConflict in order; once
	// exhausted every further conflict is skipped.
	ConflictChoices []entities.ConflictChoice
	ConflictDetails []entities.ConflictDetail // every detail passed to PromptSyncConflict
	ConflictCalls   int
}

func (m *MockPromptService) PromptToolAction(_ string) string {
	m.ToolActionCalls++
	return m.ToolAction
}

func (m *MockPromptService) PromptConfirmation(_ string) bool {
	m.ConfirmationCalls++
	return m.Confirmation
}

func (m *MockPromptService) PromptSyncConflict(detail entities.ConflictDetail) entities.ConflictChoice {
	m.ConflictCalls++
	m.ConflictDetails = append(m.ConflictDetails, detail)
	if len(m.ConflictChoices) == 0 {
		return entities.ChoiceSkip
	}
	choice := m.ConflictChoices[0]
	m.ConflictChoices = m.ConflictChoices[1:]
	return choice
}

// MockTextDiffer is a manual stub for repositories.TextDiffer. It returns a
// recognisable marker instead of a real diff and records every call.
type MockTextDiffer struct {
	Calls int
}

func (m *MockTextDiffer) Unified(fromLabel, toLabel string, from, to []byte) string {
	m.Calls++
	if string(from) == string(to) {
		return ""
	}
	return "--- " + fromLabel + "\n+++ " + toLabel + "\n-" + string(from) + "\n+" + string(to) + "\n"
}

// MockRepoHistory is a manual stub for repositories.RepoHistory. Changes maps
// repo-relative paths to the commit returned for them; other paths report no
// history.
type MockRepoHistory struct {
	Changes map[string]MockRepoChange
	Err     error
	Calls   int
}

// MockRepoChange is the commit MockRepoHistory reports for one path.
type MockRepoChange struct {
	When    time.Time
	Subject string
}

// LastChange reports the change recorded for the first of relPaths that has
// one.
func (m *MockRepoHistory) LastChange(_ string, relPaths ...string) (time.Time, string, error) {
	m.Calls++
	if m.Err != nil {
		return time.Time{}, "", m.Err
	}
	for _, relPath := range relPaths {
		if change, ok := m.Changes[relPath]; ok {
			return change.When, change.Subject, nil
		}
	}
	return time.Time{}, "", nil
}

func (m *MockPromptService) PromptFileAction(_, _ string) string {
	m.FileActionCalls++
	return m.FileAction
}

// MockForbiddenTermsRepository is a manual stub for
// repositories.ForbiddenTermsRepository. It stores the in-memory term
// list, captures Save calls, and lets tests preconfigure errors per
// method.
type MockForbiddenTermsRepository struct {
	Terms      []entities.ForbiddenTerm
	SavedRepo  string
	SavedTerms []entities.ForbiddenTerm
	LoadErr    error
	SaveErr    error
	LoadCalls  int
	SaveCalls  int
	PathCalls  int
}

func (m *MockForbiddenTermsRepository) Load(_ string) ([]entities.ForbiddenTerm, error) {
	m.LoadCalls++
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	return m.Terms, nil
}

func (m *MockForbiddenTermsRepository) Save(repoPath string, terms []entities.ForbiddenTerm) error {
	m.SaveCalls++
	m.SavedRepo = repoPath
	m.SavedTerms = terms
	if m.SaveErr != nil {
		return m.SaveErr
	}
	m.Terms = terms
	return nil
}

func (m *MockForbiddenTermsRepository) Path(repoPath string) string {
	m.PathCalls++
	return repoPath + "/.aisync-forbidden.age"
}

// MockGitInspector is a manual stub for repositories.GitInspector. Tests
// can preconfigure each method's return values and (optionally) an error
// per method to exercise the per-source error tolerance in AutoDeriver.
type MockGitInspector struct {
	EmailDomainVal     string
	EmailDomainErr     error
	SelfIdentitiesVal  []string
	SelfIdentitiesErr  error
	LocalRemotesVal    []repositories.DerivedTerm
	LocalRemotesErr    error
	DirectoryLayoutVal []repositories.DerivedTerm
	DirectoryLayoutErr error
	SSHHostAliasesVal  []repositories.DerivedTerm
	SSHHostAliasesErr  error

	EmailDomainCalls     int
	SelfIdentitiesCalls  int
	LocalRemotesCalls    int
	DirectoryLayoutCalls int
	SSHHostAliasesCalls  int
}

func (m *MockGitInspector) EmailDomain() (string, error) {
	m.EmailDomainCalls++
	return m.EmailDomainVal, m.EmailDomainErr
}

func (m *MockGitInspector) SelfIdentities() ([]string, error) {
	m.SelfIdentitiesCalls++
	return m.SelfIdentitiesVal, m.SelfIdentitiesErr
}

func (m *MockGitInspector) LocalRemotes(_ []string, _ int) ([]repositories.DerivedTerm, error) {
	m.LocalRemotesCalls++
	return m.LocalRemotesVal, m.LocalRemotesErr
}

func (m *MockGitInspector) DirectoryLayout(_ []string) ([]repositories.DerivedTerm, error) {
	m.DirectoryLayoutCalls++
	return m.DirectoryLayoutVal, m.DirectoryLayoutErr
}

func (m *MockGitInspector) SSHHostAliases() ([]repositories.DerivedTerm, error) {
	m.SSHHostAliasesCalls++
	return m.SSHHostAliasesVal, m.SSHHostAliasesErr
}

// MockBundleService is a manual stub for repositories.BundleService. The
// stub returns canned ciphertext / manifest values configured by tests.
// HashName uses a simple deterministic transform so tests asserting the
// produced filename can predict it. HashErr can be set to simulate
// identity-file failures (e.g. missing AGE-SECRET-KEY entry).
type MockBundleService struct {
	BundleCipher      []byte
	BundleManifest    *entities.BundleManifest
	BundleErr         error
	ExtractManifest   *entities.BundleManifest
	ExtractFiles      []repositories.BundleFile
	ExtractErr        error
	MergeReport       *repositories.BundleMergeReport
	MergeErr          error
	HashErr           error
	BundleCalls       int
	ExtractCalls      int
	MergeCalls        int
	HashCalls         int
	LastBundleSrc     string
	LastBundleName    string
	LastHashIdentity  string
	LastExtractCipher []byte
}

func (m *MockBundleService) HashName(name, identityPath string) (string, error) {
	m.HashCalls++
	m.LastHashIdentity = identityPath
	if m.HashErr != nil {
		return "", m.HashErr
	}
	return "h_" + name, nil
}

func (m *MockBundleService) Bundle(
	src, name string,
	_ []string,
) ([]byte, *entities.BundleManifest, error) {
	m.BundleCalls++
	m.LastBundleSrc = src
	m.LastBundleName = name
	return m.BundleCipher, m.BundleManifest, m.BundleErr
}

func (m *MockBundleService) Extract(
	cipher []byte,
	_ string,
) (*entities.BundleManifest, []repositories.BundleFile, error) {
	m.ExtractCalls++
	m.LastExtractCipher = cipher
	return m.ExtractManifest, m.ExtractFiles, m.ExtractErr
}

func (m *MockBundleService) MergeIntoLocal(
	_ []repositories.BundleFile,
	_ string,
	_ entities.BundleMergeStrategy,
) (*repositories.BundleMergeReport, error) {
	m.MergeCalls++
	return m.MergeReport, m.MergeErr
}

// MockBundleStateRepository is a manual stub for repositories.BundleStateRepository.
type MockBundleStateRepository struct {
	State     *entities.BundleState
	LoadErr   error
	SaveErr   error
	LoadCalls int
	SaveCalls int
	Saved     *entities.BundleState
}

func (m *MockBundleStateRepository) Load() (*entities.BundleState, error) {
	m.LoadCalls++
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	if m.State == nil {
		return entities.NewBundleState(), nil
	}
	return m.State, nil
}

func (m *MockBundleStateRepository) Save(state *entities.BundleState) error {
	m.SaveCalls++
	m.Saved = state
	return m.SaveErr
}

// MockOpSecretRepository is a manual stub for repositories.OpSecretRepository.
type MockOpSecretRepository struct {
	Identity         string
	GetIdentityErr   error
	RequestedVault   string
	RequestedItem    string
	GetIdentityCalls int
}

func (m *MockOpSecretRepository) GetIdentity(vault, item string) (string, error) {
	m.GetIdentityCalls++
	m.RequestedVault = vault
	m.RequestedItem = item
	if m.GetIdentityErr != nil {
		return "", m.GetIdentityErr
	}
	return m.Identity, nil
}

// MockExcludeRepository is a manual stub for repositories.ExcludeRepository.
// Lines are parsed like the exclude file's lines; LoadErr fails every Load.
type MockExcludeRepository struct {
	Lines     []string
	PathVal   string
	LoadErr   error
	LoadCalls int
}

func (m *MockExcludeRepository) Load() (*entities.ExcludePatterns, error) {
	m.LoadCalls++
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	patterns, _ := entities.ParseExcludePatterns([]byte(strings.Join(m.Lines, "\n")))
	return patterns, nil
}

func (m *MockExcludeRepository) Path() string {
	return m.PathVal
}

// MockSSHAliasRepository is a manual stub for repositories.SSHAliasRepository.
type MockSSHAliasRepository struct {
	Aliases             []string
	ResolveAliasesErr   error
	ResolvedHostname    string
	ResolveAliasesCalls int
}

func (m *MockSSHAliasRepository) ResolveAliases(hostname string) ([]string, error) {
	m.ResolveAliasesCalls++
	m.ResolvedHostname = hostname
	return m.Aliases, m.ResolveAliasesErr
}

// MockTranscriptService is an in-memory repositories.TranscriptService with
// real lease semantics: Publish fails with ErrSnapshotMoved when the given
// snapshot was not fetched from the current state.
type MockTranscriptService struct {
	Files map[string]entities.TranscriptFile // current snapshot, by key
	Blobs map[string][]byte                  // stored path -> plaintext
	Lease string                             // current branch commit ("" before the first publish)

	// MoveBeforeNextPublish simulates another device publishing between
	// this device's fetch and publish, once.
	MoveBeforeNextPublish bool
	FetchErr              error
	PublishErr            error

	FetchCalls   int
	ReadCalls    int
	PublishCalls int
	commits      int
}

func (m *MockTranscriptService) Fetch(_, _, _ string) (*entities.TranscriptSnapshot, error) {
	m.FetchCalls++
	if m.FetchErr != nil {
		return nil, m.FetchErr
	}
	snapshot := &entities.TranscriptSnapshot{Lease: m.Lease, Files: make(map[string]entities.TranscriptFile)}
	maps.Copy(snapshot.Files, m.Files)
	return snapshot, nil
}

func (m *MockTranscriptService) Read(_, _ string, file entities.TranscriptFile, _ string) ([]byte, error) {
	m.ReadCalls++
	content, ok := m.Blobs[file.Stored]
	if !ok {
		return nil, errors.New("not stored: " + file.Key)
	}
	return content, nil
}

func (m *MockTranscriptService) Publish(
	_, _ string,
	snapshot *entities.TranscriptSnapshot,
	_ []string,
	_, _ string,
) error {
	m.PublishCalls++
	if m.PublishErr != nil {
		return m.PublishErr
	}
	if m.MoveBeforeNextPublish {
		m.MoveBeforeNextPublish = false
		m.commits++
		m.Lease = "moved-" + strconv.Itoa(m.commits)
		return repositories.ErrSnapshotMoved
	}
	if snapshot.Lease != m.Lease {
		return repositories.ErrSnapshotMoved
	}

	if m.Blobs == nil {
		m.Blobs = make(map[string][]byte)
	}
	m.Files = make(map[string]entities.TranscriptFile, len(snapshot.Files))
	for key, file := range snapshot.Files {
		if file.Content != nil {
			file.Stored = "stored/" + key
			m.Blobs[file.Stored] = file.Content
			file.Content = nil
		}
		m.Files[key] = file
	}
	m.commits++
	m.Lease = "commit-" + strconv.Itoa(m.commits)
	snapshot.Lease = m.Lease
	return nil
}

// Seed stores content under key as if another device had published it.
func (m *MockTranscriptService) Seed(key string, content []byte, modTime time.Time, device string) {
	if m.Files == nil {
		m.Files = make(map[string]entities.TranscriptFile)
	}
	if m.Blobs == nil {
		m.Blobs = make(map[string][]byte)
	}
	stored := "stored/" + key
	m.Blobs[stored] = content
	m.Files[key] = entities.TranscriptFile{
		Key:      key,
		Checksum: "sha256:" + hex.EncodeToString(sha256Sum(content)),
		ModTime:  modTime,
		Device:   device,
		Stored:   stored,
	}
	if m.Lease == "" {
		m.commits++
		m.Lease = "commit-" + strconv.Itoa(m.commits)
	}
}

func sha256Sum(content []byte) []byte {
	sum := sha256.Sum256(content)
	return sum[:]
}

// MockTranscriptBaseRepository is an in-memory
// repositories.TranscriptBaseRepository.
type MockTranscriptBaseRepository struct {
	Base      map[string]string
	LoadErr   error
	SaveErr   error
	SaveCalls int
}

func (m *MockTranscriptBaseRepository) Load(_ string) (map[string]string, error) {
	if m.LoadErr != nil {
		return nil, m.LoadErr
	}
	base := make(map[string]string, len(m.Base))
	maps.Copy(base, m.Base)
	return base, nil
}

func (m *MockTranscriptBaseRepository) Save(_ string, base map[string]string) error {
	m.SaveCalls++
	if m.SaveErr != nil {
		return m.SaveErr
	}
	m.Base = make(map[string]string, len(base))
	maps.Copy(m.Base, base)
	return nil
}
