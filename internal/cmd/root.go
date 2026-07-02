// Package cmd wires up the elgato CLI.
package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

var opts struct {
	host    string
	port    int
	light   string
	refresh bool
	json    bool
	color   string
	timeout time.Duration
}

// NewRootCmd builds the root command tree.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "elgato",
		Short: "Control Elgato Key Lights from the command line",
		Long: "elgato controls Elgato Key Lights over their local REST API.\n" +
			"Lights are found automatically via mDNS; use --host to target one directly.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	pf := root.PersistentFlags()
	pf.StringVar(&opts.host, "host", "", "target a light directly by IP/hostname (skips discovery)")
	pf.IntVar(&opts.port, "port", 9123, "device API port")
	pf.StringVarP(&opts.light, "light", "l", "", "target a single light by name or serial")
	pf.BoolVar(&opts.refresh, "refresh", false, "force mDNS discovery instead of using cached addresses")
	pf.BoolVar(&opts.json, "json", false, "output machine-readable JSON")
	pf.StringVar(&opts.color, "color", "auto", "when to show the temperature color swatch: auto|always|never")
	pf.DurationVar(&opts.timeout, "timeout", 2*time.Second, "mDNS discovery timeout")

	root.AddCommand(
		newOnCmd(),
		newOffCmd(),
		newToggleCmd(),
		newBrightnessCmd(),
		newTempCmd(),
		newStatusCmd(),
		newListCmd(),
	)
	return root
}
