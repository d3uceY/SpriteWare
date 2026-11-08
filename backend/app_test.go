package backend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"patch up", "0.1.0", "0.1.1", true},
		{"minor up", "0.1.0", "0.2.0", true},
		{"major up", "1.9.9", "2.0.0", true},
		{"same", "1.2.3", "1.2.3", false},
		{"older", "2.0.0", "1.9.9", false},
		{"leading v on both", "v1.0.0", "v1.0.1", true},
		{"prerelease suffix dropped", "1.0.0", "1.0.1-rc.1", true},
		{"build metadata dropped", "1.0.0", "1.0.0+abc", false},
		{"short tag", "1.0.0", "1.1", true},
		{"garbage latest", "1.0.0", "nightly", false},
		{"garbage current", "dev", "9.9.9", false},
		{"empty latest", "1.0.0", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := newer(c.current, c.latest); got != c.want {
				t.Fatalf("newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
			}
		})
	}
}

func TestCheckForUpdateReadsRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q", got)
		}
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "SpriteWare/") {
			t.Errorf("User-Agent = %q", ua)
		}
		_, _ = w.Write([]byte(`{
			"tag_name": "v9.9.9",
			"body": "## What changed\n\n- newer tools\n\nFull changelog below."
		}`))
	}))
	defer srv.Close()

	swapReleases(t, srv.URL)

	info := NewAppService().CheckForUpdate()
	if info.Error != "" {
		t.Fatalf("unexpected error: %s", info.Error)
	}
	if !info.Available {
		t.Fatalf("expected an available update, got %+v", info)
	}
	if info.Latest != "9.9.9" {
		t.Fatalf("Latest = %q", info.Latest)
	}
	if info.URL != DownloadsURL {
		t.Fatalf("URL = %q, want the README download table %q", info.URL, DownloadsURL)
	}
	if !strings.HasSuffix(info.URL, "#download") {
		t.Fatalf("URL = %q, want the #download anchor", info.URL)
	}
	// Markdown markers must not leak into a line of plain text.
	for _, marker := range []string{"##", "- "} {
		if strings.Contains(info.Notes, marker) {
			t.Fatalf("notes kept markdown %q: %q", marker, info.Notes)
		}
	}
	if !strings.Contains(info.Notes, "newer tools") {
		t.Fatalf("notes = %q", info.Notes)
	}
}

func TestCheckForUpdateSameVersionIsQuiet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v` + NewAppService().GetVersion() + `","html_url":"https://github.com/x/y"}`))
	}))
	defer srv.Close()

	swapReleases(t, srv.URL)

	if info := NewAppService().CheckForUpdate(); info.Available {
		t.Fatalf("same version reported as an update: %+v", info)
	}
}

func TestCheckForUpdateReportsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	swapReleases(t, srv.URL)

	info := NewAppService().CheckForUpdate()
	if info.Error == "" {
		t.Fatal("a 404 should be reported, not swallowed")
	}
	if info.Available {
		t.Fatal("a failed check must never claim an update")
	}
}

func TestCheckForUpdateBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	swapReleases(t, srv.URL)

	if info := NewAppService().CheckForUpdate(); info.Error == "" {
		t.Fatalf("expected a decode error, got %+v", info)
	}
}

func TestOpenDownloadsPageRejectsOtherPages(t *testing.T) {
	for _, raw := range []string{
		"http://github.com/d3uceY/SpriteWare#download",
		"https://evil.example/github.com",
		"https://github.com/other/repo#download",
		"javascript:alert(1)",
		"file:///etc/passwd",
		"",
	} {
		if err := NewAppService().OpenDownloadsPage(raw); err == nil {
			t.Fatalf("OpenDownloadsPage(%q) was allowed", raw)
		}
	}
}

func TestOpenDownloadsPageAcceptsTheReadmeAnchor(t *testing.T) {
	// The only URL the window sends is DownloadsURL, so the check has to pass it.
	// Without a window the call stops at the app lookup, which is how a passing
	// URL is told apart from a rejected one.
	err := NewAppService().OpenDownloadsPage(DownloadsURL)
	if err == nil {
		t.Fatal("no window should be reported, not success")
	}
	if !strings.Contains(err.Error(), "no window") {
		t.Fatalf("DownloadsURL was rejected: %v", err)
	}
}

func TestSummaryStripsMarkdown(t *testing.T) {
	got := summary("# Title\n\n- one\n- two\n")
	if strings.ContainsAny(got, "#") {
		t.Fatalf("heading marker kept: %q", got)
	}
	if !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatalf("bullets lost: %q", got)
	}
}

// swapReleases points the checker at a test server for the duration of a test.
func swapReleases(t *testing.T, url string) {
	t.Helper()
	old := ReleasesURL
	ReleasesURL = url
	t.Cleanup(func() { ReleasesURL = old })
}
