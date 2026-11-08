// Package craft holds the colour maths shared by the MCP server and the app, so
// a palette snap can never disagree with a palette report.
package craft

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RGB is an 8-bit colour with alpha, matching Aseprite's order.
type RGB struct {
	R, G, B, A uint8
}

// Fractions returns each channel as a 0..1 value.
func (c RGB) Fractions() (r, g, b, a float64) {
	return float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, float64(c.A) / 255
}

// Opaque reports whether the colour would be drawn.
func (c RGB) Opaque() bool { return c.A > 0 }

// Hex renders "#rrggbb", or "#rrggbbaa" when the colour is not fully opaque.
func (c RGB) Hex() string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

// ParseHex accepts #rgb, #rrggbb and #rrggbbaa (with or without the hash).
func ParseHex(s string) (RGB, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	switch len(s) {
	case 3:
		var out [3]uint8
		for i, ch := range s {
			v, err := strconv.ParseUint(string(ch)+string(ch), 16, 8)
			if err != nil {
				return RGB{}, fmt.Errorf("invalid hex colour %q", "#"+s)
			}
			out[i] = uint8(v)
		}
		return RGB{R: out[0], G: out[1], B: out[2], A: 255}, nil
	case 6, 8:
		v, err := strconv.ParseUint(s, 16, 64)
		if err != nil {
			return RGB{}, fmt.Errorf("invalid hex colour %q", "#"+s)
		}
		if len(s) == 6 {
			return RGB{
				R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255,
			}, nil
		}
		return RGB{
			R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v),
		}, nil
	default:
		return RGB{}, fmt.Errorf("invalid hex colour %q: use #rgb, #rrggbb or #rrggbbaa", "#"+s)
	}
}

// MustHex is ParseHex for constants in code and tests.
func MustHex(s string) RGB {
	c, err := ParseHex(s)
	if err != nil {
		panic(err)
	}
	return c
}

// ---------------------------------------------------------------------------
// Colour spaces
// ---------------------------------------------------------------------------

// Lab is CIELAB under D65: L* is lightness (0..100), a* green→red, b* blue→yellow.
//
// Used over RGB distance, which calls different greens close and identical
// greys far.
type Lab struct {
	L, A, B float64
}

// LCh is the cylindrical form of Lab: lightness, chroma, hue in degrees.
type LCh struct {
	L, C, H float64
}

// whiteD65 is the reference white for sRGB.
var whiteD65 = [3]float64{0.95047, 1.0, 1.08883}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func linearToSrgb(v float64) float64 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1.0/2.4) - 0.055
}

func labF(t float64) float64 {
	const epsilon = 216.0 / 24389.0
	const kappa = 24389.0 / 27.0
	if t > epsilon {
		return math.Cbrt(t)
	}
	return (kappa*t + 16) / 116
}

func labFInv(t float64) float64 {
	const delta = 6.0 / 29.0
	if t > delta {
		return t * t * t
	}
	return 3 * delta * delta * (t - 4.0/29.0)
}

// ToLab converts sRGB to CIELAB.
func (c RGB) ToLab() Lab {
	r := srgbToLinear(float64(c.R) / 255)
	g := srgbToLinear(float64(c.G) / 255)
	b := srgbToLinear(float64(c.B) / 255)

	x := 0.4124564*r + 0.3575761*g + 0.1804375*b
	y := 0.2126729*r + 0.7151522*g + 0.0721750*b
	z := 0.0193339*r + 0.1191920*g + 0.9503041*b

	fx := labF(x / whiteD65[0])
	fy := labF(y / whiteD65[1])
	fz := labF(z / whiteD65[2])

	return Lab{
		L: 116*fy - 16,
		A: 500 * (fx - fy),
		B: 200 * (fy - fz),
	}
}

// ToLCh converts to cylindrical Lab.
func (l Lab) ToLCh() LCh {
	c := math.Hypot(l.A, l.B)
	h := math.Atan2(l.B, l.A) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return LCh{L: l.L, C: c, H: h}
}

