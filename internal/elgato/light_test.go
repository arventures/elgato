package elgato

import (
	"encoding/json"
	"testing"
)

func TestKelvinToMireds(t *testing.T) {
	cases := []struct {
		kelvin int
		want   int
	}{
		{4500, 222}, // 1_000_000/4500 = 222.2
		{4700, 213}, // matches the user's example cURL value
		{7000, 143}, // coolest, clamps to MinMireds
		{8000, 143}, // beyond range, clamps to MinMireds
		{2900, 344}, // warmest, clamps to MaxMireds
		{2000, 344}, // beyond range, clamps to MaxMireds
	}
	for _, c := range cases {
		if got := KelvinToMireds(c.kelvin); got != c.want {
			t.Errorf("KelvinToMireds(%d) = %d, want %d", c.kelvin, got, c.want)
		}
	}
}

func TestMiredsToKelvin(t *testing.T) {
	// 213 mireds -> 1_000_000/213 = 4694.8 -> nearest 50 -> 4700
	if got := MiredsToKelvin(213); got != 4700 {
		t.Errorf("MiredsToKelvin(213) = %d, want 4700", got)
	}
	// Round-trip a value that sits on the grid.
	if got := MiredsToKelvin(KelvinToMireds(5000)); got < 4900 || got > 5100 {
		t.Errorf("round-trip 5000K = %d, want within 100K", got)
	}
}

func TestParseAndApplyBrightness(t *testing.T) {
	cases := []struct {
		arg     string
		current int
		want    int
	}{
		{"50", 10, 50},
		{"+10", 45, 55},
		{"-20", 45, 25},
		{"+100", 80, 100}, // clamps high
		{"-100", 30, 0},   // clamps low
		{"200", 0, 100},   // absolute clamps high
		{"75%", 0, 75},    // trailing percent tolerated
	}
	for _, c := range cases {
		n, rel, err := ParseBrightnessArg(c.arg)
		if err != nil {
			t.Fatalf("ParseBrightnessArg(%q) error: %v", c.arg, err)
		}
		if got := ApplyBrightness(c.current, n, rel); got != c.want {
			t.Errorf("brightness %q on current %d = %d, want %d", c.arg, c.current, got, c.want)
		}
	}

	if _, _, err := ParseBrightnessArg("bright"); err == nil {
		t.Error("ParseBrightnessArg(\"bright\") should error")
	}
}

func TestParseKelvinArg(t *testing.T) {
	for _, in := range []string{"4500", "4500K", "4500k", " 4500 "} {
		got, err := ParseKelvinArg(in)
		if err != nil || got != 4500 {
			t.Errorf("ParseKelvinArg(%q) = %d, %v; want 4500", in, got, err)
		}
	}
	if _, err := ParseKelvinArg("warm"); err == nil {
		t.Error("ParseKelvinArg(\"warm\") should error")
	}
}

// TestPayloadShape guards that the JSON we send matches the exact structure of
// the user's working cURL request.
func TestPayloadShape(t *testing.T) {
	p := &LightsPayload{
		NumberOfLights: 1,
		Lights:         []LightState{{On: 1, Brightness: 50, Temperature: 213}},
	}
	got, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"numberOfLights":1,"lights":[{"on":1,"brightness":50,"temperature":213}]}`
	if string(got) != want {
		t.Errorf("payload JSON mismatch:\n got:  %s\n want: %s", got, want)
	}
}
