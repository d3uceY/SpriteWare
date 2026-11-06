// Package analyze checks a doc.Dump and reports concrete, located problems.
//
// Nothing here needs Aseprite: a broken sprite is a dump plus a PNG, so the
// checks are fast and unit-testable, and all three front ends share them.
package analyze

import (
	"fmt"
	"image"
	"sort"
	"strings"

	"github.com/d3uceY/spriteware/internal/craft"
	"github.com/d3uceY/spriteware/internal/doc"
)

// Severity orders findings. Errors mean "do not call this finished".
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Note    Severity = "note"
)

// Order returns 0 for errors, 1 for warnings, 2 for notes.
func (s Severity) Order() int {
	switch s {
	case Error:
		return 0
	case Warning:
		return 1
	default:
		return 2
	}
}

// Point is a sprite coordinate.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Rect is a sprite-space rectangle.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Finding is one problem.
type Finding struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Layer    string   `json:"layer,omitempty"`
	Frame    int      `json:"frame,omitempty"`
	At       *Point   `json:"at,omitempty"`
	Rect     *Rect    `json:"rect,omitempty"`
	Count    int      `json:"count,omitempty"`
}

// Scope decides what the pixel checks look at.
type Scope string

const (
	// ScopeComposite combines all visible layers. The default, because a
	// correct rig looks broken judged layer by layer.
	ScopeComposite Scope = "composite"
	// ScopeLayer checks every layer on its own, for debugging a specific one.
	ScopeLayer Scope = "layer"
)

// Check names, also used by callers to run a subset.
const (
	CheckPalette    = "palette"
	CheckStrays     = "strays"
	CheckOrphans    = "orphans"
	CheckGaps       = "gaps"
	CheckBounds     = "bounds"
	CheckOutline    = "outline"
	CheckBanding    = "banding"
	CheckAlpha      = "antialiasing"
	CheckLayers     = "layers"
	CheckAnimation  = "animation"
	CheckReadiness  = "export_readiness"
	CheckExpect     = "expect"
	CheckVolume     = "volume"
	CheckCompliance = "compliance"
)

// AllChecks is the default set, in the order they are reported.
var AllChecks = []string{
	CheckPalette, CheckStrays, CheckOrphans, CheckGaps, CheckBounds,
	CheckOutline, CheckBanding, CheckAlpha, CheckLayers, CheckAnimation,
	CheckReadiness, CheckVolume,
}

// Expect is the caller's own plan, run on top of the requested checks.
type Expect struct {
	// LayerFrames maps a layer to the 1-based ranges it should hold ink in.
	// Ink outside them, or a gap inside, is an error.
	LayerFrames map[string][][2]int `json:"layerFrames,omitempty"`
	// MustNotOverlap lists layer pairs whose opaque pixels may never touch.
	MustNotOverlap [][2]string `json:"mustNotOverlap,omitempty"`
	// PaletteSize caps how many colours the palette may hold.
	PaletteSize *int `json:"paletteSize,omitempty"`
	// Bounds caps where art may be, in sprite coordinates.
	Bounds *Rect `json:"bounds,omitempty"`
	// SingleComponent asks every frame to be exactly one connected shape.
	SingleComponent bool `json:"singleComponent,omitempty"`
}

// Options configures a run.
type Options struct {
	Checks []string
	Scope  Scope
	Strict bool
	Expect *Expect
}

func (o Options) scope() Scope {
	if o.Scope == ScopeLayer {
		return ScopeLayer
	}
	return ScopeComposite
}

func (o Options) wanted() map[string]bool {
	name := o.Checks
	if len(name) == 0 {
		name = AllChecks
	}
	out := make(map[string]bool, len(name))
	for _, c := range name {
		out[c] = true
	}
	return out
}

// Metrics is the numeric vector a run produces. Always computed: these are the
// numbers correlated against a human's judgement.
type Metrics struct {
	ColorsUsed       int     `json:"colorsUsed"`
	PaletteSize      int     `json:"paletteSize"`
	OffPalettePixels int     `json:"offPalettePixels"`
	OffPaletteColors int     `json:"offPaletteColors"`
	HalfAlphaPixels  int     `json:"halfAlphaPixels"`
	StrayPixels      int     `json:"strayPixels"`
	OrphanPixels     int     `json:"orphanPixels"`
	MaxComponents    int     `json:"maxComponents"`
	EdgePixels       int     `json:"edgePixels"`
	TonalClusters    int     `json:"tonalClusters"`
	BandingRun       int     `json:"bandingRun"`
	HeightDrift      int     `json:"heightDrift"`
	DurationSpread   int     `json:"durationSpread"`
	InkPixels        int     `json:"inkPixels"`
	Coverage         float64 `json:"coverage"`
}

