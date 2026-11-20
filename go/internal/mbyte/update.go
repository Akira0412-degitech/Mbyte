package mbyte

// Step 9 / US-5.1: the "check for a newer release" half of `mbyte update`.
//
// This file is deliberately stdlib-only (net/http + encoding/json): it never
// touches the running binary. Downloading and atomically replacing the binary
// (US-5.2) is a separate, more dangerous step added later; see docs/planning.
//
// LatestRelease talks to the GitHub Releases API. The api base URL and the HTTP
// client are injectable so tests can point at an httptest.Server instead of the
// real network.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	githubOwner = "Akira0412-degitech"
	githubRepo  = "Mbyte"

	// defaultAPIBase is the real GitHub API root. Tests pass their own base.
	defaultAPIBase = "https://api.github.com"

	// userAgent is a generic client name. GitHub requires a User-Agent header;
	// we deliberately do NOT put the user's identity here.
	userAgent = "mbyte-selfupdate"
)

// Asset is one downloadable file attached to a release (the goreleaser
// tarball/zip, or checksums.txt).
type Asset struct {
	Name               string
	BrowserDownloadURL string
}

// releaseJSON mirrors the subset of the GitHub "latest release" response we use.
type releaseJSON struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// LatestRelease fetches the latest published release for the mbyte repo. apiBase
// defaults to the real GitHub API when empty; client defaults to a 15s-timeout
// client when nil. It returns the tag (e.g. "v0.1.0") and the asset list. On any
// network/HTTP/decoding problem it returns an error and touches no local state.
func LatestRelease(ctx context.Context, client *http.Client, apiBase string) (tag string, assets []Asset, err error) {
	if apiBase == "" {
		apiBase = defaultAPIBase
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest",
		strings.TrimRight(apiBase, "/"), githubOwner, githubRepo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", nil, newMbyteErrorf("could not build update request: %v", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, newMbyteErrorf("could not reach the update server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read a little of the body for context, but keep the message short.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", nil, newMbyteErrorf(
			"update server returned HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel releaseJSON
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", nil, newMbyteErrorf("could not parse the update server response: %v", err)
	}
	if rel.TagName == "" {
		return "", nil, newMbyteErrorf("update server response had no tag_name")
	}

	out := make([]Asset, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		out = append(out, Asset{Name: a.Name, BrowserDownloadURL: a.BrowserDownloadURL})
	}
	return rel.TagName, out, nil
}

// CheckUpdate compares the current version against a release tag. Both may carry
// a leading "v". It returns hasUpdate=true when latest > current. A version that
// cannot be parsed as dotted integers is an error (so the caller can report it
// rather than silently claim "up to date").
func CheckUpdate(current, tag string) (hasUpdate bool, err error) {
	cur, err := parseVersion(current)
	if err != nil {
		return false, newMbyteErrorf("could not parse current version %q: %v", current, err)
	}
	lat, err := parseVersion(tag)
	if err != nil {
		return false, newMbyteErrorf("could not parse latest version %q: %v", tag, err)
	}
	return compareVersions(lat, cur) > 0, nil
}

// parseVersion turns "v1.2.3" / "1.2" / "1" into a slice of ints. Extra
// non-numeric suffixes (pre-release tags like "-rc1") are not expected from our
// own releases and are rejected to stay strict.
func parseVersion(v string) ([]int, error) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return nil, fmt.Errorf("empty version")
	}
	parts := strings.Split(v, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid segment %q", p)
		}
		out[i] = n
	}
	return out, nil
}

// compareVersions returns -1/0/+1 for a<b / a==b / a>b, comparing segment by
// segment and treating a missing segment as 0 (so 1.2 == 1.2.0).
func compareVersions(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		switch {
		case av < bv:
			return -1
		case av > bv:
			return 1
		}
	}
	return 0
}
