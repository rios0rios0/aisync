package entities

// Tool represents an AI coding assistant whose configuration is managed by aisync.
//
// ExtraAllowlist is an optional user-supplied list of gitwildmatch-style glob
// patterns (tool-relative) that extend the compiled-in allowlist for this
// tool. Matched paths are allowed to sync in addition to whatever is in
// [ToolAllowlists] or [DefaultAllowlist]. It lets users opt in to syncing
// files that the shipped allowlist deliberately omits (e.g. a custom
// research subdirectory) without editing aisync source code. Leave nil to
// rely purely on the compiled-in list — which is what almost everyone wants.
//
// Bundles is an optional list of opaque-bundle specs. Each entry tells
// aisync to walk the immediate subdirectories of <tool path>/<source> and
// produce one age-encrypted tarball per subdirectory under
// personal/<tool>/<target>/<sha256(name)[:16]>.age. The bundle's filename
// is intentionally a hash so the source directory name (which can leak
// project paths or company codenames) never appears in the git tree. On
// pull, each bundle is decrypted and its contents are merged into the
// matching local subdirectory using the configured strategy.
type Tool struct {
	Path           string       `yaml:"path"`
	Enabled        bool         `yaml:"enabled"`
	ExtraAllowlist []string     `yaml:"extra_allowlist,omitempty"`
	Bundles        []BundleSpec `yaml:"bundles,omitempty"`
}

// BundleMergeStrategy controls how an extracted bundle merges into a local
// directory on pull.
type BundleMergeStrategy string

const (
	// BundleMergeMTime keeps whichever copy of a file has the newer mtime,
	// preserves files that exist only locally, and adds files that exist
	// only in the bundle. Pragmatic default for memory-style append-mostly
	// content.
	BundleMergeMTime BundleMergeStrategy = "mtime"
	// BundleMergeReplace overwrites local content with the bundle. Loses
	// any unsynced local edits. Available for users who want bundle-first
	// semantics.
	BundleMergeReplace BundleMergeStrategy = "replace"
)

// BundlePruneStrategy controls when a bundle whose source directory no
// longer exists locally is removed from the repo.
type BundlePruneStrategy string

const (
	// BundlePruneManual keeps orphan bundles until the user runs
	// `aisync bundles prune`. Safer default — a transient `rm -rf` of a
	// projects directory should not propagate as a remote deletion.
	BundlePruneManual BundlePruneStrategy = "manual"
)

// BundleSpec declares a directory whose immediate subdirectories should
// each be packaged into one age-encrypted tarball during push and merged
// back into the matching local subdirectory during pull. Source and Target
// are tool-relative paths (so a Source of "projects" under the claude tool
// resolves to ~/.claude/projects/). MergeStrategy and Prune fall back to
// the BundleMerge*/BundlePrune* defaults when empty.
type BundleSpec struct {
	Source        string              `yaml:"source"`
	Target        string              `yaml:"target"`
	MergeStrategy BundleMergeStrategy `yaml:"merge_strategy,omitempty"`
	Prune         BundlePruneStrategy `yaml:"prune,omitempty"`
}

// EffectiveMergeStrategy returns the configured merge strategy or the
// safe default ([BundleMergeMTime]) when none is set, so callers do not
// need to repeat the fallback at every site.
func (b BundleSpec) EffectiveMergeStrategy() BundleMergeStrategy {
	if b.MergeStrategy == "" {
		return BundleMergeMTime
	}
	return b.MergeStrategy
}

// EffectivePrune returns the configured prune strategy or the safe
// default ([BundlePruneManual]) when none is set.
func (b BundleSpec) EffectivePrune() BundlePruneStrategy {
	if b.Prune == "" {
		return BundlePruneManual
	}
	return b.Prune
}

// DefaultTools returns the Tier 1 AI tools that aisync detects out of the box.
func DefaultTools() map[string]Tool {
	return map[string]Tool{
		"claude":      {Path: "~/.claude", Enabled: true},
		"cursor":      {Path: "~/.cursor", Enabled: true},
		"copilot":     {Path: "~/.github", Enabled: true},
		"codex":       {Path: "~/.codex", Enabled: true},
		"gemini":      {Path: "~/.gemini", Enabled: false},
		"windsurf":    {Path: "~/.codeium/windsurf", Enabled: false},
		"cline":       {Path: "~/.cline", Enabled: false},
		"roo":         {Path: "~/.roo", Enabled: false},
		"continue":    {Path: "~/.continue", Enabled: false},
		"trae":        {Path: "~/.trae", Enabled: false},
		"amazonq":     {Path: "~/.amazonq", Enabled: false},
		"kilo":        {Path: "~/.config/kilo", Enabled: false},
		"opencode":    {Path: "~/.config/opencode", Enabled: false},
		"kiro":        {Path: "~/.kiro", Enabled: false},
		"factory":     {Path: "~/.factory", Enabled: false},
		"augment":     {Path: "~/.augment", Enabled: false},
		"tabnine":     {Path: "~/.tabnine", Enabled: false},
		"qwen":        {Path: "~/.qwen", Enabled: false},
		"rovodev":     {Path: "~/.rovodev", Enabled: false},
		"deepagents":  {Path: "~/.deepagents", Enabled: false},
		"warp":        {Path: "~/.warp", Enabled: false},
		"goose":       {Path: "~/.config/goose", Enabled: false},
		"zed":         {Path: "~/.config/zed", Enabled: false},
		"aider":       {Path: "~/.aider", Enabled: false},
		"junie":       {Path: "~/.junie", Enabled: false},
		"amp":         {Path: "~/.amp", Enabled: false},
		"replit":      {Path: "~/.replit", Enabled: false},
		"blackbox":    {Path: "~/.blackbox", Enabled: false},
		"openclaw":    {Path: "~/.openclaw", Enabled: false},
		"antigravity": {Path: "~/.antigravity", Enabled: false},
		"copilot-cli": {Path: "~/.copilot", Enabled: false},
	}
}
