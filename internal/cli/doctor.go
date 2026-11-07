package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/d3uceY/spriteware/internal/aseprite"
	"github.com/d3uceY/spriteware/internal/config"
)

// check is one line of the report: what was looked at, what was found, and what
// to do about it.
type check struct {
	Level  string `json:"level"` // ok, warn, fail
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the pieces the tools need, and name what is broken",
		Long: `Every check prints ok, warn or fail, and a failed check leaves a command that
fixes it. The exit code is 1 when any check failed, so a script can gate on it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			results := diagnose()

			if jsonOut {
				if err := printJSON(cmd.OutOrStdout(), results); err != nil {
					return err
				}
			} else {
				out := cmd.OutOrStdout()
				for _, r := range results {
					fmt.Fprintf(out, "%-5s %-11s %s\n", r.Level, r.Title, r.Detail)
					if r.Fix != "" {
						fmt.Fprintf(out, "      %-11s fix: %s\n", "", r.Fix)
					}
				}
			}

			failed := 0
			for _, r := range results {
				if r.Level == "fail" {
					failed++
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d check(s) failed", failed)
			}
			return nil
		},
	}
}

// diagnose runs the checks in the order a person meets them.
func diagnose() []check {
	store, err := config.Load()
	if err != nil {
		return []check{{
			Level:  "fail",
			Title:  "config",
			Detail: err.Error(),
			Fix:    "fix the file, or delete it to start from the defaults",
		}}
	}
	cfg := store.Get()

	results := []check{checkConfig(store.FilePath()), checkAseprite(cfg), checkExtension(cfg)}
	results = append(results, checkPort("port", cfg.Port), checkPort("bridgePort", cfg.BridgePort))
	return append(results, checkPATH())
}

func checkConfig(path string) check {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return check{Level: "fail", Title: "config", Detail: err.Error(), Fix: "check the file's permissions, or delete it to start over"}
	}
	_ = f.Close()

	info, err := os.Stat(path)
	if err != nil {
		return check{Level: "ok", Title: "config", Detail: path}
	}
	return check{Level: "ok", Title: "config", Detail: fmt.Sprintf("%s (%d bytes, writable)", path, info.Size())}
}

func checkAseprite(cfg config.Config) check {
	if cfg.AsepritePath != "" {
		info, err := os.Stat(cfg.AsepritePath)
		if err != nil || info.IsDir() {
			return check{
				Level:  "fail",
				Title:  "aseprite",
				Detail: cfg.AsepritePath + " is not a file",
				Fix:    `spriteware config set asepritePath "<path to aseprite>"`,
			}
		}
		return check{Level: "ok", Title: "aseprite", Detail: cfg.AsepritePath}
	}

	found, _ := config.SearchAseprite()
	if found == "" {
		return check{
			Level:  "fail",
			Title:  "aseprite",
			Detail: "not on PATH and not in the usual install folders",
			Fix:    `spriteware config set asepritePath "<path to aseprite>"`,
		}
	}
	return check{
		Level:  "warn",
		Title:  "aseprite",
		Detail: "found at " + found + " but not confirmed",
		Fix:    fmt.Sprintf(`spriteware config set asepritePath %q`, found),
	}
}

func checkExtension(cfg config.Config) check {
	dir, port, ok := aseprite.Installed()
	if !ok {
		return check{
			Level:  "warn",
			Title:  "extension",
			Detail: "the Aseprite extension is not installed",
			Fix:    "spriteware mcp --mode live  (installs it, then serves)",
		}
	}
	// The port is baked into the installed script, so a settings change without
	// a reinstall is a live mode that dials a socket nobody listens on.
	if port != cfg.BridgePort {
		return check{
			Level:  "warn",
			Title:  "extension",
			Detail: fmt.Sprintf("installed for :%d but the config says :%d", port, cfg.BridgePort),
			Fix:    "spriteware mcp --mode live  (rewrites it with the configured port)",
		}
	}
	return check{Level: "ok", Title: "extension", Detail: fmt.Sprintf("%s, dials :%d", dir, port)}
}

func checkPort(key string, port int) check {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		// 65536 is not an address, so a full port wraps to the default.
		next := port + 1
		if next > 65535 {
			next = config.DefaultPort
		}
		return check{
			Level:  "fail",
			Title:  key,
			Detail: fmt.Sprintf("127.0.0.1:%d is taken", port),
			Fix:    fmt.Sprintf("spriteware config set %s %d", key, next),
		}
	}
	_ = ln.Close()
	return check{Level: "ok", Title: key, Detail: fmt.Sprintf("127.0.0.1:%d is free", port)}
}

func checkPATH() check {
	exe, err := os.Executable()
	if err != nil {
		return check{Level: "warn", Title: "path", Detail: "cannot tell where this program is"}
	}
	found, err := exec.LookPath("spriteware")
	switch {
	case err != nil:
		return check{
			Level:  "warn",
			Title:  "path",
			Detail: "the command is not on PATH",
			Fix:    "spriteware path add",
		}
	case sameExecutable(found, exe):
		return check{Level: "ok", Title: "path", Detail: found}
	default:
		return check{
			Level:  "warn",
			Title:  "path",
			Detail: "resolves to " + found + ", which is a different build",
			Fix:    "spriteware path add",
		}
	}
}

func sameExecutable(a, b string) bool {
	x, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	y, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(x, y)
	}
	return x == y
}
