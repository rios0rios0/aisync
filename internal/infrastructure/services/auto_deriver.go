package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/aisync/internal/domain/entities"
	"github.com/rios0rios0/aisync/internal/domain/repositories"
)

// AutoDeriveCacheTTL controls how long auto-derivation caches machine-state
// extractions before re-running the inspector. Short enough to pick up new
// git remotes within a reasonable window, long enough that back-to-back
// pushes don't re-scan the filesystem.
const AutoDeriveCacheTTL = time.Hour

// cacheLineFields is the number of tab-delimited columns in the on-disk
// cache format: `<canonical>\t<original>\t<kind>`. Defined as a constant
// so the writer and parser cannot drift apart.
const cacheLineFields = 3

// DefaultDevRoots is the built-in set of directories auto-derivation scans
// for git repos. Users can override via `config.yaml: nda.dev_roots`.
var DefaultDevRoots = []string{ //nolint:gochecknoglobals // compile-time default
	"~/Development",
	"~/workspace",
	"~/code",
	"~/src",
	"~/projects",
	"~/go/src",
}

// DefaultAutoDeriveDepth is the maximum number of directory levels below
// each dev root that auto-derivation will walk when discovering git repos.
// 4 is enough for `~/Development/dev.azure.com/<org>/<project>/<repo>/`
// which is the deepest common layout.
const DefaultAutoDeriveDepth = 4

// AutoDeriver extracts forbidden-term candidates from machine state via a
// [repositories.GitInspector] and caches the result on disk so repeated
// `aisync push` invocations don't re-scan the filesystem every time. The
// cache lives at `~/.cache/aisync/derived-terms.txt` with `0600` perms and
// is never committed to the sync repo.
type AutoDeriver struct {
	inspector repositories.GitInspector
	cachePath string
	ttl       time.Duration
}

// NewAutoDeriver builds an AutoDeriver wired to the given git inspector.
// The cache path defaults to `~/.cache/aisync/derived-terms.txt`; tests
// can override it via [AutoDeriver.WithCachePath].
func NewAutoDeriver(inspector repositories.GitInspector) *AutoDeriver {
	return &AutoDeriver{
		inspector: inspector,
		cachePath: defaultCachePath(),
		ttl:       AutoDeriveCacheTTL,
	}
}

// WithCachePath overrides the cache location. Intended for tests.
func (d *AutoDeriver) WithCachePath(path string) *AutoDeriver {
	d.cachePath = path
	return d
}

// WithTTL overrides the cache TTL. Intended for tests.
func (d *AutoDeriver) WithTTL(ttl time.Duration) *AutoDeriver {
	d.ttl = ttl
	return d
}

// DeriveTerms returns the current set of machine-state forbidden terms.
// If the cache is fresh (within TTL and newer than every inspected source
// file) it returns the cached result. Otherwise it re-runs the inspector,
// rewrites the cache, and returns the fresh result.
//
// `devRoots` defaults to [DefaultDevRoots] when nil/empty.
// `excludes` is a list of canonical-form strings the caller wants
// filtered out (user's own github login, `nda.auto_derive_exclude`, etc.).
func (d *AutoDeriver) DeriveTerms(devRoots []string, excludes []string) ([]entities.ForbiddenTerm, error) {
	roots := devRoots
	if len(roots) == 0 {
		roots = DefaultDevRoots
	}
	if cached, ok := d.loadCache(); ok {
		logger.Debugf("auto-derive: using cache at %s (%d terms)", d.cachePath, len(cached))
		return d.applyExcludes(cached, excludes), nil
	}

	fresh := d.runInspector(roots)
	if saveErr := d.saveCache(fresh); saveErr != nil {
		logger.Warnf("auto-derive: could not write cache at %s: %v", d.cachePath, saveErr)
	}
	return d.applyExcludes(fresh, excludes), nil
}

// runInspector runs the full machine-state extraction pipeline. Errors
// from individual sources are logged at debug and tolerated — a missing
// `~/.ssh/config` should not break auto-derivation.
func (d *AutoDeriver) runInspector(devRoots []string) []entities.ForbiddenTerm {
	seen := make(map[string]entities.ForbiddenTerm)
	self, identityErr := d.inspector.SelfIdentities()
	if identityErr != nil {
		logger.Debugf("auto-derive: failed to read self identities: %v", identityErr)
	}
	selfSet := toSet(self)

	// 1. Email domain
	domain, domainErr := d.inspector.EmailDomain()
	if domainErr != nil {
		logger.Debugf("auto-derive: failed to read email domain: %v", domainErr)
	} else if domain != "" {
		d.addDerived(seen, selfSet, repositories.DerivedTerm{
			Value:  domain,
			Origin: "gitconfig:user.email",
		})
	}

	// 2. Git remotes
	remotes, remotesErr := d.inspector.LocalRemotes(devRoots, DefaultAutoDeriveDepth)
	if remotesErr != nil {
		logger.Debugf("auto-derive: failed to enumerate local remotes: %v", remotesErr)
	}
	for _, term := range remotes {
		d.addDerived(seen, selfSet, term)
	}

	// 3. Directory layout
	layout, layoutErr := d.inspector.DirectoryLayout(devRoots)
	if layoutErr != nil {
		logger.Debugf("auto-derive: failed to read directory layout: %v", layoutErr)
	}
	for _, term := range layout {
		d.addDerived(seen, selfSet, term)
	}

	// 4. SSH host aliases
	aliases, aliasErr := d.inspector.SSHHostAliases()
	if aliasErr != nil {
		logger.Debugf("auto-derive: failed to read ssh config: %v", aliasErr)
	}
	for _, term := range aliases {
		d.addDerived(seen, selfSet, term)
	}

	return toSortedSlice(seen)
}

