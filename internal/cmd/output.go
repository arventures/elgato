package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/arventures/elgato/internal/elgato"
)

// Output writers, overridable in tests.
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

// withDevices resolves the target lights and runs fn against each, joining any
// per-device errors so one unreachable light does not hide the others.
func withDevices(fn func(ctx context.Context, d *elgato.Device) error) error {
	ctx := context.Background()
	devices, err := resolveDevices(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range devices {
		if err := fn(ctx, d); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d.Label(), err))
		}
	}
	return errors.Join(errs...)
}

// deviceReport is the JSON representation of a light's state.
type deviceReport struct {
	Name        string `json:"name"`
	Serial      string `json:"serial,omitempty"`
	Host        string `json:"host"`
	On          bool   `json:"on"`
	Brightness  int    `json:"brightness"`
	Temperature int    `json:"temperatureKelvin"`
	Color       string `json:"color"` // "#rrggbb" for the temperature
}

func report(d *elgato.Device, s elgato.LightState) deviceReport {
	kelvin := elgato.MiredsToKelvin(s.Temperature)
	r, g, b := elgato.KelvinToRGB(kelvin)
	return deviceReport{
		Name:        d.Label(),
		Serial:      d.Serial(),
		Host:        d.Host,
		On:          s.On == 1,
		Brightness:  s.Brightness,
		Temperature: kelvin,
		Color:       elgato.HexColor(r, g, b),
	}
}

// printStates renders one or more light states as a table or JSON.
func printStates(reports []deviceReport) {
	if opts.json {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(reports)
		return
	}
	tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	// COLOR is kept last: the swatch contains invisible ANSI escapes that would
	// otherwise throw off tabwriter's column-width math.
	fmt.Fprintln(tw, "NAME\tHOST\tPOWER\tBRIGHTNESS\tTEMPERATURE\tCOLOR")
	showColor := colorEnabled()
	for _, r := range reports {
		power := "off"
		if r.On {
			power = "on"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d%%\t%dK\t%s\n", r.Name, r.Host, power, r.Brightness, r.Temperature, colorCell(r.Temperature, r.Color, showColor))
	}
	_ = tw.Flush()
}

// colorCell renders the temperature's color: a truecolor swatch plus hex when
// show is true, otherwise just the hex code.
func colorCell(kelvin int, hex string, show bool) string {
	if !show {
		return hex
	}
	r, g, b := elgato.KelvinToRGB(kelvin)
	// 24-bit foreground color, three full blocks, then reset.
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm███\x1b[0m %s", r, g, b, hex)
}

// colorEnabled decides whether to emit ANSI color, honoring --color and the
// NO_COLOR convention, and auto-detecting a terminal otherwise.
func colorEnabled() bool {
	switch opts.color {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
		return isTerminal(stdout)
	}
}

// isTerminal reports whether w is a character device (a TTY).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// reportPayload flattens a device's multi-light payload into reports.
func reportPayload(d *elgato.Device, p *elgato.LightsPayload) []deviceReport {
	var out []deviceReport
	for _, l := range p.Lights {
		out = append(out, report(d, l))
	}
	return out
}
