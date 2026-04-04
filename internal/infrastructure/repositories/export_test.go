package repositories

import "net/http"

// SetHTTPSourceClient sets the HTTP client on an HTTPSourceRepository for testing.
func SetHTTPSourceClient(r *HTTPSourceRepository, c *http.Client) {
	r.client = c
}

// MatchesMapping exposes matchesMapping for black-box testing.
var MatchesMapping = matchesMapping

// RemapPath exposes remapPath for black-box testing.
var RemapPath = remapPath
