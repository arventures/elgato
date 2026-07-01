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
}

func report(d *elgato.Device, s elgato.LightState) deviceReport {
	return deviceReport{
		Name:        d.Label(),
		Serial:      d.Serial(),
		Host:        d.Host,
		On:          s.On == 1,
		Brightness:  s.Brightness,
		Temperature: elgato.MiredsToKelvin(s.Temperature),
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
	fmt.Fprintln(tw, "NAME\tHOST\tPOWER\tBRIGHTNESS\tTEMPERATURE")
	for _, r := range reports {
		power := "off"
		if r.On {
			power = "on"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d%%\t%dK\n", r.Name, r.Host, power, r.Brightness, r.Temperature)
	}
	_ = tw.Flush()
}

// reportPayload flattens a device's multi-light payload into reports.
func reportPayload(d *elgato.Device, p *elgato.LightsPayload) []deviceReport {
	var out []deviceReport
	for _, l := range p.Lights {
		out = append(out, report(d, l))
	}
	return out
}
