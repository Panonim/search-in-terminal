// Command install downloads the latest sit release for this system, verifies it and puts it on disk.
package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const repo = "Panonim/search-in-terminal"

var (
	tty   = isTerminal()
	color = tty && os.Getenv("NO_COLOR") == "" && (runtime.GOOS != "windows" || os.Getenv("WT_SESSION") != "")
	web   = &http.Client{Timeout: 5 * time.Minute}
)

func main() {
	dir := flag.String("dir", defaultDir(), "directory to install sit into")
	flag.Parse()

	err := run(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  %s %v\n\n", paint("31", "✗"), err)
	}
	// A double-clicked installer on Windows closes its window before the output can be read.
	if runtime.GOOS == "windows" && tty {
		fmt.Print("  Press Enter to exit...")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
	if err != nil {
		os.Exit(1)
	}
}

func run(dir string) error {
	fmt.Printf("\n  %s\n\n", paint("1;36", "sit installer"))
	done("Detected " + runtime.GOOS + "/" + runtime.GOARCH)

	tag, err := latestTag()
	if err != nil {
		return fmt.Errorf("finding latest release: %w", err)
	}
	done("Latest release " + paint("1", tag))

	name := assetName(strings.TrimPrefix(tag, "v"))
	base := "https://github.com/" + repo + "/releases/download/" + tag + "/"

	type result struct {
		data []byte
		err  error
	}
	sums := make(chan result, 1)
	go func() {
		data, err := fetch(base+"checksums.txt", nil)
		sums <- result{data, err}
	}()

	archive, err := fetch(base+name, &bar{start: time.Now()})
	if err != nil {
		return fmt.Errorf("downloading %s: %w", name, err)
	}
	done(fmt.Sprintf("Downloaded %s %s", name, paint("90", "("+size(int64(len(archive)))+")")))

	s := <-sums
	if s.err != nil {
		return fmt.Errorf("downloading checksums: %w", s.err)
	}
	if err := verify(archive, string(s.data), name); err != nil {
		return err
	}
	done("Checksum verified")

	bin, err := extract(archive, name)
	if err != nil {
		return err
	}
	path, err := install(bin, dir)
	if err != nil {
		return err
	}
	done("Installed " + paint("1", path))

	if !onPath(dir) {
		fmt.Printf("\n  %s %s is not on your PATH, add it with:\n", paint("33", "!"), dir)
		if runtime.GOOS == "windows" {
			fmt.Printf("    [Environment]::SetEnvironmentVariable('Path', \"$env:Path;%s\", 'User')\n", dir)
		} else {
			fmt.Printf("    export PATH=\"%s:$PATH\"\n", dir)
		}
	}
	fmt.Printf("\n  Run %s to start searching.\n\n", paint("1;36", "sit"))
	return nil
}

// latestTag reads the tag from GitHub's /releases/latest redirect, which avoids the API rate limit.
func latestTag() (string, error) {
	c := &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := c.Head("https://github.com/" + repo + "/releases/latest")
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return "", fmt.Errorf("no published release for %s", repo)
	}
	return loc[i+len("/tag/"):], nil
}

func assetName(version string) string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("sit_%s_%s_%s%s", version, runtime.GOOS, runtime.GOARCH, ext)
}

func fetch(url string, progress *bar) ([]byte, error) {
	resp, err := web.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	if progress == nil {
		return io.ReadAll(resp.Body)
	}
	progress.total = resp.ContentLength
	data, err := io.ReadAll(io.TeeReader(resp.Body, progress))
	progress.clear()
	return data, err
}

type bar struct {
	total, n    int64
	start, last time.Time
}

func (b *bar) Write(p []byte) (int, error) {
	b.n += int64(len(p))
	if now := time.Now(); tty && (now.Sub(b.last) > 50*time.Millisecond || b.n == b.total) {
		b.last = now
		b.draw()
	}
	return len(p), nil
}

func (b *bar) draw() {
	const width = 30
	speed := size(int64(float64(b.n)/time.Since(b.start).Seconds())) + "/s"
	if b.total <= 0 {
		fmt.Printf("\r  %s  %s ", size(b.n), paint("90", speed))
		return
	}
	fill := int(width * b.n / b.total)
	fmt.Printf("\r  %s%s %3d%%  %s / %s  %s ",
		paint("36", strings.Repeat("━", fill)), paint("90", strings.Repeat("━", width-fill)),
		100*b.n/b.total, size(b.n), size(b.total), paint("90", speed))
}

func (b *bar) clear() {
	if tty {
		fmt.Print("\r" + strings.Repeat(" ", 80) + "\r")
	}
}

func verify(data []byte, checksums, name string) error {
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != fields[0] {
				return fmt.Errorf("checksum mismatch for %s", name)
			}
			return nil
		}
	}
	return fmt.Errorf("no checksum listed for %s", name)
}

func extract(archive []byte, name string) ([]byte, error) {
	exe := exeName()
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == exe {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("%s has no %s", name, exe)
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s has no %s", name, exe)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == exe {
			return io.ReadAll(tr)
		}
	}
}

// install writes to a temp file first so an interrupted install never leaves a broken binary behind.
func install(bin []byte, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".sit-install-*")
	if err != nil {
		return "", fmt.Errorf("%s is not writable, pick another with -dir or run with sudo: %w", dir, err)
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(bin)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o755)
	}
	path := filepath.Join(dir, exeName())
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	return path, err
}

func defaultDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "sit")
	}
	if os.Geteuid() == 0 {
		return "/usr/local/bin"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

func onPath(dir string) bool {
	dir = filepath.Clean(dir)
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		p = filepath.Clean(p)
		if p == dir || runtime.GOOS == "windows" && strings.EqualFold(p, dir) {
			return true
		}
	}
	return false
}

func exeName() string {
	if runtime.GOOS == "windows" {
		return "sit.exe"
	}
	return "sit"
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func done(msg string) { fmt.Printf("  %s %s\n", paint("32", "✓"), msg) }

func paint(code, s string) string {
	if !color {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func size(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }
