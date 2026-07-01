package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arventures/elgato/internal/config"
	"github.com/arventures/elgato/internal/discovery"
	"github.com/arventures/elgato/internal/elgato"
)

// errNoLights is returned when no target light could be found.
var errNoLights = errors.New(
	"no Elgato lights found\n" +
		"  • make sure the light is powered and on the same network/Wi-Fi\n" +
		"  • try: elgato --refresh list\n" +
		"  • or target it directly: elgato --host 10.0.0.11 on")

// resolveDevices returns the set of lights a command should act on, honoring
// --host, --light and --refresh. Strategy: use cached addresses first (fast),
// falling back to mDNS discovery, which also refreshes the cache.
func resolveDevices(ctx context.Context) ([]*elgato.Device, error) {
	// 1. Direct host override — no discovery, no config.
	if opts.host != "" {
		d := &elgato.Device{Host: opts.host, Port: opts.port}
		// Best-effort info fetch so we can show a real name; ignore failures.
		infoCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, _ = d.FetchInfo(infoCtx)
		return []*elgato.Device{d}, nil
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// 2. Try cached addresses unless a refresh was requested.
	if !opts.refresh {
		if devices := reachableFromCache(ctx, cfg); len(devices) > 0 {
			return devices, nil
		}
	}

	// 3. Discover on the network and refresh the cache.
	found, err := discovery.Discover(ctx, cfg.Timeout(opts.timeout))
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
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

	devices := filterByLight(found, opts.light)
	if len(devices) == 0 {
		return nil, errNoLights
	}
	return devices, nil
}

// reachableFromCache builds devices from cached config entries and keeps only
// the ones that answer quickly.
func reachableFromCache(ctx context.Context, cfg *config.Config) []*elgato.Device {
	var devices []*elgato.Device
	for name, l := range cfg.Lights {
		if l.Host == "" {
			continue
		}
		if opts.light != "" && !nameMatches(name, l.Serial, l.Host, opts.light) {
			continue
		}
		d := &elgato.Device{Name: name, Host: l.Host, Port: l.Port}
		probeCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		_, err := d.FetchInfo(probeCtx)
		cancel()
		if err == nil {
			devices = append(devices, d)
		}
	}
	return devices
}

func filterByLight(devices []*elgato.Device, target string) []*elgato.Device {
	if target == "" {
		return devices
	}
	var out []*elgato.Device
	for _, d := range devices {
		if nameMatches(d.Name, d.Serial(), d.Host, target) || strings.EqualFold(d.Label(), target) {
			out = append(out, d)
		}
	}
	return out
}

func nameMatches(name, serial, host, target string) bool {
	return strings.EqualFold(name, target) ||
		strings.EqualFold(serial, target) ||
		host == target
}
