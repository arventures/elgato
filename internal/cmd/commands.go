package cmd

import (
	"context"

	"github.com/arventures/elgato/internal/elgato"
	"github.com/spf13/cobra"
)

// setPower turns every targeted light on (1) or off (0) and prints the result.
func setPower(on int) error {
	var reports []deviceReport
	err := withDevices(func(ctx context.Context, d *elgato.Device) error {
		state, err := d.Apply(ctx, func(l *elgato.LightState) { l.On = on })
		if err != nil {
			return err
		}
		reports = append(reports, reportPayload(d, state)...)
		return nil
	})
	printStates(reports)
	return err
}

func newOnCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "on",
		Short: "Turn lights on",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return setPower(1) },
	}
}

func newOffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Turn lights off",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return setPower(0) },
	}
}

func newToggleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "toggle",
		Short: "Toggle lights on/off",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var reports []deviceReport
			err := withDevices(func(ctx context.Context, d *elgato.Device) error {
				state, err := d.Apply(ctx, func(l *elgato.LightState) { l.On = 1 - l.On })
				if err != nil {
					return err
				}
				reports = append(reports, reportPayload(d, state)...)
				return nil
			})
			printStates(reports)
			return err
		},
	}
}

func newBrightnessCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "brightness <0-100|+N|-N>",
		Short: "Set brightness (absolute or relative)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, relative, err := elgato.ParseBrightnessArg(args[0])
			if err != nil {
				return err
			}
			var reports []deviceReport
			runErr := withDevices(func(ctx context.Context, d *elgato.Device) error {
				state, err := d.Apply(ctx, func(l *elgato.LightState) {
					l.Brightness = elgato.ApplyBrightness(l.Brightness, n, relative)
				})
				if err != nil {
					return err
				}
				reports = append(reports, reportPayload(d, state)...)
				return nil
			})
			printStates(reports)
			return runErr
		},
	}
}

func newTempCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "temp <kelvin>",
		Aliases: []string{"temperature"},
		Short:   "Set color temperature in Kelvin (≈2900–7000K)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kelvin, err := elgato.ParseKelvinArg(args[0])
			if err != nil {
				return err
			}
			mireds := elgato.KelvinToMireds(kelvin)
			var reports []deviceReport
			runErr := withDevices(func(ctx context.Context, d *elgato.Device) error {
				state, err := d.Apply(ctx, func(l *elgato.LightState) { l.Temperature = mireds })
				if err != nil {
					return err
				}
				reports = append(reports, reportPayload(d, state)...)
				return nil
			})
			printStates(reports)
			return runErr
		},
	}
	return cmd
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current state of lights",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var reports []deviceReport
			err := withDevices(func(ctx context.Context, d *elgato.Device) error {
				state, err := d.GetState(ctx)
				if err != nil {
					return err
				}
				reports = append(reports, reportPayload(d, state)...)
				return nil
			})
			printStates(reports)
			return err
		},
	}
}