// Report is the whole answer.
type Report struct {
	Sprite   string    `json:"sprite"`
	Scope    Scope     `json:"scope"`
	Passed   bool      `json:"passed"`
	Score    int       `json:"score"`
	Findings []Finding `json:"findings"`
	Metrics  Metrics   `json:"metrics"`
	Summary  string    `json:"summary"`
}

// Run checks a dump and returns a report.
func Run(d *doc.Dump, opts Options) *Report {
	want := opts.wanted()
	scope := opts.scope()

	frames := make([]*image.NRGBA, 0, d.FrameCount())
	for f := 1; f <= d.FrameCount(); f++ {
		frames = append(frames, d.Composite(f))
	}

	acc := newAccumulator()

	if want[CheckPalette] {
		checkPalette(d, frames, acc)
	}
	if want[CheckAlpha] {
		checkAlpha(frames, acc)
	}
	if want[CheckLayers] {
		checkLayers(d, acc)
	}
	if want[CheckAnimation] {
		checkAnimation(d, acc)
	}
	if want[CheckReadiness] {
		checkReadiness(d, acc)
	}
	if want[CheckVolume] {
		checkVolume(frames, acc)
	}

	// Pixel checks run over whatever the scope selects.
	type target struct {
		layer string
		img   *image.NRGBA
		frame int
	}
	var targets []target
	if scope == ScopeLayer {
		for li := range d.Layers {
			if d.Layers[li].IsGroup {
				continue
			}
			for f := 1; f <= d.FrameCount(); f++ {
				targets = append(targets, target{d.Layers[li].Name, d.LayerImage(li, f), f})
			}
		}
	} else {
		for i, img := range frames {
			targets = append(targets, target{"", img, i + 1})
		}
	}

	for _, t := range targets {
		if want[CheckStrays] {
			checkStrays(t.img, t.layer, t.frame, acc)
		}
		if want[CheckOrphans] {
			checkOrphans(t.img, t.layer, t.frame, acc)
		}
		if want[CheckGaps] {
			checkGaps(t.img, t.layer, t.frame, opts.Expect, acc)
		}
		if want[CheckBounds] {
			checkBounds(t.img, t.layer, t.frame, acc)
		}
		if want[CheckOutline] {
			checkOutline(t.img, t.layer, t.frame, acc)
		}
		if want[CheckBanding] {
			checkBanding(t.img, t.layer, t.frame, acc)
		}
	}

	if opts.Expect != nil {
		checkExpect(d, opts.Expect, acc)
	}

	return acc.report(d, scope, opts.Strict)
}

// ---------------------------------------------------------------------------
// Accumulator
// ---------------------------------------------------------------------------

type accumulator struct {
	findings []Finding
	metrics  Metrics
	// perTarget is the frame a per-frame metric came from, so maxima need no
	// second pass.
	seenComponents bool
	seenBanding    bool
	heights        []int
	durations      []int
	colors         map[craft.RGB]int
	frames         int
	inkPixels      int
}

func newAccumulator() *accumulator {
	return &accumulator{colors: map[craft.RGB]int{}}
}

func (a *accumulator) add(f Finding) { a.findings = append(a.findings, f) }

func (a *accumulator) report(d *doc.Dump, scope Scope, strict bool) *Report {
	sort.SliceStable(a.findings, func(i, j int) bool {
		if a.findings[i].Severity != a.findings[j].Severity {
			return a.findings[i].Severity.Order() < a.findings[j].Severity.Order()
		}
		if a.findings[i].Frame != a.findings[j].Frame {
			return a.findings[i].Frame < a.findings[j].Frame
		}
		return a.findings[i].Check < a.findings[j].Check
	})

	var errors, warnings int
	for _, f := range a.findings {
		switch f.Severity {
		case Error:
			errors++
		case Warning:
			warnings++
		}
	}

	passed := errors == 0 && (!strict || warnings == 0)
	score := 100 - errors*15 - warnings*5
	if score < 0 {
		score = 0
	}

	m := a.metrics
	m.ColorsUsed = len(a.colors)
	m.PaletteSize = len(d.Palette)
	m.InkPixels = a.inkPixels
	if d.Sprite.Width > 0 && d.Sprite.Height > 0 && a.frames > 0 {
		m.Coverage = float64(a.inkPixels) / float64(d.Sprite.Width*d.Sprite.Height*a.frames)
	}
	m.TonalClusters = tonalClusters(a.colors)
	m.HeightDrift = spread(a.heights)
	m.DurationSpread = spread(a.durations)

	summary := summarize(errors, warnings, len(a.findings), passed)

	return &Report{
		Sprite:   d.Sprite.ID,
		Scope:    scope,
		Passed:   passed,
		Score:    score,
		Findings: a.findings,
		Metrics:  m,
		Summary:  summary,
	}
}

func summarize(errors, warnings, total int, passed bool) string {
	if total == 0 {
		return "Clean: nothing to report."
	}
	if passed {
		return fmt.Sprintf("Clean, with %d style note(s) you may keep.", total)
	}
	return fmt.Sprintf("%d error(s), %d warning(s). Fix the errors before calling this done.", errors, warnings)
}

// ---------------------------------------------------------------------------
// Whole-document checks
// ---------------------------------------------------------------------------

func checkPalette(d *doc.Dump, frames []*image.NRGBA, acc *accumulator) {
	for _, img := range frames {
		for c, n := range doc.Colors(img) {
			acc.colors[c] += n
			acc.inkPixels += n
		}
	}
	acc.frames = len(frames)

	palette := d.PaletteRGB()
	if len(palette) == 0 {
		acc.add(Finding{
			Check:    CheckPalette,
			Severity: Note,
			Message:  "The document has no palette, so colours cannot be checked against one. Load a preset or extract a palette from the art.",
		})
		return
	}

	off := map[craft.RGB]int{}
	for c, n := range acc.colors {
		idx, _ := craft.Nearest(c, palette)
		if idx >= 0 && palette[idx] == c {
			continue // an exact palette entry
		}
		off[c] = n
		acc.metrics.OffPalettePixels += n
	}
	if len(off) > 0 {
		worst := doc.SortedColors(off)[0]
		acc.metrics.OffPaletteColors = len(off)
		acc.add(Finding{
			Check:    CheckPalette,
			Severity: Warning,
			Message: fmt.Sprintf("%d colour(s) in the art are not in the palette. The most used is %s (%d px). Recolor 'snap' brings them in line.",
				len(off), worst.Hex(), off[worst]),
			Count: len(off),
		})
	}
	if len(palette) > 64 {
		acc.add(Finding{
			Check:    CheckPalette,
			Severity: Note,
			Message:  fmt.Sprintf("%d palette entries. Most sprites read better under 32 — a large palette usually means ramps that were never merged.", len(palette)),
			Count:    len(palette),
		})
	}
}

func checkAlpha(frames []*image.NRGBA, acc *accumulator) {
	total := 0
	var first *Point
	for _, img := range frames {
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if a := img.NRGBAAt(x, y).A; a > 0 && a < 255 {
					total++
					if first == nil {
						first = &Point{X: x, Y: y}
					}
				}
			}
		}
	}
	acc.metrics.HalfAlphaPixels = total
	if total > 0 {
		acc.add(Finding{
			Check:    CheckAlpha,
			Severity: Warning,
			Message:  fmt.Sprintf("%d semi-transparent pixel(s). Partial alpha usually comes from a soft brush or a resize, and reads as blur at 1x.", total),
			At:       first,
			Count:    total,
		})
	}
}

func checkLayers(d *doc.Dump, acc *accumulator) {
	seen := map[string]int{}
	for _, l := range d.Layers {
		if l.IsGroup {
			continue
		}
		seen[l.Name]++
		if len(l.Cels) == 0 {
			acc.add(Finding{
				Check:    CheckLayers,
				Severity: Note,
				Message:  fmt.Sprintf("Layer %q has no cels on any frame.", l.Name),
				Layer:    l.Name,
			})
		}
	}
	for name, n := range seen {
		if n > 1 {
			acc.add(Finding{
				Check:    CheckLayers,
				Severity: Warning,
				Message:  fmt.Sprintf("%d layers share the name %q. Tools that look a layer up by name cannot tell them apart.", n, name),
				Layer:    name,
				Count:    n,
			})
		}
	}
}

