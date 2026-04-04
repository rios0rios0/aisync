package entities

import "time"

// Manifest tracks all files managed by aisync in a single AI tool directory.
// It is written to .aisync-manifest.json in each managed directory.
type Manifest struct {
	ManagedBy string                  `json:"managed_by"`
	Version   string                  `json:"version"`
	LastSync  time.Time               `json:"last_sync"`
	Device    string                  `json:"device"`
	Files     map[string]ManifestFile `json:"files"`
}

// ManifestFile records the provenance of a single managed file.
type ManifestFile struct {
	Source    string `json:"source"`
	Namespace string `json:"namespace"`
	Checksum  string `json:"checksum"`
}

// NewManifest creates a new empty manifest for the given device.
func NewManifest(version, device string) *Manifest {
	return &Manifest{
		ManagedBy: "aisync",
		Version:   version,
		LastSync:  time.Now(),
		Device:    device,
		Files:     make(map[string]ManifestFile),
	}
}

// SetFile records a managed file in the manifest.
func (m *Manifest) SetFile(relativePath, source, namespace, checksum string) {
	m.Files[relativePath] = ManifestFile{
		Source:    source,
		Namespace: namespace,
		Checksum:  checksum,
	}
}

// RemoveFile removes a file from the manifest.
func (m *Manifest) RemoveFile(relativePath string) {
	delete(m.Files, relativePath)
}