// addDerived inserts a derived term into the seen map, skipping self
// identities and empty values, and building a [entities.ForbiddenTerm]
// whose Kind is the origin tag.
func (d *AutoDeriver) addDerived(
	seen map[string]entities.ForbiddenTerm,
	selfSet map[string]struct{},
	derived repositories.DerivedTerm,
) {
	if derived.Value == "" {
		return
	}
	canon := entities.Canonicalize(derived.Value)
	if canon == "" {
		return
	}
	if _, isSelf := selfSet[canon]; isSelf {
		return
	}
	if _, already := seen[canon]; already {
		return
	}
	term, err := entities.NewCanonicalTerm(derived.Value, "auto-derived:"+derived.Origin)
	if err != nil {
		return
	}
	seen[canon] = term
}

// applyExcludes filters out any term whose canonical form matches an entry
// in excludes. Used for `nda.auto_derive_exclude`.
func (d *AutoDeriver) applyExcludes(terms []entities.ForbiddenTerm, excludes []string) []entities.ForbiddenTerm {
	if len(excludes) == 0 {
		return terms
	}
	excludeSet := toSet(excludes)
	out := terms[:0:len(terms)]
	for _, term := range terms {
		if _, skip := excludeSet[entities.Canonicalize(term.Original)]; skip {
			continue
		}
		out = append(out, term)
	}
	return out
}

// toSet builds a canonical-form set from a list of raw strings.
func toSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		canon := entities.Canonicalize(v)
		if canon != "" {
			out[canon] = struct{}{}
		}
	}
	return out
}

// toSortedSlice flattens the seen map into a deterministic slice.
func toSortedSlice(seen map[string]entities.ForbiddenTerm) []entities.ForbiddenTerm {
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]entities.ForbiddenTerm, 0, len(seen))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out
}

// loadCache reads the cache file if it exists and is newer than TTL.
// Returns (terms, true) on cache hit, (nil, false) on miss. Malformed
// cache entries are silently skipped — cache corruption is never fatal.
func (d *AutoDeriver) loadCache() ([]entities.ForbiddenTerm, bool) {
	info, err := os.Stat(d.cachePath)
	if err != nil {
		return nil, false
	}
	if time.Since(info.ModTime()) > d.ttl {
		return nil, false
	}
	content, err := os.ReadFile(d.cachePath)
	if err != nil {
		return nil, false
	}
	return parseCachedTerms(content), true
}

// saveCache writes the terms to disk in a tab-delimited format:
// `<canonical>\t<original>\t<kind>` per line. Wiped every TTL.
func (d *AutoDeriver) saveCache(terms []entities.ForbiddenTerm) error {
	if err := os.MkdirAll(filepath.Dir(d.cachePath), 0700); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}
	var b strings.Builder
	for _, term := range terms {
		b.WriteString(entities.Canonicalize(term.Original))
		b.WriteString("\t")
		b.WriteString(term.Original)
		b.WriteString("\t")
		b.WriteString(term.Kind)
		b.WriteString("\n")
	}
	return os.WriteFile(d.cachePath, []byte(b.String()), 0600)
}

// parseCachedTerms reads the tab-delimited cache format back into a
// slice of ForbiddenTerm. Empty/malformed lines are skipped.
func parseCachedTerms(content []byte) []entities.ForbiddenTerm {
	var out []entities.ForbiddenTerm
	for line := range strings.SplitSeq(string(content), "\n") {
		fields := strings.SplitN(line, "\t", cacheLineFields)
		if len(fields) != cacheLineFields {
			continue
		}
		term, err := entities.NewCanonicalTerm(fields[1], fields[2])
		if err != nil {
			continue
		}
		out = append(out, term)
	}
	return out
}

// defaultCachePath returns the canonical cache location
// (`~/.cache/aisync/derived-terms.txt`), falling back to `/tmp` if the
// home directory cannot be resolved.
func defaultCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "aisync-derived-terms.txt")
	}
	return filepath.Join(home, ".cache", "aisync", "derived-terms.txt")
}
