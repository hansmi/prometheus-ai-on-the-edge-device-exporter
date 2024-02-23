package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestProbeHandler(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(ts.Close)

	for _, tc := range []struct {
		name           string
		target         string
		wantStatusCode int
	}{
		{
			name:           "no parameters",
			target:         "/",
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "bad target",
			target:         "/?target=://",
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "unhealthy target",
			target:         "/?target=" + ts.URL,
			wantStatusCode: http.StatusInternalServerError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			w := httptest.NewRecorder()

			probeHandler(w, req, time.Minute)

			if diff := cmp.Diff(tc.wantStatusCode, w.Result().StatusCode); diff != "" {
				t.Errorf("Status code diff (-want +got):\n%s", diff)
			}
		})
	}
}
