package elgato

import (
	"fmt"
	"math"
)

// KelvinToRGB approximates the sRGB color a black-body radiator emits at the
// given color temperature. It uses Tanner Helland's well-known approximation,
// which is accurate enough for a visual swatch across roughly 1000K–40000K.
// Warm temperatures (~2900K) come out amber; cool ones (~7000K) come out a
// bluish white.
func KelvinToRGB(kelvin int) (r, g, b uint8) {
	t := float64(kelvin) / 100.0

	var red, green, blue float64

	// Red channel.
	if t <= 66 {
		red = 255
	} else {
		red = 329.698727446 * math.Pow(t-60, -0.1332047592)
	}

	// Green channel.
	if t <= 66 {
		green = 99.4708025861*math.Log(t) - 161.1195681661
	} else {
		green = 288.1221695283 * math.Pow(t-60, -0.0755148492)
	}

	// Blue channel.
	switch {
	case t >= 66:
		blue = 255
	case t <= 19:
		blue = 0
	default:
		blue = 138.5177312231*math.Log(t-10) - 305.0447927307
	}

	return clampByte(red), clampByte(green), clampByte(blue)
}

// HexColor renders an RGB triple as "#rrggbb".
func HexColor(r, g, b uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func clampByte(v float64) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	default:
		return uint8(math.Round(v))
	}
}
