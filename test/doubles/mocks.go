//go:build unit

package doubles

import (
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

func (m *MockConfigRepository) Load(path string) (*entities.Config, error) {
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
	Result          *repositories.FetchResult
	ResultsBySource map[string]*repositories.FetchResult // keyed by source name
	FetchErr        error
	FetchedSources  []*entities.Source
	FetchCalls      int
}

func (m *MockSourceRepository) Fetch(source *entities.Source, hints repositories.CacheHints) (*repositories.FetchResult, error) {
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
	AddRemoteName  string
	AddRemoteURL   string
	AddRemoteErr   error
	AddRemoteCalls int
	SetConfigKey   string
	SetConfigValue string
	SetConfigErr   error
	SetConfigCalls int
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

func (m *MockEncryptionService) Encrypt(plaintext []byte, recipients []string) ([]byte, error) {
	m.EncryptCalls++
	m.EncryptPlaintext = plaintext
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

func (m *MockWatchService) Watch(trees []repositories.WatchedTree, callback func(event repositories.FileEvent)) error {
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
	Conflicts    []entities.Conflict
	DetectErr    error
	ResolveErr   error
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

// MockPromptService is a manual stub for repositories.PromptService.
type MockPromptService struct {
	ToolAction         string // preconfigured return for PromptToolAction
	Confirmation       bool   // preconfigured return for PromptConfirmation
	ConflictResolution string // preconfigured return for PromptConflictResolution
	FileAction         string // preconfigured return for PromptFileAction
	ToolActionCalls    int
	ConfirmationCalls  int
	ConflictCalls      int
	FileActionCalls    int
}

func (m *MockPromptService) PromptToolAction(_ string) string {
	m.ToolActionCalls++
	return m.ToolAction
}

func (m *MockPromptService) PromptConfirmation(_ string) bool {
	m.ConfirmationCalls++
	return m.Confirmation
}

func (m *MockPromptService) PromptConflictResolution(_, _ string) string {
	m.ConflictCalls++
	return m.ConflictResolution
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
