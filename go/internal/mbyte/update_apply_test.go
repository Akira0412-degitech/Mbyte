package mbyte

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// --- archive builders for tests ---------------------------------------------

func makeTarGz(t *testing.T, binName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{
		Name: binName, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, binName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(binName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func hexSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// assetServer serves an archive at /dl/<assetName> and a checksums.txt at
// /dl/checksums.txt. checksumBody is served verbatim so tests can inject a wrong
// hash. A nil archive for the asset makes that path return 404.
func assetServer(assetName string, archive []byte, checksumBody string) *httptest.Server {
	mux := http.NewServeMux()
	if archive != nil {
		mux.HandleFunc("/dl/"+assetName, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(archive)
		})
	}
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksumBody))
	})
	return httptest.NewServer(mux)
}

func assetsFor(srvURL, assetName string) []Asset {
	return []Asset{
		{Name: assetName, BrowserDownloadURL: srvURL + "/dl/" + assetName},
		{Name: "checksums.txt", BrowserDownloadURL: srvURL + "/dl/checksums.txt"},
	}
}

// writeFakeOldBinary creates a target file with known "old" bytes.
func writeFakeOldBinary(t *testing.T) (path string, oldContent []byte) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "mbyte-target")
	old := []byte("OLD BINARY CONTENT v0.0.1")
	if err := os.WriteFile(p, old, 0o755); err != nil {
		t.Fatal(err)
	}
	return p, old
}

// --- unit tests for the helpers ---------------------------------------------

func TestAssetName(t *testing.T) {
	cases := []struct {
		ver, goos, goarch, want string
	}{
		{"v0.1.0", "linux", "amd64", "mbyte_0.1.0_linux_amd64.tar.gz"},
		{"0.1.0", "darwin", "arm64", "mbyte_0.1.0_darwin_arm64.tar.gz"},
		{"v1.2.3", "windows", "amd64", "mbyte_1.2.3_windows_amd64.zip"},
	}
	for _, c := range cases {
		if got := assetName(c.ver, c.goos, c.goarch); got != c.want {
			t.Errorf("assetName(%q,%q,%q)=%q, want %q", c.ver, c.goos, c.goarch, got, c.want)
		}
	}
}

func TestParseChecksums(t *testing.T) {
	body := "aaaa  file1.tar.gz\nBBBB  file2.zip\n\n  \n"
	got := parseChecksums([]byte(body))
	if got["file1.tar.gz"] != "aaaa" {
		t.Errorf("file1 = %q", got["file1.tar.gz"])
	}
	if got["file2.zip"] != "bbbb" { // lowercased
		t.Errorf("file2 = %q", got["file2.zip"])
	}
}

func TestExtractBinaryTarGz(t *testing.T) {
	content := []byte("the real mbyte binary")
	arc := makeTarGz(t, "mbyte", content)
	got, err := extractBinary("mbyte_0.1.0_linux_amd64.tar.gz", arc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("extracted %q, want %q", got, content)
	}
}

func TestExtractBinaryZip(t *testing.T) {
	content := []byte("the real mbyte.exe")
	arc := makeZip(t, "mbyte.exe", content)
	got, err := extractBinary("mbyte_0.1.0_windows_amd64.zip", arc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("extracted %q, want %q", got, content)
	}
}

func TestExtractBinaryMissingBinary(t *testing.T) {
	arc := makeTarGz(t, "README.txt", []byte("not a binary"))
	if _, err := extractBinary("mbyte_0.1.0_linux_amd64.tar.gz", arc); err == nil {
		t.Fatal("expected error when archive has no mbyte binary")
	}
}

// --- ApplyUpdate: happy path and error paths --------------------------------

