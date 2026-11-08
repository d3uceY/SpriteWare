// Command stamp writes one release version into every file that carries one.
//
// The git tag is the single source of truth. A release build runs this before
// the platform packaging tasks, so the Go binary, the exe metadata, the
// installer, the .deb and the .app bundle all report the same number.
//
//	go run ./cmd/stamp v1.2.3
//
// A marker that is missing is an error, not a warning: a renamed key must fail
// the release instead of quietly shipping the old version.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// semver is the shape the release workflow accepts: a plain numeric triple.
var semver = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// rule finds one version marker and replaces its value.
type rule struct {
	path    string
	pattern *regexp.Regexp
	// value writes the replacement for one match; most take the version as-is.
	value func(version string) string
}

func plain(version string) string { return version }

func rules() []rule {
	return []rule{
		// Wails project identity, read by `wails3 task` and the MSIX packager.
		// The line carries a trailing comment, so it is not anchored to the end.
		{"build/config.yml", regexp.MustCompile(`(?m)^(  version: ")[^"]*(")`), plain},
		// exe metadata: stamped into wails_windows_amd64.syso.
		{"build/windows/info.json", regexp.MustCompile(`(?m)^(\s*"file_version": ")[^"]*(")`), plain},
		{"build/windows/info.json", regexp.MustCompile(`(?m)^(\s*"ProductVersion": ")[^"]*(")`), plain},
		// A Windows assembly identity wants four parts. The file also carries a
		// Common-Controls assemblyIdentity, so the app's own name is the anchor.
		{"build/windows/wails.exe.manifest", regexp.MustCompile(`(<assemblyIdentity[^>]*name="com\.d3ucey\.spriteware"[^>]*\sversion=")[^"]*(")`), func(v string) string { return v + ".0" }},
		// The installer has its own copy, and it goes stale without this line.
		{"build/windows/nsis/wails_tools.nsh", regexp.MustCompile(`(?m)^(\s*!define INFO_PRODUCTVERSION ")[^"]*(")`), plain},
		// nfpm reads this one for the .deb.
		{"build/linux/nfpm/nfpm.yaml", regexp.MustCompile(`(?m)^(version: ")[^"]*(")`), plain},
		// CFBundleShortVersionString is what Finder and About show;
		// CFBundleVersion is the build number, kept equal for a tag build.
		{"build/darwin/Info.plist", regexp.MustCompile(`(<key>CFBundleShortVersionString</key>\s*<string>)[^<]*(</string>)`), plain},
		{"build/darwin/Info.plist", regexp.MustCompile(`(<key>CFBundleVersion</key>\s*<string>)[^<]*(</string>)`), plain},
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: stamp <version>   (for example: stamp v1.2.3)")
		os.Exit(2)
	}

	if err := run(os.Args[1], "."); err != nil {
		fmt.Fprintln(os.Stderr, "stamp:", err)
		os.Exit(1)
	}
}

// run stamps every rule under root and reports how many files changed.
func run(arg, root string) error {
	version, err := normalize(arg)
	if err != nil {
		return err
	}
	if err := stampAll(root, version); err != nil {
		return err
	}
	fmt.Printf("stamped %s\n", version)
	return nil
}

// normalize accepts v1.2.3 or 1.2.3 and refuses anything the release workflow
// would not have created.
func normalize(arg string) (string, error) {
	version := strings.TrimSpace(arg)
	version = strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V")
	if !semver.MatchString(version) {
		return "", fmt.Errorf("%q is not a version like 1.2.3", arg)
	}
	return version, nil
}

// stampAll applies every rule, then rewrites only the files that changed.
func stampAll(root, version string) error {
	changed := map[string]string{}

	for _, r := range rules() {
		path := filepath.Join(root, filepath.FromSlash(r.path))

		src, ok := changed[path]
		if !ok {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src = string(raw)
		}

		next, err := replace(r, src, version)
		if err != nil {
			return fmt.Errorf("%s: %w", r.path, err)
		}
		if next != src {
			changed[path] = next
		}
	}

	for path, body := range changed {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// replace applies one marker. Exactly one match is required: none means the
// file moved on without this tool, many means the pattern is too loose.
func replace(r rule, src, version string) (string, error) {
	matches := r.pattern.FindAllStringSubmatchIndex(src, -1)
	if len(matches) != 1 {
		return src, errors.New("expected one version marker, found " + fmt.Sprint(len(matches)))
	}

	m := matches[0]
	value := r.value(version)
	// Groups are (whole, prefix, suffix): keep the prefix, swap the value, keep
	// everything from the suffix opening quote onward.
	return src[:m[3]] + value + src[m[4]:], nil
}
