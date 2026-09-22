package update

import (
	"fmt"
	"runtime"
	"testing"
)

func TestNewerThan(t *testing.T) {
	cases := []struct {
		release, current string
		want             bool
	}{
		{"1.2.0", "1.1.9", true},
		{"1.2.0", "1.2.0", false},
		{"v1.2.0", "1.2.1", false},
		{"1.10.0", "1.9.0", true},
		{"1.0.0", "dev", true},
		{"1.0.1", "1.0.0-rc1", true},
	}
	for _, c := range cases {
		if got := NewerThan(c.release, c.current); got != c.want {
			t.Errorf("NewerThan(%q, %q) = %v", c.release, c.current, got)
		}
	}
}

func TestAssetName(t *testing.T) {
	rel := Release{TagName: "v1.2.0"}
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	want := fmt.Sprintf("sit_1.2.0_%s_%s%s", runtime.GOOS, runtime.GOARCH, ext)
	if got := rel.AssetName(); got != want {
		t.Errorf("AssetName() = %q, want %q", got, want)
	}
}

func TestVerify(t *testing.T) {
	data := []byte("hello")
	sums := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824  sit_1.0.0_linux_amd64.tar.gz\n"
	if err := verify(data, sums, "sit_1.0.0_linux_amd64.tar.gz"); err != nil {
		t.Errorf("matching checksum rejected: %v", err)
	}
	if err := verify([]byte("tampered"), sums, "sit_1.0.0_linux_amd64.tar.gz"); err == nil {
		t.Error("want an error for a mismatched checksum")
	}
	if err := verify(data, sums, "sit_1.0.0_darwin_arm64.tar.gz"); err != nil {
		t.Errorf("unlisted asset should be skipped, got %v", err)
	}
}
