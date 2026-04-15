package entities

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ForbiddenTermMode controls how a single term is compared against file content.
type ForbiddenTermMode int

const (
	// ForbiddenModeCanonical matches the canonical form of the term as a
	// substring of the canonical form of each line. Canonicalization lowercases
	// the text, strips accents via NFKD, and removes every non-alphanumeric
	// rune — so "ZestSecurity" catches "zest-security", "Zest Security",
	// "ZEST_SECURITY", "zést-sécurity", and every other normal writing
	// variant without the user enumerating them. This is the default mode
	// and fits the 90% use case for company-name and codename detection.
	ForbiddenModeCanonical ForbiddenTermMode = iota

	// ForbiddenModeCanonicalWord is like [ForbiddenModeCanonical] but also
	// requires the match to start AND end at a word boundary in the original
	// text. It exists for short or ambiguous terms where substring matching
	// would hit common English words (e.g. "QA" matching "aquarium"). Users
	// opt in by prefixing the term with `word:` in the forbidden file.
	ForbiddenModeCanonicalWord

	// ForbiddenModeRegex runs the raw pattern as a Go regular expression
	// against each line, case-insensitive by default. It bypasses
	// canonicalization so power users can express shape-based matches.
	// Users opt in by prefixing the term with `regex:` in the forbidden file.
	ForbiddenModeRegex
)

// ForbiddenTerm is a single pattern the user has asked aisync to block, or a
// term that auto-derivation or a heuristic has produced at push time. The
// Kind field identifies the source so block messages can tell the user which
// knob to turn to fix a finding.
type ForbiddenTerm struct {
	// Original is the raw term as the user wrote it (or as auto-derivation
	// extracted it). Used verbatim in block messages so the user sees what
	// they actually wrote, not the canonicalized form.
	Original string

	// Mode controls how this term matches content.
	Mode ForbiddenTermMode

	// Kind is a human-readable tag describing where this term came from.
	// Examples: "user", "auto-derived:git-remote:dev.azure.com",
	// "heuristic:home-path". Rendered verbatim in block messages.
	Kind string

	// canonical is the precomputed canonical form used for substring and
	// word-boundary matching. Empty when Mode == ForbiddenModeRegex.
	canonical string

	// regex is the precompiled pattern for Mode == ForbiddenModeRegex.
	regex *regexp.Regexp
}

// NewCanonicalTerm builds a [ForbiddenModeCanonical] term from raw text.
// The term is considered invalid (returns an error) if it canonicalizes to
// an empty string (e.g. the term was only whitespace or punctuation).
func NewCanonicalTerm(original, kind string) (ForbiddenTerm, error) {
	canonical := Canonicalize(original)
	if canonical == "" {
		return ForbiddenTerm{}, fmt.Errorf("forbidden term %q has empty canonical form", original)
	}
	return ForbiddenTerm{
		Original:  original,
		Mode:      ForbiddenModeCanonical,
		Kind:      kind,
		canonical: canonical,
	}, nil
}

// NewCanonicalWordTerm is like [NewCanonicalTerm] but sets
// [ForbiddenModeCanonicalWord] so matching requires word boundaries in the
// original text on both sides of the canonical match.
func NewCanonicalWordTerm(original, kind string) (ForbiddenTerm, error) {
	term, err := NewCanonicalTerm(original, kind)
	if err != nil {
		return ForbiddenTerm{}, err
	}
	term.Mode = ForbiddenModeCanonicalWord
	return term, nil
}

// NewRegexTerm compiles a raw Go regex pattern (with implicit (?i) case
// insensitivity) for use as a [ForbiddenModeRegex] term. Returns an error if
// the pattern fails to compile.
func NewRegexTerm(pattern, kind string) (ForbiddenTerm, error) {
	compiled, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return ForbiddenTerm{}, fmt.Errorf("invalid regex %q: %w", pattern, err)
	}
	return ForbiddenTerm{
		Original: pattern,
		Mode:     ForbiddenModeRegex,
		Kind:     kind,
		regex:    compiled,
	}, nil
}

