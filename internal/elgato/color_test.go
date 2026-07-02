package elgato

import "testing"

func TestKelvinToRGBRange(t *testing.T) {
	// Across the whole plausible range, channels must stay valid bytes.
	for k := 1000; k <= 12000; k += 250 {
		r, g, b := KelvinToRGB(k)
		_ = r // uint8 by construction, but assert intent for readability
		_ = g
		_ = b
		if k > 0 && r == 0 && g == 0 && b == 0 {
			t.Errorf("KelvinToRGB(%d) produced pure black", k)
		}
	}
}

func TestKelvinToRGBWarmVsCool(t *testing.T) {
	// Warm light is red-dominant with little blue.
	wr, wg, wb := KelvinToRGB(2900)
	if wr != 255 {
		t.Errorf("warm red = %d, want 255", wr)
	}
	if !(wr >= wg && wg >= wb) {
		t.Errorf("warm channels not ordered r>=g>=b: %d,%d,%d", wr, wg, wb)
	}

	// Cool light is blue-saturated.
	_, _, cb := KelvinToRGB(7000)
	if cb != 255 {
		t.Errorf("cool blue = %d, want 255", cb)
	}

	// Cooler light has strictly more blue than warmer light.
	if _, _, warmBlue := KelvinToRGB(3000); warmBlue >= cb {
		t.Errorf("expected cooler light to have more blue: warm=%d cool=%d", warmBlue, cb)
	}
}

func TestHexColor(t *testing.T) {
	if got := HexColor(255, 222, 195); got != "#ffdec3" {
		t.Errorf("HexColor(255,222,195) = %q, want #ffdec3", got)
	}
	if got := HexColor(0, 0, 0); got != "#000000" {
		t.Errorf("HexColor(0,0,0) = %q, want #000000", got)
	}
}
