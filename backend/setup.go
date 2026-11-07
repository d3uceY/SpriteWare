package backend

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	if s.searching {
		// A walk is already running - the one at launch, or an earlier press.
		// A second one would fight it for the same folders and could clear its
		// result when it finishes first.
		s.mu.Unlock()
		return s.GetSetupState()
	}
	s.searching = true
	s.mu.Unlock()

	// Tell the window the walk has started before it starts: the scan has to be
	// on screen while it is true, not arrive with the answer.
	s.pushStatus()

	found, searched := config.SearchAsepriteDeep(config.WalkBudget())

	s.mu.Lock()
	s.candidate = found
	s.searched = searched
	s.searching = false
	s.mu.Unlock()

	s.pushStatus()
	return s.GetSetupState()
}

// BrowseAsepritePath opens the platform's file picker on the Aseprite binary.
//
// Picking the file is already the agreement, so this one confirms immediately;
// only a path the app *found* has to be confirmed in a dialog, because that is
// a guess the user never made. A cancelled picker returns the state unchanged.
func (s *MCPService) BrowseAsepritePath() (SetupState, error) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()

	if app == nil {
		return s.GetSetupState(), errors.New("no window is available to open a file picker")
	}

	label, pattern := asepriteFilter()
	dialog := app.Dialog.OpenFile().
		SetTitle("Choose the Aseprite program").
		SetButtonText("Use this").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		// A source build or a portable copy is often not named *.exe, so the
		// filter guides without locking anybody out of their own binary.
		AddFilter(label, pattern).
		AllowsOtherFileTypes(true)

	// Open where the current program is, so a user replacing it starts in the
	// folder they are replacing from.
	if dir := filepath.Dir(strings.TrimSpace(s.cfg.Get().AsepritePath)); dir != "" && dir != "." {
		dialog = dialog.SetDirectory(dir)
	}

	chosen, err := dialog.PromptForSingleSelection()
	if err != nil {
		// Escape is not a failure. Wails reports a dismissal as an error, and
		// showing it would put "cancelled by user" on a screen where the user
		// already knows what they did.
		if isCancelled(err) {
			return s.GetSetupState(), nil
		}
		return s.GetSetupState(), err
	}
	if strings.TrimSpace(chosen) == "" {
		return s.GetSetupState(), nil
	}
	return s.ConfirmAsepritePath(chosen)
}

// isCancelled reports the dismissal Wails returns as an error.
func isCancelled(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "cancel")
}

// asepriteFilter is the dialog's file filter: the program's own name on each
// platform rather than a guessed folder.
func asepriteFilter() (string, string) {
	if runtime.GOOS == "windows" {
		return "Aseprite", "*.exe"
	}
	return "Aseprite", "aseprite"
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
//
// The candidate goes with it: someone changing the program is not looking for
// the one they just rejected, and leaving it would pop the same suggestion over
// the screen they asked for.
func (s *MCPService) ForgetAsepritePath() (SetupState, error) {
	if _, err := s.cfg.Update(func(c *config.Config) {
		c.AsepritePath = ""
		c.AsepritePathConfirmed = false
	}); err != nil {
		return s.GetSetupState(), err
	}
	s.links.SetAsepritePath("")

	s.mu.Lock()
	s.candidate = ""
	s.searched = nil
	s.mu.Unlock()

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
