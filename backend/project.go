package backend

import (
	"errors"
	"os"
	"strings"

	"github.com/d3uceY/spriteware/internal/config"
)

// ProjectFolder is the folder "This project" means, and whether the user chose
// one.
//
// An unchosen project is "", never the working directory. From a shortcut the
// working directory is the install folder (or System32), and printing that is
// the app claiming a folder nobody picked: a skill written there is one no
// agent reads, and the screen would look like the choice had been made.
func (s *MCPService) ProjectFolder() (string, bool) {
	if chosen := s.chosenProjectDir(); chosen != "" {
		return chosen, true
	}
	return "", false
}

// chosenProjectDir is the folder the user picked, or "" when they have not.
//
// Everything that shows or writes a project path uses this one, so the card and
// the installer can never disagree about which folder is in play.
func (s *MCPService) chosenProjectDir() string {
	return strings.TrimSpace(s.cfg.Get().ProjectPath)
}

// pickerStart is where the folder picker opens: the folder already chosen, else
// the working directory. A starting point is not a claim about the project, so
// the working directory is the right guess here.
func (s *MCPService) pickerStart() string {
	if chosen := s.chosenProjectDir(); chosen != "" {
		return chosen
	}
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	return ""
}

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
		SetDirectory(s.pickerStart()).
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForSingleSelection()
	if err != nil {
		// Escape keeps the old choice rather than reporting a failure.
		if isCancelled(err) {
			return s.chosenProjectDir(), nil
		}
		return "", err
	}

	// A cancelled dialog comes back empty: keep the old choice.
	chosen = strings.TrimSpace(chosen)
	if chosen == "" {
		return s.chosenProjectDir(), nil
	}

	if _, err := s.cfg.Update(func(c *config.Config) {
		c.ProjectPath = chosen
	}); err != nil {
		return "", err
	}
	s.pushStatus()
	return chosen, nil
}

// ClearProjectFolder forgets the choice, so "This project" is unset again.
func (s *MCPService) ClearProjectFolder() (string, error) {
	if _, err := s.cfg.Update(func(c *config.Config) {
		c.ProjectPath = ""
	}); err != nil {
		return "", err
	}
	return "", nil
}
