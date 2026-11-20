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
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/minio/selfupdate"
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

// confirmUpdate prints the US-5.3 prompt and reuses readYes (run.go) so the
// y/N semantics match the insert/replace confirmation exactly.
func confirmUpdate(r io.Reader, w io.Writer, version string) bool {
	fmt.Fprintf(w, "[?] update to %s? [y/N]: ", version)
	return readYes(r)
}

// --- US-5.2: download + self-replace -----------------------------------------

// UpdateConfig carries the injectable knobs for ApplyUpdate. All zero values are
// safe defaults (real GitHub, real http client, the running executable, the
// build's own GOOS/GOARCH); tests override them.
type UpdateConfig struct {
	Client     *http.Client // nil -> 15s-timeout client
	APIBase    string       // "" -> real GitHub API (tests inject httptest URL)
	TargetPath string       // "" -> selfupdate uses os.Executable()
	GOOS       string       // "" -> runtime.GOOS
	GOARCH     string       // "" -> runtime.GOARCH
}

// RunUpdate is the orchestration behind the `update` subcommand, kept in this
// package so the CLI layer stays thin and so it can reuse the confirm pattern.
// It fetches the latest release, reports current-vs-latest, and (unless
// checkOnly) confirms on a TTY and self-replaces via ApplyUpdate. Results go to
// s.Out; the prompt and status lines go to s.Err. On any failure nothing is
// left half-written.
func RunUpdate(ctx context.Context, s IO, cfg UpdateConfig, current string, checkOnly, assumeYes bool) error {
	tag, assets, err := LatestRelease(ctx, cfg.Client, cfg.APIBase)
	if err != nil {
		return err
	}
	hasUpdate, err := CheckUpdate(current, tag)
	if err != nil {
		return err
	}
	if !hasUpdate {
		fmt.Fprintf(s.Out, "mbyte v%s is the latest release\n", strings.TrimPrefix(current, "v"))
		return nil
	}
	fmt.Fprintf(s.Out, "update available: %s (current v%s)\n", tag, strings.TrimPrefix(current, "v"))
	if checkOnly {
		return nil
	}

	// US-5.3: confirm on a real terminal unless -y/--yes was given.
	if s.Interactive && !assumeYes {
		if !confirmUpdate(s.In, s.Err, tag) {
			fmt.Fprintln(s.Err, "[*] aborted: binary not changed")
			return nil
		}
	}

	if err := ApplyUpdate(ctx, cfg, tag, assets); err != nil {
		return err
	}
	fmt.Fprintf(s.Out, "[+] updated to %s\n", tag)
	return nil
}

// assetName builds the goreleaser archive name for a platform, e.g.
// "mbyte_0.1.0_linux_amd64.tar.gz" (or ".zip" on Windows).
func assetName(version, goos, goarch string) string {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("mbyte_%s_%s_%s.%s", v, goos, goarch, ext)
}

// findAsset returns the asset with exactly the given name.
func findAsset(assets []Asset, name string) (Asset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// parseChecksums parses a goreleaser checksums.txt ("<hex>  <filename>" lines)
// into a filename -> lowercase-hex map.
func parseChecksums(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		out[fields[1]] = strings.ToLower(fields[0])
	}
	return out
}

// extractBinary pulls the mbyte executable bytes out of a downloaded archive.
// It selects tar.gz vs zip by the asset name's extension and looks for an entry
// whose base name is "mbyte" or "mbyte.exe".
func extractBinary(assetName string, archive []byte) ([]byte, error) {
	switch {
	case strings.HasSuffix(assetName, ".tar.gz") || strings.HasSuffix(assetName, ".tgz"):
		return extractFromTarGz(archive)
	case strings.HasSuffix(assetName, ".zip"):
		return extractFromZip(archive)
	default:
		return nil, newMbyteErrorf("unsupported archive type for asset %q", assetName)
	}
}

func isBinaryEntry(name string) bool {
	base := path.Base(name)
	return base == "mbyte" || base == "mbyte.exe"
}

