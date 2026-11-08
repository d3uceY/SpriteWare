package craft

import (
	"math"
	"testing"
)

// The numbers a colour space must get right. If these drift, every palette
// report and snap is wrong in a way that looks plausible.
func TestLabMatchesKnownValues(t *testing.T) {
	cases := []struct {
		hex  string
		want float64
		tol  float64
	}{
		{"#ffffff", 100.0, 0.01},
		{"#000000", 0.0, 0.01},
		{"#808080", 53.585, 0.05},
	}
	for _, c := range cases {
		got := MustHex(c.hex).ToLab().L
		if math.Abs(got-c.want) > c.tol {
			t.Errorf("%s: L* = %.3f, want %.3f (±%.2f)", c.hex, got, c.want, c.tol)
		}
	}
}

func TestLabRoundTrip(t *testing.T) {
	for _, hex := range []string{"#123456", "#ff8800", "#7ee787", "#0b0d12"} {
		src := MustHex(hex)
		back := src.ToLab().RGB()
		if back.R != src.R || back.G != src.G || back.B != src.B {
			t.Errorf("%s round-tripped to %s", hex, back.Hex())
		}
	}
}

func TestParseHex(t *testing.T) {
	ok := map[string]string{
		"#abc":     "#aabbcc",
		"aabbcc":   "#aabbcc",
		"#AABBCC":  "#aabbcc",
		"#1122334": "",
	}
	for in, want := range ok {
		if want == "" {
			if _, err := ParseHex(in); err == nil {
				t.Errorf("ParseHex(%q) accepted an invalid colour", in)
			}
			continue
		}
		got, err := ParseHex(in)
		if err != nil {
			t.Errorf("ParseHex(%q) returned an error: %v", in, err)
			continue
		}
		if got.Hex() != want {
			t.Errorf("ParseHex(%q) = %s, want %s", in, got.Hex(), want)
		}
	}
	alpha, err := ParseHex("#11223344")
	if err != nil {
		t.Fatalf("8-digit hex rejected: %v", err)
	}
	if alpha.A != 0x44 {
		t.Errorf("alpha = %d, want 0x44", alpha.A)
	}
}

func TestDeltaEIsZeroForIdenticalAndSymmetric(t *testing.T) {
	a, b := MustHex("#3355aa"), MustHex("#aa5533")
	if d := DeltaE2000(a, a); d != 0 {
		t.Errorf("identical colours have DeltaE %.4f, want 0", d)
	}
	if x, y := DeltaE2000(a, b), DeltaE2000(b, a); math.Abs(x-y) > 1e-9 {
		t.Errorf("DeltaE is not symmetric: %.6f vs %.6f", x, y)
	}
	if DeltaE2000(a, b) <= DeltaE2000(a, MustHex("#3355ab")) {
		t.Error("a near-identical colour should be closer than a very different one")
	}
}

// A ramp that only changes brightness reads as plastic. The hue has to move.
func TestRampShiftsHueInOppositeDirections(t *testing.T) {
	base := MustHex("#7a3b2e")
	ramp, err := GenerateRamp(base, RampOptions{Steps: 5})
	if err != nil {
		t.Fatalf("GenerateRamp returned an error: %v", err)
	}
	if len(ramp.Colors) != 5 {
		t.Fatalf("got %d steps, want 5", len(ramp.Colors))
	}
	if err := ramp.Verify(); err != nil {
		t.Fatalf("ramp failed its own check: %v", err)
	}

	darkest := ramp.Colors[0].ToLab().ToLCh()
	lightest := ramp.Colors[len(ramp.Colors)-1].ToLab().ToLCh()

	if darkest.L >= lightest.L {
		t.Fatalf("ramp is not ordered dark to light: %.1f then %.1f", darkest.L, lightest.L)
	}
	// A warm red-orange base: shadows pull toward blue, highlights toward
	// yellow, so the ends differ in hue.
	diff := math.Abs(shortestAngle(lightest.H - darkest.H))
	if diff < 4 {
		t.Errorf("hue only moved %.1f degrees between the ends; that is brightness banding, not a ramp", diff)
	}
}

func TestRampRejectsImpossibleRequests(t *testing.T) {
	if _, err := GenerateRamp(MustHex("#808080"), RampOptions{Steps: 1}); err == nil {
		t.Error("a one-step ramp was accepted")
	}
}

func TestShadeDarkensAndLightens(t *testing.T) {
	base := MustHex("#6a8f4f")
	darker := Shade(base, -2)
	lighter := Shade(base, 2)

	if darker.ToLab().L >= base.ToLab().L {
		t.Errorf("Shade(-2) produced %s, which is not darker than %s", darker.Hex(), base.Hex())
	}
	if lighter.ToLab().L <= base.ToLab().L {
		t.Errorf("Shade(+2) produced %s, which is not lighter than %s", lighter.Hex(), base.Hex())
	}
}

// An exact match has to come back at distance zero: a snap step that quietly
// picks a neighbour is how a sprite drifts out of its palette unnoticed.
func TestNearestReturnsExactMatch(t *testing.T) {
	palette := []RGB{MustHex("#112233"), MustHex("#445566")}
	idx, dist := Nearest(MustHex("#445566"), palette)
	if idx != 1 || dist != 0 {
		t.Errorf("Nearest = (%d, %.3f), want (1, 0)", idx, dist)
	}
}
