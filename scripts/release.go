package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/pflag"
)

var (
	versionFile = "VERSION"
	versionFlag bool
	commitFlag  bool
)

func init() {
	pflag.BoolVarP(&versionFlag, "version", "v", false, "resolved version number")
	pflag.Parse()
}

func main() {
	if err := release(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func release() error {
	if len(pflag.Args()) != 1 {
		return errors.New("error: expected version number")
	}

	version, err := getVersion()
	if err != nil {
		return err
	}

	if err := bumpVersion(version, pflag.Arg(0)); err != nil {
		return err
	}

	if versionFlag {
		fmt.Println(version)
		return nil
	}

	if err := writeVersionFile(version); err != nil {
		return fmt.Errorf("failed to write version file: %w", err)
	}

	return nil
}

func getVersion() (*semver.Version, error) {
	cmd := exec.Command("git", "tag", "-l", "[0-9]*.[0-9]*.[0-9]*", "--sort=-v:refname")
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	tags := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(tags) == 0 || tags[0] == "" {
		return nil, errors.New("error: no tags found")
	}

	return semver.NewVersion(tags[0])
}

func bumpVersion(version *semver.Version, verb string) error {
	switch verb {
	case "major":
		*version = version.IncMajor()
	case "minor":
		*version = version.IncMinor()
	case "patch":
		*version = version.IncPatch()
	case "current":
		// do nothing
	default:
		*version = *semver.MustParse(verb)
	}
	return nil
}

func writeVersionFile(version *semver.Version) error {
	return os.WriteFile(versionFile, []byte(version.String()), 0644)
}
