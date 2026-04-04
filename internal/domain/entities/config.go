package entities

// Config is the top-level configuration for aisync, stored in config.yaml.
type Config struct {
	Sync         SyncConfig          `yaml:"sync"`
	Encryption   EncryptionConfig    `yaml:"encryption"`
	Tools        map[string]Tool     `yaml:"tools"`
	Sources      []Source            `yaml:"sources"`
	Watch        WatchConfig         `yaml:"watch"`
	HooksExclude []HooksExcludeEntry `yaml:"hooks_exclude,omitempty"`
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