func checkAnimation(d *doc.Dump, acc *accumulator) {
	for _, f := range d.Frames {
		acc.durations = append(acc.durations, f.DurationMS)
	}
	if len(d.Frames) < 2 {
		return
	}
	if len(d.Tags) == 0 {
		acc.add(Finding{
			Check:    CheckAnimation,
			Severity: Error,
			Message:  fmt.Sprintf("%d frames and no animation tag. An engine cannot address untagged frames — add a tag per cycle.", len(d.Frames)),
			Count:    len(d.Frames),
		})
	}
	distinct := map[int]bool{}
	for _, f := range d.Frames {
		distinct[f.DurationMS] = true
	}
	if len(distinct) == 1 && len(d.Frames) > 3 {
		acc.add(Finding{
			Check:    CheckAnimation,
			Severity: Note,
			Message:  "Every frame has the same duration. Uniform timing reads mechanical; hold contact poses longer than passing poses.",
		})
	}
}

func checkReadiness(d *doc.Dump, acc *accumulator) {
	if !d.Sprite.Saved || d.Sprite.Path == "" {
		acc.add(Finding{
			Check:    CheckReadiness,
			Severity: Warning,
			Message:  "This sprite has never been saved, so there is nothing on disk to hand to a game project.",
		})
	}
}

func checkVolume(frames []*image.NRGBA, acc *accumulator) {
	heights := make([]int, 0, len(frames))
	for _, img := range frames {
		if r, ok := doc.Bounds(img); ok {
			heights = append(heights, r.Dy())
		} else {
			heights = append(heights, 0)
		}
	}
	acc.heights = heights
	if len(heights) < 3 {
		return
	}
	drift := spread(heights)
	if drift > 2 {
		acc.add(Finding{
			Check:    CheckVolume,
			Severity: Warning,
			Message:  fmt.Sprintf("The art's height changes by %d px across the cycle. Some change is squash and stretch; this much is usually a redraw that drifted.", drift),
			Count:    drift,
		})
	}
}

// ---------------------------------------------------------------------------
// Per-image checks
// ---------------------------------------------------------------------------

func checkStrays(img *image.NRGBA, layer string, frame int, acc *accumulator) {
	b := img.Bounds()
	count := 0
	var first *Point
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			neighbours := 0
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				if inBounds(b, x+d[0], y+d[1]) && img.NRGBAAt(x+d[0], y+d[1]).A > 0 {
					neighbours++
				}
			}
			if neighbours == 0 {
				count++
				if first == nil {
					first = &Point{X: x, Y: y}
				}
			}
		}
	}
	acc.metrics.StrayPixels += count
	if count == 0 {
		return
	}
	sev := Warning
	if count > 3 {
		sev = Error
	}
	acc.add(Finding{
		Check:    CheckStrays,
		Severity: sev,
		Message:  fmt.Sprintf("%d isolated pixel(s) with no neighbour. These read as noise at 1x.", count),
		Layer:    layer,
		Frame:    frame,
		At:       first,
		Count:    count,
	})
}

func checkOrphans(img *image.NRGBA, layer string, frame int, acc *accumulator) {
	b := img.Bounds()
	count := 0
	var first *Point
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			sameNeighbour := false
			opaqueNeighbour := false
			for dy := -1; dy <= 1 && !sameNeighbour; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if !inBounds(b, nx, ny) {
						continue
					}
					n := img.NRGBAAt(nx, ny)
					if n.A == 0 {
						continue
					}
					opaqueNeighbour = true
					if n.R == c.R && n.G == c.G && n.B == c.B && n.A == c.A {
						sameNeighbour = true
						break
					}
				}
			}
			if opaqueNeighbour && !sameNeighbour {
				count++
				if first == nil {
					first = &Point{X: x, Y: y}
				}
			}
		}
	}
	acc.metrics.OrphanPixels += count
	if count == 0 {
		return
	}
	acc.add(Finding{
		Check:    CheckOrphans,
		Severity: Note,
		Message:  fmt.Sprintf("%d pixel(s) whose colour matches none of their neighbours — the classic stray inside a fill or a shading band. A deliberate 1 px detail (an eye glint) looks the same: check before fixing.", count),
		Layer:    layer,
		Frame:    frame,
		At:       first,
		Count:    count,
	})
}