// ToLab converts back to rectangular Lab.
func (c LCh) ToLab() Lab {
	rad := c.H * math.Pi / 180
	return Lab{L: c.L, A: c.C * math.Cos(rad), B: c.C * math.Sin(rad)}
}

// RGB converts Lab back to 8-bit sRGB, clamping out-of-gamut results.
func (l Lab) RGB() RGB {
	fy := (l.L + 16) / 116
	fx := fy + l.A/500
	fz := fy - l.B/200

	x := whiteD65[0] * labFInv(fx)
	y := whiteD65[1] * labFInv(fy)
	z := whiteD65[2] * labFInv(fz)

	r := 3.2404542*x - 1.5371385*y - 0.4985314*z
	g := -0.9692660*x + 1.8760108*y + 0.0415560*z
	b := 0.0556434*x - 0.2040259*y + 1.0572252*z

	return RGB{
		R: to8(linearToSrgb(clamp01(r))),
		G: to8(linearToSrgb(clamp01(g))),
		B: to8(linearToSrgb(clamp01(b))),
		A: 255,
	}
}

// inGamut reports whether a Lab colour can be shown in sRGB without clamping.
func (l Lab) inGamut() bool {
	fy := (l.L + 16) / 116
	fx := fy + l.A/500
	fz := fy - l.B/200

	x := whiteD65[0] * labFInv(fx)
	y := whiteD65[1] * labFInv(fy)
	z := whiteD65[2] * labFInv(fz)

	r := 3.2404542*x - 1.5371385*y - 0.4985314*z
	g := -0.9692660*x + 1.8760108*y + 0.0415560*z
	b := 0.0556434*x - 0.2040259*y + 1.0572252*z

	const tolerance = 1e-6
	return within(r, tolerance) && within(g, tolerance) && within(b, tolerance)
}

func within(v, tol float64) bool { return v >= -tol && v <= 1+tol }

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func to8(v float64) uint8 {
	return uint8(math.Round(clamp01(v) * 255))
}

// ---------------------------------------------------------------------------
// Distance
// ---------------------------------------------------------------------------

// DeltaE76 is Euclidean distance in Lab: cheap, and enough for most palettes.
func DeltaE76(a, b RGB) float64 {
	la, lb := a.ToLab(), b.ToLab()
	return math.Sqrt(sq(la.L-lb.L) + sq(la.A-lb.A) + sq(la.B-lb.B))
}

// DeltaE2000 accounts for hue and chroma interaction, where DeltaE76 goes wrong
// on saturated colours. Snapping uses this one.
func DeltaE2000(c1, c2 RGB) float64 {
	x, y := c1.ToLab(), c2.ToLab()
	return deltaE2000Lab(x, y)
}

