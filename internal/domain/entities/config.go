package entities

// Config is the top-level configuration for aisync, stored in config.yaml.
type Config struct {
	Sync         SyncConfig          `yaml:"sync"`
	Encryption   EncryptionConfig    `yaml:"encryption"`
	Tools        map[string]Tool     `yaml:"tools"`
	Sources      []Source            `yaml:"sources"`
	Watch        WatchConfig         `yaml:"watch"`
	HooksExclude []HooksExcludeEntry `yaml:"hooks_exclude,omitempty"`
	NDA          NDAConfig           `yaml:"nda,omitempty"`
}

// NDAConfig holds knobs for the NDA content scanner. Every field is
// optional and zero-valued fields use the documented defaults.
type NDAConfig struct {
	// AutoDerive controls whether push-time auto-derivation extracts
	// forbidden-term candidates from machine state (git remotes,
	// `~/.gitconfig`, `~/.ssh/config`, dev directory layouts). Default
	// true. Users who want a pure-explicit-list experience can set this
	// to false.
	AutoDerive *bool `yaml:"auto_derive,omitempty"`

	// AutoDeriveExclude is a list of user-provided strings that should
	// be removed from the auto-derived set even when machine state
	// exposes them. Values are canonicalized at comparison time rather
	// than being stored canonically in config.yaml. Use for false
	// positives (e.g., the user has a personal open-source project under
	// `~/Development/github.com/<some-org>/` that shouldn't be treated
	// as NDA).
	AutoDeriveExclude []string `yaml:"auto_derive_exclude,omitempty"`

	// Heuristics controls whether compile-time content-shape checks
	// (home-path, WSL path, ADO org URL, SSH host alias) run during
	// push scans. Default true.
	Heuristics *bool `yaml:"heuristics,omitempty"`

	// DevRoots overrides the default list of directories auto-derivation
	// walks when discovering git repos. Empty slice uses the built-in
	// defaults from [services.DefaultDevRoots].
	DevRoots []string `yaml:"dev_roots,omitempty"`
}

// AutoDeriveEnabled reports whether auto-derivation is enabled. Pointer
// default (nil) is treated as true so users who never set the field in
// their config.yaml get the secure-by-default behavior.
func (c NDAConfig) AutoDeriveEnabled() bool {
	if c.AutoDerive == nil {
		return true
	}
	return *c.AutoDerive
}

// HeuristicsEnabled reports whether the compile-time heuristics should
// run. Same pointer-default semantics as AutoDeriveEnabled.
func (c NDAConfig) HeuristicsEnabled() bool {
	if c.Heuristics == nil {
		return true
	}
	return *c.Heuristics
}

// HooksExcludeEntry defines a hook event that should be excluded from execution
// when the matcher pattern matches.
type HooksExcludeEntry struct {
	Event   string `yaml:"event"`
	Matcher string `yaml:"matcher"`
	Command string `yaml:"command"`
}

// SyncConfig holds settings for the Git sync repository.
type SyncConfig struct {
	Remote       string `yaml:"remote"`
	Branch       string `yaml:"branch"`
	AutoPush     bool   `yaml:"auto_push"`
	Debounce     string `yaml:"debounce"`
	CommitPrefix string `yaml:"commit_prefix"`
}

// EncryptionConfig holds age encryption settings.
type EncryptionConfig struct {
	Identity   string   `yaml:"identity"`
	Recipients []string `yaml:"recipients"`
}

// WatchConfig holds file-watching settings.
type WatchConfig struct {
	PollingInterval string   `yaml:"polling_interval"`
	IgnoredPatterns []string `yaml:"ignored_patterns"`
}