// Canonicalize lowercases, NFKD-decomposes, strips accents (combining marks)
// and every non-alphanumeric rune, returning a string containing only lowercase
// letters and digits. It is the engine behind canonical-form substring
// matching: "Zést Security" and "zest-security" both canonicalize to
// "zestsecurity". Exported so tests and the scanner pipeline can share a
// single implementation.
func Canonicalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFKD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			// Skip combining marks (accents decomposed by NFKD).
			continue
		}
		r = unicode.ToLower(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ForbiddenTerms is a list of compiled terms plus the canonical form of
// each content line the scanner has seen. Scanning a file reuses the
// canonicalized lines across all terms so the cost is O(lines × terms)
// with the canonicalization paid once per line.
type ForbiddenTerms struct {
	Terms []ForbiddenTerm
}

// NDAFinding describes a single forbidden-term hit inside a file. A single
// line can produce multiple findings if multiple terms match.
type NDAFinding struct {
	// Path is the file path reported to the user. In push flow this is the
	// repo-relative personal/<tool>/<rel> form so the user can navigate
	// straight to the offending file.
	Path string
	// Line is 1-indexed.
	Line int
	// Term is the original pattern text, not the canonical form.
	Term string
	// Kind is the source tag from [ForbiddenTerm.Kind].
	Kind string
	// Snippet is the matching portion of the original line, trimmed to a
	// reasonable display length.
	Snippet string
}

// Match runs every term in the list against the content and returns all
// findings. Lines are canonicalized once and re-used across canonical and
// canonical-word terms. Regex terms operate on the raw line.
func (f *ForbiddenTerms) Match(path string, content []byte) []NDAFinding {
	if f == nil || len(f.Terms) == 0 {
		return nil
	}

	var findings []NDAFinding
	lines := strings.Split(string(content), "\n")
	for lineIdx, line := range lines {
		lineNum := lineIdx + 1
		canonicalLine := Canonicalize(line)

		for _, term := range f.Terms {
			finding, ok := matchTermAgainstLine(term, path, lineNum, line, canonicalLine)
			if ok {
				findings = append(findings, finding)
			}
		}
	}
	return findings
}

// matchTermAgainstLine applies a single term to a single already-canonicalized
// line and returns a finding if it matches.
func matchTermAgainstLine(
	term ForbiddenTerm,
	path string,
	lineNum int,
	originalLine, canonicalLine string,
) (NDAFinding, bool) {
	switch term.Mode {
	case ForbiddenModeCanonical:
		if strings.Contains(canonicalLine, term.canonical) {
			return NDAFinding{
				Path:    path,
				Line:    lineNum,
				Term:    term.Original,
				Kind:    term.Kind,
				Snippet: clampSnippet(originalLine),
			}, true
		}
	case ForbiddenModeCanonicalWord:
		if matchesCanonicalWord(canonicalLine, originalLine, term.canonical) {
			return NDAFinding{
				Path:    path,
				Line:    lineNum,
				Term:    term.Original,
				Kind:    term.Kind,
				Snippet: clampSnippet(originalLine),
			}, true
		}
	case ForbiddenModeRegex:
		if loc := term.regex.FindStringIndex(originalLine); loc != nil {
			return NDAFinding{
				Path:    path,
				Line:    lineNum,
				Term:    term.Original,
				Kind:    term.Kind,
				Snippet: clampSnippet(originalLine[loc[0]:loc[1]]),
			}, true
		}
	}
	return NDAFinding{}, false
}

// matchesCanonicalWord checks whether the canonical-form term appears in the
// canonical line AND the match corresponds to a word-boundary-delimited span
// in the original line. Since canonicalization strips separators, a canonical
// substring match is always a legitimate "word" match in the canonical space;
// we only need to confirm that the ORIGINAL line doesn't smoosh the match
// into a larger alphanumeric run. We do that by canonicalizing the original
// line char-by-char and checking that the index range before the match and
// after the match in the canonical form corresponds to either start/end of
// line or a non-alphanumeric rune in the original.
func matchesCanonicalWord(canonicalLine, originalLine, canonicalTerm string) bool {
	if canonicalTerm == "" {
		return false
	}
	// Build a mapping from canonical-index → original-rune-index so we can
	// inspect the neighbors of a match in the original text.
	originalRunes := []rune(originalLine)
	origIdxOfCanon := make([]int, 0, len(canonicalLine))
	for origIdx, r := range norm.NFKD.String(originalLine) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		r = unicode.ToLower(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			origIdxOfCanon = append(origIdxOfCanon, origIdx)
		}
	}
	// Note: origIdxOfCanon[i] is a BYTE offset into the NFKD-decomposed
	// original. For rune-level word-boundary checks we re-index into
	// originalRunes by counting runes up to that byte offset.
	search := 0
	for {
		hit := strings.Index(canonicalLine[search:], canonicalTerm)
		if hit < 0 {
			return false
		}
		canonStart := search + hit
		canonEnd := canonStart + len(canonicalTerm)
		if canonStart >= len(origIdxOfCanon) || canonEnd-1 >= len(origIdxOfCanon) {
			return false
		}
		// Check the rune immediately before and after the match in the
		// original line. If either is alphanumeric, the match is inside a
		// larger word (e.g. "qa" inside "aquarium") and does not satisfy
		// the word-boundary requirement.
		beforeByte := origIdxOfCanon[canonStart] - 1
		afterByte := origIdxOfCanon[canonEnd-1] + 1
		if isAtWordBoundary(originalRunes, originalLine, beforeByte, afterByte) {
			return true
		}
		search = canonStart + 1
	}
}

