package entities

const (
	// ChangeTrailerKey is the git trailer aisync adds to a commit that changed
	// how files are stored without changing what they contain.
	ChangeTrailerKey = "Aisync-Change"
	// ChangeEncryptionOnly marks a commit that only encrypted files in place.
	// History lookups skip it, so a file keeps the date of its last real
	// change instead of the date it was encrypted.
	ChangeEncryptionOnly = "encryption-only"
)