func deltaE2000Lab(x, y Lab) float64 {
	const (
		kL = 1.0
		kC = 1.0
		kH = 1.0
	)

	c1 := math.Hypot(x.A, x.B)
	c2 := math.Hypot(y.A, y.B)
	cBar := (c1 + c2) / 2
	cBar7 := math.Pow(cBar, 7)
	g := 0.5 * (1 - math.Sqrt(cBar7/(cBar7+math.Pow(25, 7))))

	a1p := (1 + g) * x.A
	a2p := (1 + g) * y.A
	c1p := math.Hypot(a1p, x.B)
	c2p := math.Hypot(a2p, y.B)

	h1p := hueDegrees(x.B, a1p)
	h2p := hueDegrees(y.B, a2p)

	dLp := y.L - x.L
	dCp := c2p - c1p

	var dhp float64
	switch {
	case c1p*c2p == 0:
		dhp = 0
	case math.Abs(h2p-h1p) <= 180:
		dhp = h2p - h1p
	case h2p-h1p > 180:
		dhp = h2p - h1p - 360
	default:
		dhp = h2p - h1p + 360
	}
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(dhp*math.Pi/360)

	lBarP := (x.L + y.L) / 2
	cBarP := (c1p + c2p) / 2

	var hBarP float64
	switch {
	case c1p*c2p == 0:
		hBarP = h1p + h2p
	case math.Abs(h1p-h2p) <= 180:
		hBarP = (h1p + h2p) / 2
	case h1p+h2p < 360:
		hBarP = (h1p + h2p + 360) / 2
	default:
		hBarP = (h1p + h2p - 360) / 2
	}

	t := 1 -
		0.17*math.Cos((hBarP-30)*math.Pi/180) +
		0.24*math.Cos(2*hBarP*math.Pi/180) +
		0.32*math.Cos((3*hBarP+6)*math.Pi/180) -
		0.20*math.Cos((4*hBarP-63)*math.Pi/180)

	dTheta := 30 * math.Exp(-sq((hBarP-275)/25))
	cBarP7 := math.Pow(cBarP, 7)
	rC := 2 * math.Sqrt(cBarP7/(cBarP7+math.Pow(25, 7)))
	sL := 1 + (0.015*sq(lBarP-50))/math.Sqrt(20+sq(lBarP-50))
	sC := 1 + 0.045*cBarP
	sH := 1 + 0.015*cBarP*t
	rT := -math.Sin(2*dTheta*math.Pi/180) * rC

	return math.Sqrt(
		sq(dLp/(kL*sL)) +
			sq(dCp/(kC*sC)) +
			sq(dHp/(kH*sH)) +
			rT*(dCp/(kC*sC))*(dHp/(kH*sH)))
}

