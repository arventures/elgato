package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/arventures/elgato/internal/config"
	"github.com/arventures/elgato/internal/discovery"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "discover"},
		Short:   "Discover Elgato lights on the network and update the cache",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			found, err := discovery.Discover(ctx, cfg.Timeout(opts.timeout))
			if err != nil {
				return fmt.Errorf("discovery failed: %w", err)
			}
			for _, d := range found {
				infoCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				_, _ = d.FetchInfo(infoCtx)
				cancel()
			}
			if cfg.RememberDevices(found) {
				if err := cfg.Save(); err != nil {
					fmt.Fprintln(stderr, "warning: could not save config:", err)
				}
			}

			type row struct {
				Name     string `json:"name"`
				Serial   string `json:"serial"`
				Host     string `json:"host"`
				Port     int    `json:"port"`
				Firmware string `json:"firmware"`
			}
			rows := make([]row, 0, len(found))
			for _, d := range found {
				fw := ""
				if d.Info != nil {
					fw = d.Info.FirmwareVersion
				}
				rows = append(rows, row{d.Label(), d.Serial(), d.Host, d.Port, fw})
			}

			if opts.json {
				enc := json.NewEncoder(stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			if len(rows) == 0 {
				fmt.Fprintln(stdout, "No Elgato lights found on the network.")
				return nil
			}
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSERIAL\tHOST\tPORT\tFIRMWARE")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", r.Name, r.Serial, r.Host, r.Port, r.Firmware)
			}
			return tw.Flush()
		},
	}
}
