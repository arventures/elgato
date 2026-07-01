package elgato

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Elgato expresses color temperature in "mireds" (micro reciprocal degrees):
// mireds = 1_000_000 / kelvin. The device accepts values in [MinMireds,
// MaxMireds], i.e. roughly 7000K (coolest) down to 2900K (warmest).
const (
	MinMireds = 143 // ~7000K, coolest
	MaxMireds = 344 // ~2900K, warmest

	MinBrightness = 0
	MaxBrightness = 100
)

// KelvinToMireds converts a color temperature in Kelvin to the Elgato
// temperature value, clamped to the device's supported range.
func KelvinToMireds(kelvin int) int {
	if kelvin <= 0 {
		return MaxMireds
	}
	m := int(math.Round(1_000_000 / float64(kelvin)))
	return clamp(m, MinMireds, MaxMireds)
}

// MiredsToKelvin converts an Elgato temperature value back to Kelvin,
// rounded to the nearest 50K for a friendly display.
func MiredsToKelvin(mireds int) int {
	if mireds <= 0 {
		return 0
	}
	k := 1_000_000 / float64(mireds)
	return int(math.Round(k/50) * 50)
}

// ClampBrightness constrains a brightness percentage to [0, 100].
func ClampBrightness(v int) int { return clamp(v, MinBrightness, MaxBrightness) }

// ClampMireds constrains a raw temperature value to the device range.
func ClampMireds(v int) int { return clamp(v, MinMireds, MaxMireds) }

// ParseBrightnessArg parses a brightness argument that is either absolute
// ("50") or relative ("+10" / "-10"). It returns the numeric magnitude and
// whether it should be applied relative to the light's current value.
func ParseBrightnessArg(arg string) (n int, relative bool, err error) {
	arg = strings.TrimSpace(strings.TrimSuffix(arg, "%"))
	if arg == "" {
		return 0, false, fmt.Errorf("empty brightness value")
	}
	relative = arg[0] == '+' || arg[0] == '-'
	n, err = strconv.Atoi(arg)
	if err != nil {
		return 0, false, fmt.Errorf("invalid brightness %q: want a number like 50, +10 or -10", arg)
	}
	return n, relative, nil
}

// ApplyBrightness resolves a parsed brightness argument against the light's
// current value and returns the new, clamped brightness.
func ApplyBrightness(current, n int, relative bool) int {
	if relative {
		return ClampBrightness(current + n)
	}
	return ClampBrightness(n)
}

// ParseKelvinArg parses a color-temperature argument in Kelvin. It tolerates a
// trailing "k"/"K" (e.g. "4500K").
func ParseKelvinArg(arg string) (int, error) {
	arg = strings.TrimSpace(arg)
	arg = strings.TrimSuffix(arg, "K")
	arg = strings.TrimSuffix(arg, "k")
	k, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil {
		return 0, fmt.Errorf("invalid temperature %q: want Kelvin, e.g. 4500", arg)
	}
	return k, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// LightState mirrors a single light's settings inside an Elgato device.
type LightState struct {
	On          int `json:"on"`          // 0 (off) or 1 (on)
	Brightness  int `json:"brightness"`  // 0..100
	Temperature int `json:"temperature"` // mireds, 143..344
}

// LightsPayload is the body of GET/PUT /elgato/lights.
type LightsPayload struct {
	NumberOfLights int          `json:"numberOfLights"`
	Lights         []LightState `json:"lights"`
}

// AccessoryInfo is the subset of GET /elgato/accessory-info we use. The
// serial number is stable across reboots and DHCP changes, so it is the key we
// use to identify a light over time.
type AccessoryInfo struct {
	ProductName     string `json:"productName"`
	SerialNumber    string `json:"serialNumber"`
	DisplayName     string `json:"displayName"`
	FirmwareVersion string `json:"firmwareVersion"`
}