func hueDegrees(b, a float64) float64 {
	if a == 0 && b == 0 {
		return 0
	}
	h := math.Atan2(b, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

func sq(v float64) float64 { return v * v }

// Nearest returns the closest palette index by DeltaE2000 and the distance. An
// empty palette returns -1.
func Nearest(c RGB, palette []RGB) (int, float64) {
	best, bestD := -1, math.Inf(1)
	for i, p := range palette {
		d := DeltaE2000(c, p)
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best, bestD
}

// ---------------------------------------------------------------------------
// Ramps
// ---------------------------------------------------------------------------

// RampOptions tunes GenerateRamp. The zero value is a sensible default.
type RampOptions struct {
	// Steps is how many colours the ramp holds. Default 5.
	Steps int
	// Span is how far in L* the ramp reaches either side of the base. Default
	// 18, about three visible steps on a 32×32 sprite.
	Span float64
	// ShadowHue and HighlightHue are the hues the ramp drifts toward. Defaults
	// 240 (cool blue) and 60 (warm yellow).
	ShadowHue    float64
	HighlightHue float64
	// ShadowPull and HighlightPull are how far to those hues the extremes get,
	// 0..1. Defaults 0.12 and 0.10.
	ShadowPull    float64
	HighlightPull float64
}

func (o RampOptions) withDefaults() RampOptions {
	if o.Steps <= 0 {
		o.Steps = 5
	}
	if o.Span <= 0 {
		o.Span = 18
	}
	if o.ShadowHue == 0 {
		o.ShadowHue = 240
	}
	if o.HighlightHue == 0 {
		o.HighlightHue = 60
	}
	if o.ShadowPull <= 0 {
		o.ShadowPull = 0.12
	}
	if o.HighlightPull <= 0 {
		o.HighlightPull = 0.10
	}
	return o
}

// Ramp is a shading ramp from darkest to lightest.
type Ramp struct {
	Colors []RGB
	Base   int // index of the colour closest to the one the ramp was built from
}

// GenerateRamp builds a shading ramp from one base colour. Real shadows drift
// cool and highlights warm; a ramp that only changes brightness reads as
// plastic. The result is verified before it is returned.
func GenerateRamp(base RGB, opts RampOptions) (Ramp, error) {
	o := opts.withDefaults()
	if o.Steps < 2 || o.Steps > 64 {
		return Ramp{}, errors.New("steps must be between 2 and 64")
	}

	src := base.ToLab().ToLCh()
	lo := math.Max(2, src.L-o.Span)
	hi := math.Min(98, src.L+o.Span)
	if hi-lo < float64(o.Steps) {
		return Ramp{}, fmt.Errorf("base colour leaves no room for %d steps", o.Steps)
	}

	colors := make([]RGB, o.Steps)
	baseIdx := 0
	bestDist := math.Inf(1)

	for i := 0; i < o.Steps; i++ {
		t := float64(i) / float64(o.Steps-1) // 0 darkest, 1 lightest

		l := lo + (hi-lo)*t

		// Hue drifts toward the cool anchor low and the warm one high, never
		// more than Pull of the way.
		hue := src.H
		hue += shortestAngle(o.ShadowHue-src.H) * o.ShadowPull * (1 - t)
		hue += shortestAngle(o.HighlightHue-src.H) * o.HighlightPull * t
		hue = math.Mod(hue+360, 360)

		// Chroma is given up at both extremes so the ends do not read as another
		// material.
		chroma := src.C * (1 - 0.15*math.Abs(2*t-1))

		lch := LCh{L: l, C: chroma, H: hue}
		colors[i] = fitToGamut(lch)

		if d := DeltaE2000(base, colors[i]); d < bestDist {
			bestDist, baseIdx = d, i
		}
	}

	r := Ramp{Colors: colors, Base: baseIdx}
	if err := r.Verify(); err != nil {
		return r, err
	}
	return r, nil
}

// fitToGamut reduces chroma until the colour fits sRGB, keeping lightness and
// hue. Saturated bases otherwise clip to a different hue.
func fitToGamut(c LCh) RGB {
	if c.ToLab().inGamut() {
		return c.ToLab().RGB()
	}
	lo, hi := 0.0, c.C
	for i := 0; i < 24; i++ {
		mid := (lo + hi) / 2
		trial := LCh{L: c.L, C: mid, H: c.H}
		if trial.ToLab().inGamut() {
			lo = mid
		} else {
			hi = mid
		}
	}
	return LCh{L: c.L, C: lo, H: c.H}.ToLab().RGB()
}

func shortestAngle(deg float64) float64 {
	deg = math.Mod(deg+540, 360) - 180
	return deg
}

// Verify checks a usable ramp: lightness rises monotonically and neighbouring
// steps are far enough apart to read as different colours.
func (r Ramp) Verify() error {
	if len(r.Colors) < 2 {
		return errors.New("ramp needs at least two colours")
	}
	const minStep = 1.5 // ~1.5 DeltaE is roughly one just-noticeable difference

	prev := r.Colors[0].ToLab().L
	for i := 1; i < len(r.Colors); i++ {
		l := r.Colors[i].ToLab().L
		if l <= prev {
			return fmt.Errorf("ramp step %d is not lighter than step %d (%.1f -> %.1f)", i, i-1, prev, l)
		}
		prev = l
	}
	for i := 1; i < len(r.Colors); i++ {
		if d := DeltaE2000(r.Colors[i-1], r.Colors[i]); d < minStep {
			return fmt.Errorf("ramp steps %d and %d are only %.1f apart; they will read as one colour", i-1, i, d)
		}
	}
	return nil
}

// Shade moves a colour along a ramp by steps, positive meaning lighter.
//
// This exists so "make this darker" never means "subtract from every channel",
// which looks muddy.
func Shade(c RGB, steps int) RGB {
	if steps == 0 {
		return c
	}
	n := steps
	if n < 0 {
		n = -n
	}
	if n > 12 {
		n = 12
	}
	span := 4.0
	ramp, err := GenerateRamp(c, RampOptions{Steps: n*2 + 1, Span: span * float64(n)})
	if err != nil {
		// Pure black or white comes back as itself rather than as an error the
		// caller has to special-case.
		return c
	}
	idx := ramp.Base + steps
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ramp.Colors) {
		idx = len(ramp.Colors) - 1
	}
	return ramp.Colors[idx]
}
