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
type Tool struct {
	Path           string   `yaml:"path"`
	Enabled        bool     `yaml:"enabled"`
	ExtraAllowlist []string `yaml:"extra_allowlist,omitempty"`
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
