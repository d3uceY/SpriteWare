package backend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/d3uceY/spriteware/internal/onpath"
)

// PathStatus is what the settings panel shows about the command line.
type PathStatus struct {
	OnPath      bool   `json:"onPath"`
	InThisShell bool   `json:"inThisShell"`
	Dir         string `json:"dir"`
	Found       string `json:"found"`
	Executable  string `json:"executable"`
	Problem     string `json:"problem,omitempty"`
}

// executable is the file a terminal should be able to type.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot tell where this program is: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// GetPathStatus reports whether typing `spriteware` works, without changing
// anything.
func (s *MCPService) GetPathStatus() PathStatus {
	exe, err := executable()
	if err != nil {
		return PathStatus{Problem: err.Error()}
	}
	st := onpath.Status(exe)
	return PathStatus{
		OnPath:      st.OnPath,
		InThisShell: st.InThisShell,
		Dir:         st.Dir,
		Found:       st.Found,
		Executable:  exe,
		Problem:     st.Problem,
	}
}

// AddToPath is the button: it makes a new terminal find the command, and
// tolerates being pressed twice.
func (s *MCPService) AddToPath() (PathStatus, error) {
	exe, err := executable()
	if err != nil {
		return s.GetPathStatus(), err
	}
	if _, err := onpath.Add(exe); err != nil {
		return s.GetPathStatus(), err
	}
	return s.GetPathStatus(), nil
}

// RemoveFromPath is the same button, backwards.
func (s *MCPService) RemoveFromPath() (PathStatus, error) {
	exe, err := executable()
	if err != nil {
		return s.GetPathStatus(), err
	}
	if err := onpath.Remove(exe); err != nil {
		return s.GetPathStatus(), err
	}
	return s.GetPathStatus(), nil
}
