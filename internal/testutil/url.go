package testutil

import (
	"net/url"
	"testing"
)

func MustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("Parsing %q failed: %v", raw, err)
	}

	return parsed
}
