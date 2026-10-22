// Package discovery finds Elgato lights on the local network via mDNS/Bonjour.
// Elgato Key Lights advertise the service type "_elg._tcp" in the "local."
// domain.
package discovery

import (
	"context"
	"time"

	"github.com/arventures/elgato/internal/elgato"
	"github.com/libp2p/zeroconf/v2"
)

const serviceType = "_elg._tcp"

// Discover browses the local network for Elgato lights, collecting responses
// for up to timeout. It returns one Device per unique host. mDNS runs over UDP
// and a light's reply now and then gets lost on Wi-Fi, so a browse that finds
// nothing is tried once more before reporting no lights.
func Discover(ctx context.Context, timeout time.Duration) ([]*elgato.Device, error) {
	devices, err := browse(ctx, timeout)
	if err != nil || len(devices) > 0 || ctx.Err() != nil {
		return devices, err
	}
	return browse(ctx, timeout)
}

// browse runs a single mDNS browse for up to timeout.
func browse(ctx context.Context, timeout time.Duration) ([]*elgato.Device, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	entries := make(chan *zeroconf.ServiceEntry)
	var devices []*elgato.Device
	seen := make(map[string]bool)
	done := make(chan struct{})

	go func() {
		for e := range entries {
			d := deviceFromEntry(e)
			if d == nil || seen[d.Host] {
				continue
			}
			seen[d.Host] = true
			devices = append(devices, d)
		}
		close(done)
	}()

	if err := zeroconf.Browse(ctx, serviceType, "local.", entries); err != nil {
		return nil, err
	}

	<-ctx.Done() // browse runs until the timeout elapses
	<-done       // wait for the collector to drain and exit
	return devices, nil
}

func deviceFromEntry(e *zeroconf.ServiceEntry) *elgato.Device {
	if e == nil {
		return nil
	}
	host := ""
	switch {
	case len(e.AddrIPv4) > 0:
		host = e.AddrIPv4[0].String()
	case len(e.AddrIPv6) > 0:
		host = e.AddrIPv6[0].String()
	default:
		return nil
	}
	return &elgato.Device{
		Name: e.Instance,
		Host: host,
		Port: e.Port,
	}
}
