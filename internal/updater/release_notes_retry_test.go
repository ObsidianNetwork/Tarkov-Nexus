package updater

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestReleaseNotes_RateLimitRetainsRetryTime(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	for _, tc := range []struct {
		name    string
		status  int
		headers map[string]string
		want    bool
	}{
		{"retry seconds", 429, map[string]string{"Retry-After": "60"}, true},
		{"retry date", 403, map[string]string{"Retry-After": future.Format(http.TimeFormat)}, true},
		{"reset timestamp", 403, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(future.Unix(), 10)}, true},
		{"invalid retry falls back", 429, map[string]string{"Retry-After": "invalid", "X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(future.Unix(), 10)}, true},
		{"quota available ignores reset", 403, map[string]string{"X-RateLimit-Remaining": "1", "X-RateLimit-Reset": strconv.FormatInt(future.Unix(), 10)}, false},
		{"invalid", 429, map[string]string{"Retry-After": "invalid"}, false},
		{"negative", 429, map[string]string{"Retry-After": "-1"}, false},
		{"overflow", 429, map[string]string{"Retry-After": "9223372036854775807"}, false},
		{"past", 403, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1"}, false},
		{"server error", 500, map[string]string{"Retry-After": "60"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, closeServer := releaseNotesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				for key, value := range tc.headers {
					w.Header().Set(key, value)
				}
				w.WriteHeader(tc.status)
			})
			defer closeServer()
			_, err := client.GetReleaseNotes(context.Background(), "v1.2.3")
			var failure *ReleaseNotesFetchError
			if !errors.As(err, &failure) || failure.StatusCode != tc.status {
				t.Fatalf("expected typed HTTP failure, got %v", err)
			}
			if (failure.RetryAt != nil) != tc.want {
				t.Fatalf("retry time = %v, want present %v", failure.RetryAt, tc.want)
			}
			if failure.RetryAt != nil && !failure.RetryAt.After(time.Now()) {
				t.Fatalf("retry time is not in the future: %v", failure.RetryAt)
			}
		})
	}
}