func checkGaps(img *image.NRGBA, layer string, frame int, expect *Expect, acc *accumulator) {
	pieces := components(img)
	if len(pieces) > acc.metrics.MaxComponents {
		acc.metrics.MaxComponents = len(pieces)
	}
	if len(pieces) <= 1 {
		return
	}

	// Sort smallest first so the report names the odd piece, not the body.
	sort.Slice(pieces, func(i, j int) bool { return pieces[i].n < pieces[j].n })

	sev := Note
	message := fmt.Sprintf("This frame is made of %d separate pieces.", len(pieces))
	if len(pieces) <= 3 {
		sev = Warning
		message = fmt.Sprintf("%d separate pieces. On a character that usually means a loose part; on an effect layer it is normal.", len(pieces))
	}
	if expect != nil && expect.SingleComponent {
		sev = Error
		message = fmt.Sprintf("%d separate pieces, but every frame was asked to be one connected shape. The smallest is %d px at (%d,%d).",
			len(pieces), pieces[0].n, pieces[0].rect.Min.X, pieces[0].rect.Min.Y)
	}

	smallest := pieces[0]
	rect := Rect{X: smallest.rect.Min.X, Y: smallest.rect.Min.Y, Width: smallest.rect.Dx(), Height: smallest.rect.Dy()}
	acc.add(Finding{
		Check:    CheckGaps,
		Severity: sev,
		Message:  message,
		Layer:    layer,
		Frame:    frame,
		At:       &Point{X: smallest.rect.Min.X, Y: smallest.rect.Min.Y},
		Rect:     &rect,
		Count:    len(pieces),
	})
}

func checkBounds(img *image.NRGBA, layer string, frame int, acc *accumulator) {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return
	}
	count := 0
	var first *Point
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			if x == b.Min.X || y == b.Min.Y || x == b.Max.X-1 || y == b.Max.Y-1 {
				count++
				if first == nil {
					first = &Point{X: x, Y: y}
				}
			}
		}
	}
	acc.metrics.EdgePixels += count
	if count == 0 {
		return
	}
	acc.add(Finding{
		Check:    CheckBounds,
		Severity: Warning,
		Message:  fmt.Sprintf("%d opaque pixel(s) touch the canvas edge. That is either a deliberate bleed or a move that clipped the art — only you know which.", count),
		Layer:    layer,
		Frame:    frame,
		At:       first,
		Count:    count,
	})
}

func checkOutline(img *image.NRGBA, layer string, frame int, acc *accumulator) {
	b := img.Bounds()
	counts := map[craft.RGB]int{}
	total := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			exposed := false
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if !inBounds(b, nx, ny) || img.NRGBAAt(nx, ny).A == 0 {
					exposed = true
					break
				}
			}
			if exposed {
				total++
				counts[craft.RGB{R: c.R, G: c.G, B: c.B, A: c.A}]++
			}
		}
	}
	if total == 0 {
		return
	}

	var best craft.RGB
	bestN := 0
	for c, n := range counts {
		if n > bestN {
			best, bestN = c, n
		}
	}
	share := float64(bestN) / float64(total)

	switch {
	case share >= 0.6 && share < 1.0:
		acc.add(Finding{
			Check:    CheckOutline,
			Severity: Warning,
			Message: fmt.Sprintf("The outline is %d%% %s but %d edge pixel(s) use a different colour. A broken outline reads as a hole in the silhouette.",
				int(share*100+0.5), best.Hex(), total-bestN),
			Layer: layer,
			Frame: frame,
			Count: total - bestN,
		})
	case share < 0.6:
		acc.add(Finding{
			Check:    CheckOutline,
			Severity: Note,
			Message:  "No single colour owns the silhouette edge, so this sprite is not outlined. That is a valid style — just keep it consistent.",
			Layer:    layer,
			Frame:    frame,
		})
	}
}

