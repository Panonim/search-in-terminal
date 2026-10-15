package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/Panonim/search-in-terminal/internal/answer"
	"github.com/Panonim/search-in-terminal/internal/config"
)

// Approval pins what the user agreed to at install time.
type Approval struct {
	SHA256 string   `json:"sha256"`
	Hosts  []string `json:"hosts"`
}

// Status is an installed plugin, or why it is disabled.
type Status struct {
	Name    string
	Plugin  *Plugin
	Problem string
}

func Dir() string { return filepath.Join(config.Dir(), "plugins") }

func approvalsPath() string { return filepath.Join(Dir(), "approved.json") }

func filePath(name string) string { return filepath.Join(Dir(), name+".star") }

func Approvals() (map[string]Approval, error) {
	out := map[string]Approval{}
	data, err := os.ReadFile(approvalsPath())
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, json.Unmarshal(data, &out)
}

func saveApprovals(a map[string]Approval) error {
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	tmp := approvalsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, approvalsPath())
}

// Install copies src into the plugin directory and pins its hash and hosts.
func Install(p *Plugin, src []byte) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	approvals, err := Approvals()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filePath(p.Name), src, 0o600); err != nil {
		return err
	}
	approvals[p.Name] = Approval{SHA256: p.SHA256, Hosts: p.Hosts}
	return saveApprovals(approvals)
}

func Remove(name string) error {
	approvals, err := Approvals()
	if err != nil {
		return err
	}
	if _, ok := approvals[name]; !ok {
		return fmt.Errorf("no plugin %q installed", name)
	}
	delete(approvals, name)
	if err := os.Remove(filePath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return saveApprovals(approvals)
}

// Installed loads every approved plugin, disabling any whose file or hosts changed since approval.
func Installed() []Status {
	approvals, err := Approvals()
	if err != nil {
		return []Status{{Name: "approved.json", Problem: err.Error()}}
	}
	out := make([]Status, 0, len(approvals))
	for _, name := range slices.Sorted(maps.Keys(approvals)) {
		out = append(out, load(name, approvals[name]))
	}
	return out
}

func load(name string, a Approval) Status {
	src, err := os.ReadFile(filePath(name))
	if err != nil {
		return Status{Name: name, Problem: err.Error()}
	}
	if sum := sha256.Sum256(src); hex.EncodeToString(sum[:]) != a.SHA256 {
		return Status{Name: name, Problem: "file changed since approval, run `sit plugin add` on it again"}
	}
	p, err := Load(filePath(name), src)
	switch {
	case err != nil:
		return Status{Name: name, Problem: err.Error()}
	case p.Name != name || !slices.Equal(p.Hosts, a.Hosts):
		return Status{Name: name, Problem: "manifest no longer matches its approval, run `sit plugin add` on it again"}
	}
	p.network = len(p.Hosts) > 0
	return Status{Name: name, Plugin: p}
}

// Providers returns the working installed plugins as answer sources.
func Providers() []answer.Provider {
	var out []answer.Provider
	for _, s := range Installed() {
		if s.Plugin != nil {
			out = append(out, s.Plugin)
		}
	}
	return out
}
