package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

const releaseRepository = "ConteMan/cubbix"

type releaseNotesClient struct {
	client   *http.Client
	api      string
	current  string
	platform string
}

func newReleaseNotesClient(current string) *releaseNotesClient {
	return &releaseNotesClient{client: http.DefaultClient, api: "https://api.github.com", current: current, platform: "darwin-" + runtime.GOARCH}
}

type githubRelease struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Notes come from each platform manifest, not GitHub's independently editable body.
// Include every stable version since the running version, even when versions were skipped.
func (c *releaseNotesClient) load(ctx context.Context) (string, error) {
	type entry struct{ version, notes string }
	var entries []entry
	seen := map[string]bool{}
	for page := 1; ; page++ {
		if page > 10 {
			return "", fmt.Errorf("release history exceeds the supported page limit")
		}
		var releases []githubRelease
		endpoint := fmt.Sprintf("%s/repos/%s/releases?per_page=100&page=%d", c.api, releaseRepository, page)
		if err := c.get(ctx, endpoint, &releases); err != nil {
			return "", err
		}
		for _, release := range releases {
			v := strings.TrimPrefix(release.Tag, "v")
			if release.Draft || release.Prerelease || !stableVersion(v) || compareVersions(v, c.current) <= 0 || seen[v] {
				continue
			}
			for _, asset := range release.Assets {
				if asset.Name != "update-"+c.platform+".json" {
					continue
				}
				u, err := url.Parse(asset.URL)
				if err != nil || u.Scheme != "https" || u.Host != "github.com" || !strings.HasPrefix(u.Path, "/"+releaseRepository+"/releases/download/") {
					return "", fmt.Errorf("invalid release manifest URL")
				}
				var manifest struct{ Version, Notes string }
				if err := c.get(ctx, asset.URL, &manifest); err != nil {
					return "", err
				}
				if manifest.Version != v {
					return "", fmt.Errorf("release manifest version does not match its tag")
				}
				entries = append(entries, entry{v, manifest.Notes})
				seen[v] = true
			}
		}
		if len(releases) < 100 {
			break
		}
	}
	slices.SortFunc(entries, func(a, b entry) int { return -compareVersions(a.version, b.version) })
	var notes strings.Builder
	for _, entry := range entries {
		fmt.Fprintf(&notes, "## %s\n\n%s\n\n", entry.version, entry.notes)
	}
	return strings.TrimSpace(notes.String()), nil
}

func (c *releaseNotesClient) get(ctx context.Context, endpoint string, value any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Cubbix/"+c.current)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("release notes: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(value)
}

func stableVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return false
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return false
			}
		}
		if _, err := strconv.ParseUint(part, 10, 64); err != nil {
			return false
		}
	}
	return true
}

func compareVersions(a, b string) int {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range 3 {
		av, _ := strconv.ParseUint(aa[i], 10, 64)
		bv, _ := strconv.ParseUint(bb[i], 10, 64)
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