// isAtWordBoundary checks the bytes on either side of a match span in the
// original line. Out-of-range (start/end of line) counts as a boundary; any
// non-alphanumeric rune counts as a boundary.
func isAtWordBoundary(originalRunes []rune, originalLine string, beforeByte, afterByte int) bool {
	leftOK := true
	if beforeByte >= 0 && beforeByte < len(originalLine) {
		r := runeAt(originalRunes, originalLine, beforeByte)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			leftOK = false
		}
	}
	rightOK := true
	if afterByte >= 0 && afterByte < len(originalLine) {
		r := runeAt(originalRunes, originalLine, afterByte)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			rightOK = false
		}
	}
	return leftOK && rightOK
}

// runeAt returns the rune at a given byte offset in the original line,
// walking the rune slice. Safe when the offset points into the middle of a
// multi-byte rune: the search in the rune slice will return the rune that
// contains the byte.
func runeAt(originalRunes []rune, originalLine string, byteOffset int) rune {
	// Count runes up to byteOffset.
	byteCount := 0
	for _, r := range originalRunes {
		next := byteCount + len(string(r))
		if byteOffset < next {
			return r
		}
		byteCount = next
	}
	if len(originalRunes) > 0 {
		return originalRunes[len(originalRunes)-1]
	}
	// Fall back: decode from the string at the byte offset.
	for _, r := range originalLine[byteOffset:] {
		return r
	}
	return 0
}

// clampSnippet trims a line to a reasonable display length so block
// messages stay readable even for very long lines.
func clampSnippet(s string) string {
	const maxLen = 120
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ParseForbiddenTermsFile parses the user-facing forbidden-terms syntax
// (plain lines, `#` comments, `word:` prefix, `regex:` prefix) into a list
// of compiled ForbiddenTerm entries with Kind="user". Returns an error on
// the FIRST invalid regex or empty-canonical term so users know exactly
// which line to fix.
func ParseForbiddenTermsFile(content []byte) ([]ForbiddenTerm, error) {
	var terms []ForbiddenTerm
	for lineNum, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		term, err := parseSingleTerm(line, "user")
		if err != nil {
			return nil, fmt.Errorf("forbidden-terms line %d: %w", lineNum+1, err)
		}
		terms = append(terms, term)
	}
	return terms, nil
}

// parseSingleTerm parses one non-blank, non-comment line into a ForbiddenTerm.
func parseSingleTerm(line, kind string) (ForbiddenTerm, error) {
	switch {
	case strings.HasPrefix(line, "regex:"):
		return NewRegexTerm(strings.TrimPrefix(line, "regex:"), kind)
	case strings.HasPrefix(line, "word:"):
		return NewCanonicalWordTerm(strings.TrimPrefix(line, "word:"), kind)
	default:
		return NewCanonicalTerm(line, kind)
	}
}
