package backend

import (
	"errors"
	"strings"

	"github.com/d3uceY/spriteware/internal/config"
)

// ChooseProjectFolder asks for the folder "This project" means, and remembers it.
//
// Without this it is the launch folder, which from a shortcut is the install
// directory (or System32), where a skill lands where no agent reads and nothing
// says so.
func (s *MCPService) ChooseProjectFolder() (string, error) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()

	if app == nil {
		return "", errors.New("no window is available to open a folder picker")
	}

	chosen, err := app.Dialog.OpenFile().
		SetTitle("Choose the project folder").
		SetButtonText("Use this folder").
		SetDirectory(s.projectDir()).
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}

	// A cancelled dialog comes back empty: keep the old choice.
	chosen = strings.TrimSpace(chosen)
	if chosen == "" {
		dir, _ := s.ProjectFolder()
		return dir, nil
	}

	if _, err := s.cfg.Update(func(c *config.Config) {
		c.ProjectPath = chosen
	}); err != nil {
		return "", err
	}
	s.pushStatus()
	return chosen, nil
}

// ClearProjectFolder goes back to the working directory.
func (s *MCPService) ClearProjectFolder() (string, error) {
	if _, err := s.cfg.Update(func(c *config.Config) {
		c.ProjectPath = ""
	}); err != nil {
		return "", err
	}
	dir, _ := s.ProjectFolder()
	return dir, nil
}
