package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/mod/semver"
)

const maxNoteBodyBytes = 1 << 20
const maxNoteResponseBytes = 8 << 20

var ErrReleaseNotesMissing = errors.New("published release not found")

type ReleaseNotesFetchError struct {
	StatusCode int
	RetryAt    *time.Time
	Cause      error
}

func (e *ReleaseNotesFetchError) Error() string {
	return fmt.Sprintf("release notes fetch failed (status %d): %v", e.StatusCode, e.Cause)
}

func (e *ReleaseNotesFetchError) Unwrap() error { return e.Cause }

func noteTag(version string) (string, error) {
	tag := "v" + strings.TrimPrefix(strings.TrimSpace(version), "v")
	core := strings.SplitN(strings.SplitN(tag, "+", 2)[0], "-", 2)[0]
	if !semver.IsValid(tag) || strings.Count(core, ".") != 2 {
		return "", errors.New("release notes require a full semantic version")
	}
	return tag, nil
}

func (c *ReleaseClient) notesURL(tag string) string {
	base := "https://github.com/" + c.owner + "/" + c.repo + "/releases"
	if tag == "" {
		return base
	}
	return base + "/tag/" + url.PathEscape(tag)
}

func (c *ReleaseClient) GetReleaseNotes(ctx context.Context, tag string) (*ReleaseNote, error) {
	exactTag, err := noteTag(tag)
	if err != nil || exactTag != tag {
		return nil, &ReleaseNotesFetchError{Cause: errors.New("invalid exact release tag")}
	}
	endpoint := c.baseURL + "/repos/" + url.PathEscape(c.owner) + "/" + url.PathEscape(c.repo) + "/releases/tags/" + url.PathEscape(tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &ReleaseNotesFetchError{Cause: err}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ReleaseNotesFetchError{Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrReleaseNotesMissing
	}
	if resp.StatusCode != http.StatusOK {
		var retryAt *time.Time
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			retryAt = notesRetryAt(resp.Header, time.Now().UTC())
		}
		return nil, &ReleaseNotesFetchError{StatusCode: resp.StatusCode, RetryAt: retryAt, Cause: errors.New("unexpected HTTP status")}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxNoteResponseBytes+1))
	if err != nil {
		return nil, &ReleaseNotesFetchError{StatusCode: resp.StatusCode, Cause: err}
	}
	if len(data) > maxNoteResponseBytes || !utf8.Valid(data) {
		return nil, &ReleaseNotesFetchError{StatusCode: resp.StatusCode, Cause: errors.New("invalid release response size or encoding")}
	}
	note, err := c.parseNote(data, tag)
	if err != nil {
		return nil, &ReleaseNotesFetchError{StatusCode: resp.StatusCode, Cause: err}
	}
	return note, nil
}

func notesRetryAt(header http.Header, now time.Time) *time.Time {
	var retryAt time.Time
	value := strings.TrimSpace(header.Get("Retry-After"))
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 && seconds <= int64((1<<63-1)/time.Second) {
		retryAt = now.Add(time.Duration(seconds) * time.Second)
	} else if date, err := http.ParseTime(value); err == nil && date.After(now) {
		retryAt = date
	}
	if strings.TrimSpace(header.Get("X-RateLimit-Remaining")) == "0" {
		seconds, err := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64)
		if err != nil || seconds <= now.Unix() || seconds > 253402300799 {
			return retryTime(retryAt)
		}
		reset := time.Unix(seconds, 0).UTC()
		if reset.After(retryAt) {
			retryAt = reset
		}
	}
	return retryTime(retryAt)
}

func retryTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func (c *ReleaseClient) parseNote(data []byte, tag string) (*ReleaseNote, error) {
	var payload struct {
		Tag         string          `json:"tag_name"`
		Name        string          `json:"name"`
		Body        json.RawMessage `json:"body"`
		Draft       *bool           `json:"draft"`
		ReleaseURL  string          `json:"html_url"`
		PublishedAt *time.Time      `json:"published_at"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	if payload.Tag != tag || payload.Draft == nil || *payload.Draft || len(payload.Body) == 0 {
		return nil, errors.New("incomplete or mismatched published release")
	}
	link, err := url.Parse(payload.ReleaseURL)
	if err != nil || link.Scheme != "https" || link.Host != "github.com" || link.User != nil || link.RawQuery != "" || link.Fragment != "" || link.Path != "/"+c.owner+"/"+c.repo+"/releases/tag/"+tag {
		return nil, errors.New("mismatched release URL")
	}
	var body string
	if err := json.Unmarshal(payload.Body, &body); err != nil {
		return nil, fmt.Errorf("decode release body: %w", err)
	}
	if len(body) > maxNoteBodyBytes {
		return nil, errors.New("release note body exceeds size limit")
	}
	return &ReleaseNote{Tag: tag, Name: payload.Name, Body: body, PublishedAt: payload.PublishedAt, ReleaseURL: c.notesURL(tag), FetchedAt: time.Now().UTC()}, nil
}