func TestApplyUpdateReplacesTarget(t *testing.T) {
	const assetName = "mbyte_0.1.0_linux_amd64.tar.gz"
	newContent := []byte("NEW BINARY CONTENT v0.1.0")
	arc := makeTarGz(t, "mbyte", newContent)
	checks := fmt.Sprintf("%s  %s\n", hexSum(arc), assetName)

	srv := assetServer(assetName, arc, checks)
	defer srv.Close()

	target, _ := writeFakeOldBinary(t)
	cfg := UpdateConfig{
		Client:     srv.Client(),
		TargetPath: target,
		GOOS:       "linux",
		GOARCH:     "amd64",
	}
	if err := ApplyUpdate(context.Background(), cfg, "v0.1.0", assetsFor(srv.URL, assetName)); err != nil {
		t.Fatalf("ApplyUpdate failed: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newContent) {
		t.Errorf("target content = %q, want %q", got, newContent)
	}
}

func TestApplyUpdateChecksumMismatchLeavesTargetIntact(t *testing.T) {
	const assetName = "mbyte_0.1.0_linux_amd64.tar.gz"
	arc := makeTarGz(t, "mbyte", []byte("NEW BINARY CONTENT v0.1.0"))
	// Deliberately wrong checksum for the archive.
	checks := fmt.Sprintf("%s  %s\n", hexSum([]byte("something else entirely")), assetName)

	srv := assetServer(assetName, arc, checks)
	defer srv.Close()

	target, old := writeFakeOldBinary(t)
	cfg := UpdateConfig{Client: srv.Client(), TargetPath: target, GOOS: "linux", GOARCH: "amd64"}

	err := ApplyUpdate(context.Background(), cfg, "v0.1.0", assetsFor(srv.URL, assetName))
	if err == nil {
		t.Fatal("expected a checksum-mismatch error, got nil")
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, old) {
		t.Errorf("target was modified on checksum mismatch: %q", got)
	}
}

func TestApplyUpdateDownloadFailureLeavesTargetIntact(t *testing.T) {
	const assetName = "mbyte_0.1.0_linux_amd64.tar.gz"
	// archive=nil -> the /dl/<asset> path 404s; checksums body is irrelevant.
	srv := assetServer(assetName, nil, "whatever  "+assetName+"\n")
	defer srv.Close()

	target, old := writeFakeOldBinary(t)
	cfg := UpdateConfig{Client: srv.Client(), TargetPath: target, GOOS: "linux", GOARCH: "amd64"}

	err := ApplyUpdate(context.Background(), cfg, "v0.1.0", assetsFor(srv.URL, assetName))
	if err == nil {
		t.Fatal("expected a download error, got nil")
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, old) {
		t.Errorf("target was modified on download failure: %q", got)
	}
}

func TestApplyUpdateNoAssetForPlatform(t *testing.T) {
	// Assets exist only for linux/amd64, but we ask for a platform with no asset.
	const assetName = "mbyte_0.1.0_linux_amd64.tar.gz"
	srv := assetServer(assetName, makeTarGz(t, "mbyte", []byte("x")), "x  "+assetName+"\n")
	defer srv.Close()

	target, old := writeFakeOldBinary(t)
	cfg := UpdateConfig{Client: srv.Client(), TargetPath: target, GOOS: "plan9", GOARCH: "ppc64"}

	if err := ApplyUpdate(context.Background(), cfg, "v0.1.0", assetsFor(srv.URL, assetName)); err == nil {
		t.Fatal("expected an error for a platform with no asset")
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, old) {
		t.Errorf("target modified although no matching asset: %q", got)
	}
}

// --- RunUpdate orchestration -------------------------------------------------

// releaseServer serves the latest-release JSON plus the asset downloads, so the
// full RunUpdate path (LatestRelease -> CheckUpdate -> ApplyUpdate) can run.
func releaseServer(t *testing.T, tag, assetName string, archive []byte, checksumBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/dl/"+assetName, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) })
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(checksumBody)) })
	mux.HandleFunc("/repos/"+githubOwner+"/"+githubRepo+"/releases/latest",
		func(w http.ResponseWriter, r *http.Request) {
			base := "http://" + r.Host
			fmt.Fprintf(w, `{"tag_name":%q,"assets":[`+
				`{"name":%q,"browser_download_url":%q},`+
				`{"name":"checksums.txt","browser_download_url":%q}]}`,
				tag, assetName, base+"/dl/"+assetName, base+"/dl/checksums.txt")
		})
	return httptest.NewServer(mux)
}

func TestRunUpdateCheckOnlyReportsAndWritesNothing(t *testing.T) {
	const assetName = "mbyte_0.1.0_linux_amd64.tar.gz"
	arc := makeTarGz(t, "mbyte", []byte("new"))
	srv := releaseServer(t, "v0.1.0", assetName, arc, hexSum(arc)+"  "+assetName+"\n")
	defer srv.Close()

	var out, errbuf bytes.Buffer
	s := IO{Out: &out, Err: &errbuf}
	cfg := UpdateConfig{Client: srv.Client(), APIBase: srv.URL, GOOS: "linux", GOARCH: "amd64"}

	// current older than tag -> "update available", but --check writes nothing.
	if err := RunUpdate(context.Background(), s, cfg, "0.0.1", true, false); err != nil {
		t.Fatalf("RunUpdate --check failed: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("update available: v0.1.0")) {
		t.Errorf("stdout = %q, want 'update available'", out.String())
	}
}

func TestRunUpdateYesPerformsReplace(t *testing.T) {
	const assetName = "mbyte_0.1.0_linux_amd64.tar.gz"
	newContent := []byte("NEW via RunUpdate")
	arc := makeTarGz(t, "mbyte", newContent)
	srv := releaseServer(t, "v0.1.0", assetName, arc, hexSum(arc)+"  "+assetName+"\n")
	defer srv.Close()

	target, _ := writeFakeOldBinary(t)
	var out, errbuf bytes.Buffer
	s := IO{Out: &out, Err: &errbuf}
	cfg := UpdateConfig{
		Client: srv.Client(), APIBase: srv.URL, TargetPath: target, GOOS: "linux", GOARCH: "amd64",
	}

	// Non-interactive + assumeYes -> performs the replace without prompting.
	if err := RunUpdate(context.Background(), s, cfg, "0.0.1", false, true); err != nil {
		t.Fatalf("RunUpdate failed: %v", err)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, newContent) {
		t.Errorf("target = %q, want %q", got, newContent)
	}
	if !bytes.Contains(out.Bytes(), []byte("[+] updated to v0.1.0")) {
		t.Errorf("stdout = %q, want '[+] updated to v0.1.0'", out.String())
	}
}

func TestRunUpdateAlreadyLatest(t *testing.T) {
	const assetName = "mbyte_9.9.9_linux_amd64.tar.gz"
	arc := makeTarGz(t, "mbyte", []byte("x"))
	srv := releaseServer(t, "v9.9.9", assetName, arc, hexSum(arc)+"  "+assetName+"\n")
	defer srv.Close()

	var out, errbuf bytes.Buffer
	s := IO{Out: &out, Err: &errbuf}
	cfg := UpdateConfig{Client: srv.Client(), APIBase: srv.URL, GOOS: "linux", GOARCH: "amd64"}

	// current == latest -> "is the latest", no replace attempt.
	if err := RunUpdate(context.Background(), s, cfg, "9.9.9", false, true); err != nil {
		t.Fatalf("RunUpdate failed: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("is the latest release")) {
		t.Errorf("stdout = %q, want 'is the latest release'", out.String())
	}
}
