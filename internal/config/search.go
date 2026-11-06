package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The short list misses portable copies, Steam libraries on other drives and
// hand-made folders, so the search walks where programs live and looks for the
// binary beside its companions.
const (
	// maxDepth keeps a walk off a user's whole drive. Three levels covers
	// Program Files/Vendor/App and .app/Contents.
	maxDepth = 6
	// walkBudget is how long a walk may run before it reports what it has.
	walkBudget = 4 * time.Second
)

// asepriteCompanions are files shipped beside the binary; their presence is what
// identifies an Aseprite folder.
var asepriteCompanions = []string{
	"aseprite.ini",     // written the first time Aseprite runs
	"aseprite.desktop", // the Linux launcher
	"data",             // themes and translations shipped with the app
	"Aseprite.app",     // the macOS bundle
	"aseprite.exe",     // Windows binary
}

// binaryNames are the executable names to accept, per platform.
func binaryNames() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"aseprite.exe"}
	case "darwin":
		return []string{"aseprite"}
	default:
		return []string{"aseprite"}
	}
}

// skipDirs are enormous, irrelevant, or full of permission errors.
var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, ".svn": true, "windows": true,
	"winsxs": true, "system32": true, "syswow64": true, "$recycle.bin": true,
	"system volume information": true, "proc": true, "sys": true, "dev": true,
	"appdata": true, "temp": true, "tmp": true, "cache": true,
}

// WalkBudget is how long a folder walk is allowed to run.
func WalkBudget() time.Duration { return walkBudget }

// SearchRoots are the folders a walk would start from on this machine.
func SearchRoots() []string { return searchRoots() }

// SearchAsepriteDeep looks in the usual places, then walks the install folders.
//
// The walk is bounded by a deadline and a depth limit, and prefers folders named
// after Aseprite. It never modifies anything.
func SearchAsepriteDeep(budget time.Duration) (string, []string) {
	if found, searched := SearchAseprite(); found != "" {
		return found, searched
	}
	if budget <= 0 {
		budget = walkBudget
	}
	return WalkForAseprite(searchRoots(), budget)
}

// WalkForAseprite breadth-first searches roots for an Aseprite install,
// returning the binary and the directories it looked in. A budget of zero looks
// nowhere.
func WalkForAseprite(roots []string, budget time.Duration) (string, []string) {
	if budget <= 0 {
		return "", nil
	}
	deadline := time.Now().Add(budget)
	visited := make([]string, 0, 64)

	type pending struct {
		dir   string
		depth int
	}
	queue := make([]pending, 0, len(roots))
	seen := map[string]bool{}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		queue = append(queue, pending{dir: root})
	}

	for len(queue) > 0 {
		if time.Now().After(deadline) {
			return "", visited
		}

		current := queue[0]
		queue = queue[1:]

		entries, err := os.ReadDir(current.dir)
		if err != nil {
			continue
		}
		visited = append(visited, current.dir)

		if binary := binaryIn(current.dir, entries); binary != "" {
			return binary, visited
		}
		if current.depth >= maxDepth {
			continue
		}

		// Promising children first: a folder called "Aseprite" beats the
		// twentieth vendor.
		var promising, rest []pending
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := strings.ToLower(entry.Name())
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				continue
			}
			next := pending{dir: filepath.Join(current.dir, entry.Name()), depth: current.depth + 1}
			if strings.Contains(name, "aseprite") {
				promising = append(promising, next)
			} else {
				rest = append(rest, next)
			}
		}
		queue = append(append(promising, queue...), rest...)
	}

	return "", visited
}

// binaryIn reports the Aseprite executable inside dir, if there is one.
func binaryIn(dir string, entries []os.DirEntry) string {
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())

		// A macOS bundle keeps its binary two folders down.
		if entry.IsDir() && strings.HasSuffix(name, ".app") && strings.Contains(name, "aseprite") {
			candidate := filepath.Join(dir, entry.Name(), "Contents", "MacOS", "aseprite")
			if isFile(candidate) {
				return candidate
			}
			continue
		}

		if entry.IsDir() {
			continue
		}
		for _, want := range binaryNames() {
			if name == want {
				return filepath.Join(dir, entry.Name())
			}
		}
	}

	// No binary here, but the companions are: it is probably in a bin/ or
	// build/ below.
	if hasCompanions(dir) {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			for _, want := range binaryNames() {
				if candidate := filepath.Join(dir, entry.Name(), want); isFile(candidate) {
					return candidate
				}
			}
		}
	}
	return ""
}

// hasCompanions reports whether dir looks like an Aseprite install by its
// companion files alone.
func hasCompanions(dir string) bool {
	for _, name := range asepriteCompanions {
		if isFile(filepath.Join(dir, name)) || isDir(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// searchRoots are the places applications are installed, per platform.
func searchRoots() []string {
	home, _ := os.UserHomeDir()

	var roots []string
	switch runtime.GOOS {
	case "windows":
		roots = []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"),
			filepath.Join(os.Getenv("USERPROFILE"), "scoop", "apps"),
			filepath.Join(os.Getenv("ProgramData"), "chocolatey", "lib"),
		}
		roots = append(roots, steamLibraryRoots()...)
		// Portable and source builds never reach Program Files: they unzip a
		// few folders deep into Downloads, e.g.
		// Downloads/Aseprite-v1.3.18.6-Source/build/bin/aseprite.exe.
		if home != "" {
			roots = append(roots,
				filepath.Join(home, "Downloads"),
				filepath.Join(home, "Desktop"),
			)
		}
	case "darwin":
		roots = []string{
			"/Applications",
			filepath.Join(home, "Applications"),
			"/opt/homebrew/Caskroom",
			"/usr/local/Caskroom",
			filepath.Join(home, "Downloads"),
			filepath.Join(home, "Desktop"),
		}
	default:
		roots = []string{
			"/usr/share",
			"/usr/local",
			"/opt",
			"/var/lib/flatpak/exports",
			filepath.Join(home, ".local", "share"),
			filepath.Join(home, "Downloads"),
			filepath.Join(home, "Desktop"),
		}
	}

	out := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" || !isDir(root) {
			continue
		}
		out = append(out, root)
	}
	return out
}

// steamLibraryRoots reads Steam's library list: a second drive is the commonest
// miss.
func steamLibraryRoots() []string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Steam", "steamapps", "libraryfolders.vdf"),
		filepath.Join(os.Getenv("ProgramFiles"), "Steam", "steamapps", "libraryfolders.vdf"),
	}

	var roots []string
	for _, vdf := range candidates {
		raw, err := os.ReadFile(vdf)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, `"path"`) {
				continue
			}
			// "path"    "D:\\SteamLibrary"
			parts := strings.Split(line, `"`)
			if len(parts) < 4 {
				continue
			}
			library := strings.ReplaceAll(parts[3], `\\`, `\`)
			roots = append(roots, filepath.Join(library, "steamapps", "common"))
		}
	}
	return roots
}