// checkBanding finds a long straight boundary between two colours, which the
// eye reads as a contour line.
func checkBanding(img *image.NRGBA, layer string, frame int, acc *accumulator) {
	b := img.Bounds()
	if b.Dx() < 4 || b.Dy() < 4 {
		return
	}
	threshold := maxInt(8, b.Dx()/3)

	worst, worstAt, worstPair := 0, (*Point)(nil), ""

	// Horizontal boundaries: compare row y with row y+1.
	for y := b.Min.Y; y < b.Max.Y-1; y++ {
		run := 0
		runStart := b.Min.X
		var prevA, prevB craft.RGB
		for x := b.Min.X; x < b.Max.X; x++ {
			a, bCol := img.NRGBAAt(x, y), img.NRGBAAt(x, y+1)
			if a.A == 0 || bCol.A == 0 {
				run = 0
				continue
			}
			ca := craft.RGB{R: a.R, G: a.G, B: a.B, A: a.A}
			cb := craft.RGB{R: bCol.R, G: bCol.G, B: bCol.B, A: bCol.A}
			if ca == cb {
				run = 0
				continue
			}
			if run > 0 && ca == prevA && cb == prevB {
				run++
			} else {
				run = 1
				runStart = x
			}
			prevA, prevB = ca, cb
			if run > worst {
				worst = run
				worstAt = &Point{X: runStart, Y: y}
				worstPair = ca.Hex() + " / " + cb.Hex()
			}
		}
	}

	if worst > acc.metrics.BandingRun {
		acc.metrics.BandingRun = worst
	}
	if worst >= threshold {
		acc.add(Finding{
			Check:    CheckBanding,
			Severity: Note,
			Message:  fmt.Sprintf("A %d px straight boundary between %s. Let the edge wander, or dither part of it, so it reads as a surface rather than a contour line.", worst, worstPair),
			Layer:    layer,
			Frame:    frame,
			At:       worstAt,
			Count:    worst,
		})
	}
}

// ---------------------------------------------------------------------------
// Expect contract
// ---------------------------------------------------------------------------

func checkExpect(d *doc.Dump, e *Expect, acc *accumulator) {
	for name, ranges := range e.LayerFrames {
		li := d.IndexOfLayer(name)
		if li < 0 {
			acc.add(Finding{
				Check:    CheckExpect,
				Severity: Error,
				Message:  fmt.Sprintf("The plan names a layer %q that does not exist. Layers present: %s.", name, strings.Join(d.LayerNames(), ", ")),
				Layer:    name,
			})
			continue
		}
		inRange := func(f int) bool {
			for _, r := range ranges {
				if f >= r[0] && f <= r[1] {
					return true
				}
			}
			return false
		}
		for f := 1; f <= d.FrameCount(); f++ {
			cel := d.Cel(li, f)
			hasInk := cel != nil && cel.Pixels != nil && hasOpaque(cel.Pixels)
			if hasInk && !inRange(f) {
				acc.add(Finding{
					Check:    CheckExpect,
					Severity: Error,
					Message:  fmt.Sprintf("Layer %q has ink on frame %d, outside the frames the plan gives it (%s).", name, f, formatRanges(ranges)),
					Layer:    name,
					Frame:    f,
				})
			}
			if !hasInk && inRange(f) {
				acc.add(Finding{
					Check:    CheckExpect,
					Severity: Error,
					Message:  fmt.Sprintf("Layer %q is empty on frame %d, but the plan puts ink there.", name, f),
					Layer:    name,
					Frame:    f,
				})
			}
		}
	}

	for _, pair := range e.MustNotOverlap {
		if len(pair) != 2 {
			continue
		}
		a, b := d.IndexOfLayer(pair[0]), d.IndexOfLayer(pair[1])
		if a < 0 || b < 0 {
			acc.add(Finding{
				Check:    CheckExpect,
				Severity: Error,
				Message:  fmt.Sprintf("The plan names layers %q and %q; at least one does not exist. Layers present: %s.", pair[0], pair[1], strings.Join(d.LayerNames(), ", ")),
			})
			continue
		}
		for f := 1; f <= d.FrameCount(); f++ {
			overlap, at := intersect(d.LayerImage(a, f), d.LayerImage(b, f))
			if overlap > 0 {
				acc.add(Finding{
					Check:    CheckExpect,
					Severity: Error,
					Message:  fmt.Sprintf("Layers %q and %q overlap on %d pixel(s) on frame %d, and the plan says they never should.", pair[0], pair[1], overlap, f),
					Frame:    f,
					At:       at,
					Count:    overlap,
				})
			}
		}
	}

	if e.PaletteSize != nil && len(d.Palette) > *e.PaletteSize {
		acc.add(Finding{
			Check:    CheckExpect,
			Severity: Error,
			Message:  fmt.Sprintf("The palette holds %d colours but the brief allows %d.", len(d.Palette), *e.PaletteSize),
			Count:    len(d.Palette),
		})
	}

	if e.Bounds != nil {
		for f := 1; f <= d.FrameCount(); f++ {
			img := d.Composite(f)
			r, ok := doc.Bounds(img)
			if !ok {
				continue
			}
			if r.Min.X < e.Bounds.X || r.Min.Y < e.Bounds.Y ||
				r.Max.X > e.Bounds.X+e.Bounds.Width || r.Max.Y > e.Bounds.Y+e.Bounds.Height {
				acc.add(Finding{
					Check:    CheckExpect,
					Severity: Error,
					Message:  fmt.Sprintf("Art on frame %d reaches (%d,%d)..(%d,%d), outside the box the brief allows.", f, r.Min.X, r.Min.Y, r.Max.X-1, r.Max.Y-1),
					Frame:    f,
				})
			}
		}
	}
}

