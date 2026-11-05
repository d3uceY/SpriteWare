// Package backend holds the Go services that the frontend calls.
//
// Every exported method on a registered service becomes a JS binding, so the
// shape of this package *is* the frontend API. Keep the methods small and the
// return values plain - they travel to the UI as JSON.
package backend

import (
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// Version is stamped into the UI from a single place.
const Version = "0.1.0"

// App is the root service. Register it once in main.go and every exported
// method below is callable from the frontend.
type App struct{}

// Info describes the runtime the window is running on. It exists so the
// template can prove that data really comes from Go - replace it with your
// first real model.
type Info struct {
	Version  string `json:"version"`
	Go       string `json:"go"`
	Wails    string `json:"wails"`
	Platform string `json:"platform"`
}

// Info returns the app version plus the Go, Wails and OS versions in use.
func (a *App) Info() (Info, error) {
	return Info{
		Version:  Version,
		Go:       runtime.Version(),
		Wails:    wailsVersion(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}, nil
}

// Echo proves the round trip: the argument arrives in Go, gets worked on here,
// and the result comes back to Elm. An empty message returns an error, which is
// how the template shows the failure path.
func (a *App) Echo(message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", errors.New("nothing to echo - type something first")
	}
	return fmt.Sprintf("%q went to Go and came back at %s", message, time.Now().Format(time.TimeOnly)), nil
}

// wailsVersion reads the Wails version out of the binary's build info rather
// than hard-coding it.
func wailsVersion() string {
	const module = "github.com/wailsapp/wails/v3"

	build, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range build.Deps {
		if dep.Path == module {
			return dep.Version
		}
	}
	return "unknown"
}
