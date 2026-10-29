package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func notesResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func TestNotesIncludeAllNewerStableManifestNotes(t *testing.T) {
	client := newReleaseNotesClient("0.0.1")
	client.platform = "darwin-arm64"
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			if req.URL.Query().Get("page") != "1" {
				t.Fatal("unexpected pagination")
			}
			return notesResponse(`[
 {"tag_name":"v0.0.1","assets":[]},
 {"tag_name":"v0.0.2","body":"WRONG BODY","assets":[{"name":"update-darwin-arm64.json","browser_download_url":"https://github.com/ConteMan/cubbix/releases/download/v0.0.2/update-darwin-arm64.json"}]},
 {"tag_name":"v0.0.10","assets":[{"name":"update-darwin-arm64.json","browser_download_url":"https://github.com/ConteMan/cubbix/releases/download/v0.0.10/update-darwin-arm64.json"}]},
 {"tag_name":"v0.0.11","draft":true},
 {"tag_name":"v0.0.12","prerelease":true},
 {"tag_name":"cli-v2.0.0"}
]`), nil
		}
		v := strings.Split(req.URL.Path, "/")[5]
		return notesResponse(fmt.Sprintf(`{"version":%q,"notes":%q}`, strings.TrimPrefix(v, "v"), "Manifest notes "+v)), nil
	})}
	notes, err := client.load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := "## 0.0.10\n\nManifest notes v0.0.10\n\n## 0.0.2\n\nManifest notes v0.0.2"
	if notes != want {
		t.Fatalf("notes = %q; want %q", notes, want)
	}
}

func TestNotesPaginateAndDoNotAssumeReleaseDateOrder(t *testing.T) {
	client := newReleaseNotesClient("0.0.1")
	client.platform = "darwin-arm64"
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			if req.URL.Query().Get("page") == "1" {
				return notesResponse("[" + strings.Repeat(`{"tag_name":"v0.0.1"},`, 99) + `{"tag_name":"v0.0.1"}]`), nil
			}
			return notesResponse(`[{"tag_name":"v0.0.2","assets":[{"name":"update-darwin-arm64.json","browser_download_url":"https://github.com/ConteMan/cubbix/releases/download/v0.0.2/update-darwin-arm64.json"}]}]`), nil
		}
		return notesResponse(`{"version":"0.0.2","notes":"Older publication date, newer version"}`), nil
	})}
	notes, err := client.load(context.Background())
	if err != nil || !strings.Contains(notes, "Older publication date") {
		t.Fatalf("paginated notes=%q err=%v", notes, err)
	}
}

func TestNotesRejectMismatchedManifestVersion(t *testing.T) {
	client := newReleaseNotesClient("0.0.1")
	client.platform = "darwin-arm64"
	client.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			return notesResponse(`[{"tag_name":"v0.0.2","assets":[{"name":"update-darwin-arm64.json","browser_download_url":"https://github.com/ConteMan/cubbix/releases/download/v0.0.2/update-darwin-arm64.json"}]}]`), nil
		}
		return notesResponse(`{"version":"0.0.3","notes":"wrong version"}`), nil
	})}
	if _, err := client.load(context.Background()); err == nil {
		t.Fatal("accepted manifest for another version")
	}
}

func TestBundledHistoryIsBilingualOrderedAndMatchesVersion(t *testing.T) {
	zh, en := parseReleaseNotes(changelogChinese), parseReleaseNotes(changelogEnglish)
	if len(zh) != 3 || len(en) != len(zh) || zh[0].Version != appVersion {
		t.Fatal("bundled version history mismatch")
	}
	for i := range zh {
		if zh[i].Version != en[i].Version || zh[i].Date != en[i].Date || len(zh[i].Date) != 10 || zh[i].Notes == en[i].Notes {
			t.Fatalf("invalid translation pair: %+v %+v", zh[i], en[i])
		}
		if i > 0 && compareVersions(zh[i-1].Version, zh[i].Version) <= 0 {
			t.Fatal("history not descending")
		}
	}
	remote := "## 0.0.10 — 2026-11-01\n\n- New notes\n\n## 0.0.3 — 2026-10-29\n\nWRONG BODY"
	for _, locale := range []string{"zh-CN", "en"} {
		history := releaseHistory(locale, remote)
		if len(history) != 4 || history[0].Version != "0.0.10" || history[0].Date != "2026-11-01" || strings.Contains(history[1].Notes, "WRONG BODY") {
			t.Fatalf("merged history: %+v", history)
		}
	}
}