func intersect(a, b *image.NRGBA) (int, *Point) {
	count := 0
	var first *Point
	bb := a.Bounds()
	for y := bb.Min.Y; y < bb.Max.Y; y++ {
		for x := bb.Min.X; x < bb.Max.X; x++ {
			if a.NRGBAAt(x, y).A > 0 && b.NRGBAAt(x, y).A > 0 {
				count++
				if first == nil {
					first = &Point{X: x, Y: y}
				}
			}
		}
	}
	return count, first
}

// ---------------------------------------------------------------------------
// Pixel helpers
// ---------------------------------------------------------------------------

func inBounds(b image.Rectangle, x, y int) bool {
	return x >= b.Min.X && y >= b.Min.Y && x < b.Max.X && y < b.Max.Y
}

func hasOpaque(img *image.NRGBA) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A > 0 {
				return true
			}
		}
	}
	return false
}

type piece struct {
	n    int
	rect image.Rectangle
}

// components counts 8-connected groups of opaque pixels, smallest last. A
// one-pixel diagonal is one stroke, not a row of dots.
func components(img *image.NRGBA) []piece {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil
	}

	seen := make([]bool, w*h)
	var out []piece

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if seen[y*w+x] || img.NRGBAAt(b.Min.X+x, b.Min.Y+y).A == 0 {
				continue
			}
			// Flood fill.
			stack := []image.Point{{X: x, Y: y}}
			seen[y*w+x] = true
			count := 0
			minX, minY, maxX, maxY := x, y, x, y
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				count++
				if p.X < minX {
					minX = p.X
				}
				if p.Y < minY {
					minY = p.Y
				}
				if p.X > maxX {
					maxX = p.X
				}
				if p.Y > maxY {
					maxY = p.Y
				}
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						nx, ny := p.X+dx, p.Y+dy
						if nx < 0 || ny < 0 || nx >= w || ny >= h {
							continue
						}
						if seen[ny*w+nx] || img.NRGBAAt(b.Min.X+nx, b.Min.Y+ny).A == 0 {
							continue
						}
						seen[ny*w+nx] = true
						stack = append(stack, image.Point{X: nx, Y: ny})
					}
				}
			}
			out = append(out, piece{n: count, rect: image.Rect(minX, minY, maxX+1, maxY+1)})
		}
	}
	return out
}

// tonalClusters counts distinct lightness bands. Under three reads flat, over
// thirty has no value structure.
func tonalClusters(colors map[craft.RGB]int) int {
	if len(colors) == 0 {
		return 0
	}
	bins := map[int]bool{}
	for c, n := range colors {
		if n < 2 {
			continue // single pixels are noise, not a tone
		}
		l := c.ToLab().L
		bins[int(l/5)] = true
	}
	return len(bins)
}

func spread(values []int) int {
	if len(values) == 0 {
		return 0
	}
	lo, hi := values[0], values[0]
	for _, v := range values {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return hi - lo
}

func formatRanges(ranges [][2]int) string {
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r[0] == r[1] {
			parts = append(parts, fmt.Sprintf("%d", r[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r[0], r[1]))
		}
	}
	return strings.Join(parts, ", ")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
