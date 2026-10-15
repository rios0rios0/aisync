package repositories

// TextDiffer renders the line differences between two versions of a file.
type TextDiffer interface {
	// Unified returns a unified diff with three lines of context that turns
	// from into to, labelling the two sides with fromLabel and toLabel. It
	// returns an empty string when the contents are identical.
	Unified(fromLabel, toLabel string, from, to []byte) string
}
