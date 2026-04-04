package repositories

// SecretFinding represents a potential secret detected in a file.
type SecretFinding struct {
	Path        string
	Line        int
	Pattern     string
	Description string
}

// SecretScanner defines the contract for scanning files for leaked secrets.
type SecretScanner interface {
	// Scan checks the given files for common secret patterns and returns any findings.
	// Files that are marked for encryption should NOT be passed to this scanner.
	Scan(files map[string][]byte) []SecretFinding
}
