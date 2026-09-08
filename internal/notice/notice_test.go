package notice

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type testLogger struct{}

func (testLogger) Debug(string)   {}
func (testLogger) Info(string)    {}
func (testLogger) Warning(string) {}
func (testLogger) Error(string)   {}

func TestActive_FiltersExpired(t *testing.T) {
	expiresSoon := time.Now().Add(1 * time.Hour)
	expiresPast := time.Now().Add(-1 * time.Hour)
	notices := []Notice{
		{ID: "live", Title: "Live", Severity: SeverityCritical},
		{ID: "expiring", Title: "Expiring soon", Expires: &expiresSoon},
		{ID: "expired", Title: "Expired", Expires: &expiresPast},
	}
	active := Active(notices, time.Now())
	if len(active) != 2 {
		t.Fatalf("got %d active notices, want 2", len(active))
	}
	if active[0].ID != "live" || active[1].ID != "expiring" {
		t.Errorf("wrong notices kept: %v, %v", active[0].ID, active[1].ID)
	}
}

func TestFetcher_FetchParseAndCallback(t *testing.T) {
	var got []Notice
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"notices":[
			{"id":"n1","severity":"critical","title":"Broken","body":"fix coming","link":"https://x","linkText":"Get fix"},
			{"id":"","title":"No ID — dropped"},
			{"title":"No ID here either"},
			{"id":"n2","title":"Default severity"}
		]}`))
	}))
	defer srv.Close()

	f := NewFetcher(srv.URL, time.Hour, testLogger{})
	f.OnUpdate(func(n []Notice) { got = n })

	notices, err := f.FetchNow()
	if err != nil {
		t.Fatalf("FetchNow error: %v", err)
	}
	if len(notices) != 2 {
		t.Fatalf("got %d notices, want 2 after boundary validation", len(notices))
	}
	if notices[0].Severity != SeverityCritical {
		t.Errorf("n1 severity = %q, want critical", notices[0].Severity)
	}
	if notices[1].Severity != SeverityInfo {
		t.Errorf("n2 severity = %q, want default info", notices[1].Severity)
	}
	if len(got) != 2 {
		t.Errorf("OnUpdate callback got %d notices, want 2", len(got))
	}
}

func TestFetcher_MalformedJSONKeepsLast(t *testing.T) {
	callCount := 0
	stage := 0 // 0 = good payload, 1 = malformed
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if stage == 0 {
			_, _ = w.Write([]byte(`{"notices":[{"id":"keep","title":"Keep me"}]}`))
		} else {
			_, _ = w.Write([]byte("this is { not json"))
		}
	}))
	defer srv.Close()

	f := NewFetcher(srv.URL, time.Hour, testLogger{})
	f.OnUpdate(func(n []Notice) { callCount++ })

	if _, err := f.FetchNow(); err != nil {
		t.Fatalf("first fetch error: %v", err)
	}
	if last := f.Last(); len(last) != 1 || last[0].ID != "keep" {
		t.Fatalf("Last() = %+v, want the good notice", last)
	}

	stage = 1
	if _, err := f.FetchNow(); err == nil {
		t.Error("malformed JSON must be an error")
	}
	if last := f.Last(); len(last) != 1 || last[0].ID != "keep" {
		t.Errorf("failure must keep last known set, got %+v", last)
	}
	if callCount != 1 {
		t.Errorf("OnUpdate called %d times, want 1 (no callback on failure)", callCount)
	}
}

func TestFetcher_HTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	f := NewFetcher(srv.URL, time.Hour, testLogger{})
	if _, err := f.FetchNow(); err == nil {
		t.Error("HTTP 503 must be an error")
	}
}

func TestFetcher_StopCancels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"notices":[]}`))
	}))
	defer srv.Close()

	f := NewFetcher(srv.URL, time.Hour, testLogger{})
	f.Start()
	f.Stop()
	// Stop must cancel the context; a second Stop must not panic.
	f.Stop()
	if f.ctx.Err() == nil {
		t.Error("Stop must cancel the fetcher context")
	}
}
