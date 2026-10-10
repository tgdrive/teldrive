package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var stableVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type version [3]uint64

func parseVersion(value string) (version, error) {
	match := stableVersion.FindStringSubmatch(value)
	if match == nil {
		return version{}, fmt.Errorf("invalid stable SemVer %q", value)
	}
	var result version
	for i := range result {
		n, err := strconv.ParseUint(match[i+1], 10, 64)
		if err != nil {
			return version{}, err
		}
		result[i] = n
	}
	return result, nil
}

func (v version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

func resolveVersion(input, tags string) (version, error) {
	if input != "major" && input != "minor" && input != "patch" && input != "current" {
		return parseVersion(input)
	}
	var latest version
	found := false
	for _, tag := range strings.Fields(tags) {
		v, err := parseVersion(tag)
		if err != nil {
			continue
		}
		newer := !found
		for i := range v {
			if v[i] != latest[i] {
				newer = v[i] > latest[i]
				break
			}
		}
		if newer {
			latest = v
			found = true
		}
	}
	if !found {
		return version{}, errors.New("no stable SemVer tags found; provide an explicit version")
	}
	index := map[string]int{"major": 0, "minor": 1, "patch": 2}
	if i, ok := index[input]; ok {
		if latest[i] == ^uint64(0) {
			return version{}, errors.New("version overflow")
		}
		latest[i]++
		for j := i + 1; j < len(latest); j++ {
			latest[j] = 0
		}
	}
	return latest, nil
}

func git(args ...string) (string, error) {
	output, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output)), nil
}

func release(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: just release major|minor|patch|current|VERSION [--version|--tag|--push]")
	}
	mode := ""
	if len(args) == 2 {
		mode = args[1]
	}
	if mode != "" && mode != "--version" && mode != "--tag" && mode != "--push" {
		return fmt.Errorf("unknown option %q", mode)
	}
	tags, err := git("tag", "--list")
	if err != nil {
		return err
	}
	v, err := resolveVersion(args[0], tags)
	if err != nil {
		return err
	}
	if mode == "--version" {
		fmt.Println(v)
		return nil
	}
	status, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		return errors.New("release requires a clean working tree")
	}
	tag := "v" + v.String()
	for _, existing := range strings.Fields(tags) {
		if existing == tag || existing == v.String() {
			return fmt.Errorf("version %s already tagged locally", v)
		}
	}
	remote, err := git("ls-remote", "--tags", "origin", "refs/tags/"+tag, "refs/tags/"+v.String())
	if err != nil {
		return err
	}
	if remote != "" {
		return fmt.Errorf("version %s already tagged on origin", v)
	}
	commit, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	fmt.Printf("Release: %s\nCommit: %s\n", tag, commit)
	if mode == "" {
		fmt.Println("Preview only; no changes made.")
		return nil
	}
	fmt.Printf("Pushing triggers release artifacts and images. Type %s to confirm: ", tag)
	confirmation, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(confirmation) != tag {
		return errors.New("cancelled")
	}
	if _, err := git("tag", "-a", tag, commit, "-m", "Release "+tag); err != nil {
		return err
	}
	if mode == "--push" {
		output, err := git("push", "origin", "refs/tags/"+tag)
		fmt.Println(output)
		return err
	}
	fmt.Printf("Local tag created. Publish with: git push origin refs/tags/%s\n", tag)
	return nil
}

func main() {
	if err := release(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
