package entities

import "time"

// BundleManifestFileName is the well-known path of the manifest entry
// inside every bundle tarball. Pull-side code reads this entry first to
// recover the original source directory name (which is hashed in the
// .age filename to keep the git tree opaque).
const BundleManifestFileName = "_aisync-manifest.json"

// BundleManifest is the in-tarball metadata describing one project
// bundle. It survives across the age round-trip and is what allows the
// pull side to map an opaque <hash>.age back to the original directory.
type BundleManifest struct {
	OriginalName string    `json:"original_name"`
	CreatedAt    time.Time `json:"created_at"`
	FileCount    int       `json:"file_count"`
	SchemaVer    int       `json:"schema_version"`
}

// CurrentBundleSchemaVersion is the schema number written into new
// manifests. Increment on backwards-incompatible changes so the pull
// side can refuse to extract bundles it does not understand instead of
// silently producing wrong results.
const CurrentBundleSchemaVersion = 1
