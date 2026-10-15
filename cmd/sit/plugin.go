package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Panonim/search-in-terminal/internal/answer"
	"github.com/Panonim/search-in-terminal/internal/plugin"
)

func cmdPlugin(args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "add":
		if len(args) != 2 {
			return fmt.Errorf("usage: sit plugin add <file.star>")
		}
		return pluginAdd(args[1], os.Stdin, os.Stdout)
	case "list":
		return pluginList(len(args) > 1 && args[1] == "--names")
	case "remove", "rm":
		if len(args) != 2 {
			return fmt.Errorf("usage: sit plugin remove <name>")
		}
		if err := plugin.Remove(args[1]); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", args[1])
		return nil
	case "test":
		if len(args) < 3 {
			return fmt.Errorf("usage: sit plugin test <name|file.star> <query…>")
		}
		return pluginTest(args[1], strings.Join(args[2:], " "))
	}
	return fmt.Errorf("unknown plugin subcommand %q (add, list, remove, test)", args[0])
}

// pluginAdd shows what the plugin will be able to do and installs it only on an explicit yes.
func pluginAdd(path string, in io.Reader, out io.Writer) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	p, err := plugin.Load(path, src)
	if err != nil {
		return err
	}
	approvals, err := plugin.Approvals()
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "⚠  Review this plugin before installing it")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  name         %s %s\n", p.Name, p.Version)
	if p.Description != "" {
		fmt.Fprintf(out, "  description  %s\n", p.Description)
	}
	fmt.Fprintf(out, "  file         %s\n", path)
	fmt.Fprintf(out, "  sha256       %s\n", p.SHA256)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  It will see every query matching:")
	for _, t := range p.Triggers {
		fmt.Fprintf(out, "    %s\n", t)
	}
	if len(p.Hosts) > 0 {
		fmt.Fprintln(out, "  It can send those queries, and anything it computes from them, to:")
		for _, h := range p.Hosts {
			fmt.Fprintf(out, "    https://%s\n", h)
		}
	} else {
		fmt.Fprintln(out, "  It has no network access.")
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  It cannot read or write files, run commands, read environment variables or")
	fmt.Fprintln(out, "  reach hosts not listed above. It runs under a step and time limit, but sit")
	fmt.Fprintln(out, "  cannot cap its memory use: only install plugins you have read.")
	if old, ok := approvals[p.Name]; ok {
		fmt.Fprintf(out, "\n  This replaces the installed %q plugin (sha256 %s).\n", p.Name, old.SHA256)
	}
	fmt.Fprint(out, "\nInstall? [y/N] ")

	line, _ := bufio.NewReader(in).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		fmt.Fprintln(out, "not installed")
		return nil
	}
	if err := plugin.Install(p, src); err != nil {
		return err
	}
	fmt.Fprintf(out, "installed %s to %s\n", p.Name, plugin.Dir())
	return nil
}

func pluginList(namesOnly bool) error {
	statuses := plugin.Installed()
	if len(statuses) == 0 && !namesOnly {
		fmt.Println("no plugins installed (sit plugin add <file.star>)")
	}
	for _, s := range statuses {
		switch {
		case namesOnly:
			fmt.Println(s.Name)
		case s.Plugin == nil:
			fmt.Printf("%-16s disabled: %s\n", s.Name, s.Problem)
		default:
			net := "no network"
			if len(s.Plugin.Hosts) > 0 {
				net = "hosts: " + strings.Join(s.Plugin.Hosts, ", ")
			}
			fmt.Printf("%-16s %-8s %s\n", s.Name, s.Plugin.Version, net)
		}
	}
	return nil
}

// pluginTest runs an installed plugin by name, or a file with the network off, for plugin authors.
func pluginTest(target, query string) error {
	var p *plugin.Plugin
	for _, s := range plugin.Installed() {
		if s.Name == target {
			if s.Plugin == nil {
				return fmt.Errorf("%s is disabled: %s", s.Name, s.Problem)
			}
			p = s.Plugin
			break
		}
	}
	if p == nil {
		src, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		if p, err = plugin.Load(target, src); err != nil {
			return err
		}
	}
	if !p.Matches(query) {
		fmt.Printf("no trigger of %s matches %q\n", p.Name, query)
		return nil
	}
	a, err := p.Answer(context.Background(), query)
	if err != nil {
		return err
	}
	if a == nil {
		fmt.Println("answer() returned None")
		return nil
	}
	printAnswer(a)
	return nil
}

func printAnswer(a *answer.Answer) {
	fmt.Printf("%s  ·  %s\n", a.Title, a.Source)
	for _, l := range strings.Split(a.Body, "\n") {
		fmt.Printf("  %s\n", l)
	}
	if a.URL != "" {
		fmt.Printf("  %s\n", a.URL)
	}
	fmt.Println()
}
