package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var ReleasesURL = "https://api.github.com/repos/d3uceY/SpriteWare/releases/latest"

const DownloadsURL = "https://github.com/d3uceY/SpriteWare#download"

type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	URL       string `json:"url"`
	Notes     string `json:"notes"`
	Error     string `json:"error,omitempty"`
}

// AppService reports the running version and asks GitHub whether a newer
// release exists. It never downloads or installs anything: the window opens the
// download table, and the user installs it the way they installed this copy.
type AppService struct {
	client *http.Client
}

// NewAppService creates the service. The client times out on its own so a
// blocked network cannot leave the check hanging for the life of the process.
func NewAppService() *AppService {
	return &AppService{client: &http.Client{Timeout: 8 * time.Second}}
}

// GetVersion is the version this binary was built with. Release builds stamp it
// from the git tag; a local build keeps the fallback in mcp.go.
func (s *AppService) GetVersion() string { return strings.TrimPrefix(Version, "v") }

// CheckForUpdate reads the newest published release and compares tags.
//
// GitHub answers "not found" until the first release exists, which is a normal
// state for a young repo and must not look like a broken update checker.
func (s *AppService) CheckForUpdate() UpdateInfo {
	info := UpdateInfo{Current: s.GetVersion()}

	req, err := http.NewRequest(http.MethodGet, ReleasesURL, nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SpriteWare/"+s.GetVersion())

	resp, err := s.client.Do(req)
	if err != nil {
		info.Error = "could not reach GitHub"
		return info
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("GitHub answered %d", resp.StatusCode)
		return info
	}

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		info.Error = "could not read the release"
		return info
	}

	info.Latest = strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	info.URL = DownloadsURL
	info.Notes = summary(release.Body)
	info.Available = newer(info.Current, info.Latest)
	return info
}

// OpenDownloadsPage hands the download table to the default browser. The URL
// arrives from the page, so it is checked against this repository rather than
// trusted.
func (s *AppService) OpenDownloadsPage(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("not a link: %w", err)
	}
	if u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || !strings.HasPrefix(u.Path, "/d3uceY/SpriteWare") {
		return errors.New("only this project's pages can be opened")
	}

	app := application.Get()
	if app == nil {
		return errors.New("no window is available")
	}
	return app.Browser.OpenURL(u.String())
}

// newer reports whether latest is a strictly higher version than current.
//
// Release tags are a plain numeric triple (the workflow refuses anything else),
// so comparing three numbers is enough. A tag that does not parse counts as
// "not newer", which keeps a malformed release quiet instead of prompting.
func newer(current, latest string) bool {
	cur, ok := parseVersion(current)
	if !ok {
		return false
	}
	next, ok := parseVersion(latest)
	if !ok {
		return false
	}
	for i := range cur {
		if next[i] != cur[i] {
			return next[i] > cur[i]
		}
	}
	return false
}

// parseVersion pulls major.minor.patch out of a tag, dropping a leading v and
// any prerelease or build suffix.
func parseVersion(s string) ([3]int, bool) {
	var out [3]int

	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return out, false
	}

	for i, part := range strings.SplitN(s, ".", 3) {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// summary is the first few lines of a release body, stripped of markdown
// markers. The dialog is not a changelog reader; this is a taste of what
// changed, and a link to the rest.
func summary(body string) string {
	var lines []string
	length := 0
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimSpace(strings.TrimLeft(line, "#*->"))
		if line == "" {
			continue
		}
		lines = append(lines, line)
		length += len(line)
		if length > 240 {
			break
		}
	}

	out := []rune(strings.Join(lines, " · "))
	if len(out) > 280 {
		return string(out[:280]) + "…"
	}
	return string(out)
}
