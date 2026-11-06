package backend

import (
	"errors"
	"os"
	"strings"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/tools"
)

// SetupState is everything the first-run screen needs in one call.
//
// A detected path is a *suggestion*: nothing is written until the user agrees.
// That separates "found Aseprite" from "will run this file".
type SetupState struct {
	Confirmed bool     `json:"confirmed"`
	Path      string   `json:"path"`
	Candidate string   `json:"candidate"`
	Found     bool     `json:"found"`
	Searching bool     `json:"searching"`
	Searched  []string `json:"searched"`
	Problem   string   `json:"problem,omitempty"`
}

// GetSetupState reports what the app knows without touching the disk.
func (s *MCPService) GetSetupState() SetupState {
	cfg := s.cfg.Get()

	s.mu.Lock()
	candidate := s.candidate
	searching := s.searching
	searched := append([]string(nil), s.searched...)
	s.mu.Unlock()

	return SetupState{
		Confirmed: cfg.AsepritePathConfirmed && cfg.AsepritePath != "",
		Path:      cfg.AsepritePath,
		Candidate: candidate,
		Found:     candidate != "",
		Searching: searching,
		Searched:  searched,
	}
}

// DetectAseprite runs the full search again, in case Aseprite was installed
// while the app was open.
func (s *MCPService) DetectAseprite() SetupState {
	s.mu.Lock()
	s.searching = true
	s.mu.Unlock()

	found, searched := config.SearchAsepriteDeep(config.WalkBudget())

	s.mu.Lock()
	s.candidate = found
	s.searched = searched
	s.searching = false
	s.mu.Unlock()

	s.pushStatus()
	return s.GetSetupState()
}

// ConfirmAsepritePath is the user saying "yes, that is the one".
//
// The path is checked, not trusted: a folder or a typo fails somewhere else
// looking like a bug.
func (s *MCPService) ConfirmAsepritePath(path string) (SetupState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return s.GetSetupState(), errors.New("choose the Aseprite executable first")
	}
	info, err := os.Stat(path)
	if err != nil {
		return s.GetSetupState(), errors.New("that path does not exist")
	}
	if info.IsDir() {
		return s.GetSetupState(), errors.New("that is a folder, not the aseprite program")
	}

	saved, err := s.cfg.Update(func(c *config.Config) {
		c.AsepritePath = path
		c.AsepritePathConfirmed = true
	})
	if err != nil {
		return s.GetSetupState(), err
	}

	// The link was built with the old path, so rebuild it.
	s.links.SetAsepritePath(saved.AsepritePath)

	s.mu.Lock()
	s.candidate = path
	s.mu.Unlock()

	s.pushStatus()
	return s.GetSetupState(), nil
}

// ForgetAsepritePath sends the user back to the setup screen.
func (s *MCPService) ForgetAsepritePath() (SetupState, error) {
	if _, err := s.cfg.Update(func(c *config.Config) {
		c.AsepritePath = ""
		c.AsepritePathConfirmed = false
	}); err != nil {
		return s.GetSetupState(), err
	}
	s.links.SetAsepritePath("")
	s.pushStatus()
	return s.GetSetupState(), nil
}

// ListTools reports the published surface from the table the server registers.
func (s *MCPService) ListTools() []tools.ToolInfo {
	return tools.Catalog
}

// startupDetection looks in the usual places, then walks in the background only
// if that found nothing. Launch never waits, and nothing is written.
func (s *MCPService) startupDetection() {
	if s.cfg.Get().AsepritePathConfirmed {
		return
	}

	found, searched := config.SearchAseprite()

	s.mu.Lock()
	s.candidate = found
	s.searched = searched
	s.searching = found == ""
	s.mu.Unlock()

	if found != "" {
		return
	}

	go func() {
		deep, walked := config.WalkForAseprite(config.SearchRoots(), config.WalkBudget())

		s.mu.Lock()
		s.candidate = deep
		s.searched = append(searched, walked...)
		s.searching = false
		s.mu.Unlock()

		s.pushStatus()
	}()
}