func extractFromTarGz(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, newMbyteErrorf("could not open gzip archive: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, newMbyteErrorf("could not read tar archive: %v", err)
		}
		if hdr.Typeflag == tar.TypeReg && isBinaryEntry(hdr.Name) {
			b, err := io.ReadAll(tr)
			if err != nil {
				return nil, newMbyteErrorf("could not read binary from archive: %v", err)
			}
			return b, nil
		}
	}
	return nil, newMbyteErrorf("no mbyte binary found inside the archive")
}

func extractFromZip(archive []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, newMbyteErrorf("could not open zip archive: %v", err)
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !isBinaryEntry(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, newMbyteErrorf("could not open binary in zip: %v", err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, newMbyteErrorf("could not read binary from zip: %v", err)
		}
		return b, nil
	}
	return nil, newMbyteErrorf("no mbyte binary found inside the archive")
}

// ApplyUpdate downloads the release archive for this platform, verifies it in
// two layers, and atomically replaces the running binary.
//
//	layer 1 (trust anchor): the archive's SHA-256 is checked against the value
//	        in the release's checksums.txt. A mismatch aborts BEFORE anything is
//	        written, so the existing binary is never touched.
//	layer 2 (write guard):  the extracted binary's SHA-256 is handed to
//	        selfupdate.Apply via Options.Checksum, so Apply re-verifies the exact
//	        bytes it writes and rolls back on any failure.
//
// tag and assets come from a prior LatestRelease call (so the caller can show a
// preview/confirm first). On any error nothing is left half-written.
func ApplyUpdate(ctx context.Context, cfg UpdateConfig, tag string, assets []Asset) error {
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	goos := cfg.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := cfg.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	wantName := assetName(tag, goos, goarch)
	archiveAsset, ok := findAsset(assets, wantName)
	if !ok {
		return newMbyteErrorf("release %s has no asset for this platform (%s)", tag, wantName)
	}
	sumsAsset, ok := findAsset(assets, "checksums.txt")
	if !ok {
		return newMbyteErrorf("release %s has no checksums.txt to verify against", tag)
	}

	// Download archive + checksums (read-only; nothing on disk is touched).
	archive, err := httpGetBytes(ctx, client, archiveAsset.BrowserDownloadURL)
	if err != nil {
		return newMbyteErrorf("could not download %s: %v", wantName, err)
	}
	sumsData, err := httpGetBytes(ctx, client, sumsAsset.BrowserDownloadURL)
	if err != nil {
		return newMbyteErrorf("could not download checksums.txt: %v", err)
	}

	// Layer 1: verify the archive against checksums.txt.
	sums := parseChecksums(sumsData)
	want, ok := sums[wantName]
	if !ok {
		return newMbyteErrorf("checksums.txt has no entry for %s", wantName)
	}
	got := sha256Hex(archive)
	if got != want {
		return newMbyteErrorf(
			"checksum mismatch for %s (expected %s, got %s); aborting without changing the binary",
			wantName, want, got)
	}

	// Extract the binary and compute its own hash for the write guard.
	binary, err := extractBinary(wantName, archive)
	if err != nil {
		return err
	}
	binSum := sha256.Sum256(binary)

	// Layer 2 + atomic replace + rollback, via the library.
	opts := selfupdate.Options{
		TargetPath: cfg.TargetPath,
		Checksum:   binSum[:],
	}
	if err := selfupdate.Apply(bytes.NewReader(binary), opts); err != nil {
		if rb := selfupdate.RollbackError(err); rb != nil {
			return newMbyteErrorf(
				"update failed AND rollback failed: %v (rollback error: %v); "+
					"the binary may be broken and need manual reinstall", err, rb)
		}
		return newMbyteErrorf("update failed, existing binary left intact: %v", err)
	}
	return nil
}

// httpGetBytes GETs a URL and returns the body, with the generic User-Agent.
func httpGetBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// sha256Hex returns the lowercase hex SHA-256 of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
