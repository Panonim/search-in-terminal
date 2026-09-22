// Package update installs newer sit binaries from GitHub releases.
package update

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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repo     = "Panonim/search-in-terminal"
	apiURL   = "https://api.github.com/repos/" + repo + "/releases/latest"
	assetCap = 64 << 20
)

type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func (r Release) Version() string { return strings.TrimPrefix(r.TagName, "v") }

// AssetName is the archive this build expects, matching the release workflow's naming.
func (r Release) AssetName() string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("sit_%s_%s_%s%s", r.Version(), runtime.GOOS, runtime.GOARCH, ext)
}

func client() *http.Client { return &http.Client{Timeout: 2 * time.Minute} }

func Latest(ctx context.Context) (Release, error) {
	var rel Release
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return rel, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sit-updater")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client().Do(req)
	if err != nil {
		return rel, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return rel, fmt.Errorf("no published release for %s yet", repo)
	}
	if resp.StatusCode != http.StatusOK {
		return rel, fmt.Errorf("github api: http %d", resp.StatusCode)
	}
	err = json.NewDecoder(resp.Body).Decode(&rel)
	return rel, err
}

// NewerThan compares dotted versions numerically, treating unparsable current versions as older.
func NewerThan(release, current string) bool {
	rel, cur := normalize(release), normalize(current)
	if cur == "" || cur == "dev" {
		return true
	}
	for i := 0; i < 3; i++ {
		a, b := part(rel, i), part(cur, i)
		if a != b {
			return a > b
		}
	}
	return false
}

func normalize(v string) string {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	if i := strings.IndexAny(v, "-+"); i > 0 {
		v = v[:i]
	}
	return v
}

func part(v string, i int) int {
	fields := strings.Split(v, ".")
	if i >= len(fields) {
		return 0
	}
	n := 0
	for _, r := range fields[i] {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// Apply downloads the matching asset, verifies its checksum and replaces the running binary.
func Apply(ctx context.Context, rel Release, log io.Writer) error {
	name := rel.AssetName()
	var assetURL, sumsURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case name:
			assetURL = a.URL
		case "checksums.txt":
			sumsURL = a.URL
		}
	}
	if assetURL == "" {
		return fmt.Errorf("release %s has no asset %s", rel.TagName, name)
	}

	fmt.Fprintf(log, "downloading %s\n", name)
	archive, err := download(ctx, assetURL)
	if err != nil {
		return err
	}
	if sumsURL != "" {
		sums, err := download(ctx, sumsURL)
		if err != nil {
			return err
		}
		if err := verify(archive, string(sums), name); err != nil {
			return err
		}
		fmt.Fprintln(log, "checksum ok")
	}

	binary, err := extract(archive, name)
	if err != nil {
		return err
	}
	return replaceSelf(binary)
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sit-updater")
	resp, err := client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, assetCap))
}

func verify(data []byte, checksums, name string) error {
	want := ""
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			want = fields[0]
		}
	}
	if want == "" {
		return nil
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}

func extract(archive []byte, name string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		return extractZip(archive)
	}
	return extractTarGz(archive)
}

func extractTarGz(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == "sit" && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, assetCap))
		}
	}
	return nil, fmt.Errorf("archive contains no sit binary")
}

func extractZip(archive []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "sit.exe" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, assetCap))
		}
	}
	return nil, fmt.Errorf("archive contains no sit.exe")
}

// replaceSelf writes the new binary beside the old one and swaps it in, keeping the old file until the rename succeeds.
func replaceSelf(binary []byte) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".sit-update-*")
	if err != nil {
		return fmt.Errorf("%s is not writable: %w", dir, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		os.Rename(old, exe)
		return err
	}
	os.Remove(old)
	return nil
}
