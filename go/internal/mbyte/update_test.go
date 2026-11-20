package mbyte

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// canned "latest release" body shaped like the GitHub API response.
const latestReleaseBody = `{
  "tag_name": "v0.1.0",
  "assets": [
    {"name": "mbyte_0.1.0_linux_amd64.tar.gz", "browser_download_url": "https://example.test/dl/mbyte_0.1.0_linux_amd64.tar.gz"},
    {"name": "checksums.txt", "browser_download_url": "https://example.test/dl/checksums.txt"}
  ]
}`

func TestLatestReleaseParsesTagAndAssets(t *testing.T) {
	var gotPath, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(latestReleaseBody))
	}))
	defer srv.Close()

	tag, assets, err := LatestRelease(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "v0.1.0" {
		t.Errorf("tag = %q, want v0.1.0", tag)
	}
	if want := "/repos/" + githubOwner + "/" + githubRepo + "/releases/latest"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
	if gotUA == "" {
		t.Errorf("User-Agent header was not set (GitHub requires one)")
	}
	if len(assets) != 2 {
		t.Fatalf("got %d assets, want 2", len(assets))
	}
	if assets[0].Name != "mbyte_0.1.0_linux_amd64.tar.gz" ||
		assets[0].BrowserDownloadURL != "https://example.test/dl/mbyte_0.1.0_linux_amd64.tar.gz" {
		t.Errorf("asset[0] = %+v, unexpected", assets[0])
	}
	if assets[1].Name != "checksums.txt" {
		t.Errorf("asset[1].Name = %q, want checksums.txt", assets[1].Name)
	}
}

func TestLatestReleaseHTTPErrorIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, _, err := LatestRelease(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("expected an error on HTTP 404, got nil")
	}
	var me *MbyteError
	if !errors.As(err, &me) {
		t.Errorf("error %v is not *MbyteError", err)
	}
}

func TestLatestReleaseMalformedJSONIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	defer srv.Close()

	if _, _, err := LatestRelease(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected an error on malformed JSON, got nil")
	}
}

func TestLatestReleaseMissingTagIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"assets": []}`))
	}))
	defer srv.Close()

	if _, _, err := LatestRelease(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("expected an error when tag_name is missing, got nil")
	}
}

func TestCheckUpdate(t *testing.T) {
	cases := []struct {
		name      string
		current   string
		tag       string
		hasUpdate bool
		wantErr   bool
	}{
		{"equal", "0.1.0", "v0.1.0", false, false},
		{"equal no-v both", "0.1.0", "0.1.0", false, false},
		{"patch newer", "0.1.0", "v0.1.1", true, false},
		{"minor newer", "0.1.0", "v0.2.0", true, false},
		{"major newer", "0.9.9", "v1.0.0", true, false},
		{"current newer than tag", "0.2.0", "v0.1.0", false, false},
		{"shorter equals padded", "1.2", "v1.2.0", false, false},
		{"shorter older", "1.2", "v1.2.1", true, false},
		{"bad current", "abc", "v0.1.0", false, true},
		{"bad tag", "0.1.0", "vX.Y", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CheckUpdate(c.current, c.tag)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error for current=%q tag=%q", c.current, c.tag)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.hasUpdate {
				t.Errorf("CheckUpdate(%q,%q) = %v, want %v", c.current, c.tag, got, c.hasUpdate)
			}
		})
	}
}
