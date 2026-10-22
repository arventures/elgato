// Package homekit puts Elgato lights in the Apple Home app. It runs a HomeKit
// Accessory Protocol (HAP) bridge on the local network in which every light is
// a dimmable bulb with adjustable color temperature, so the Home app, Siri,
// scenes and automations can control it like any other HomeKit light.
package homekit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arventures/elgato/internal/elgato"
	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
)

// requestTimeout bounds each call to a light, so an unplugged one can't keep
// the Home app waiting.
const requestTimeout = 5 * time.Second

// Options configures the bridge.
type Options struct {
	Name    string        // bridge name shown while pairing, e.g. "Elgato Bridge"
	Version string        // CLI version, reported as the bridge's firmware
	Code    string        // 8-digit setup code, see SetupCode
	Addr    string        // HAP server address, e.g. ":51827"
	Poll    time.Duration // how often to sync each light's state into HomeKit
}

// Run serves the bridge until ctx is cancelled. Every device needs a serial
// number: it is the light's identity in HomeKit.
func Run(ctx context.Context, store hap.Store, devices []*elgato.Device, opts Options) error {
	serials := make([]string, 0, len(devices))
	for _, d := range devices {
		if d.Serial() == "" {
			return fmt.Errorf("%s: serial number unknown", d.Label())
		}
		serials = append(serials, d.Serial())
	}
	ids, err := accessoryIDs(store, serials)
	if err != nil {
		return err
	}

	bridge := accessory.NewBridge(accessory.Info{
		Name:         opts.Name,
		Manufacturer: "arventures",
		Model:        "elgato CLI",
		Firmware:     revision(opts.Version),
	})
	bridge.Id = 1

	lights := make([]*light, 0, len(devices))
	accessories := make([]*accessory.A, 0, len(devices))
	for _, d := range devices {
		l := newLight(d, ids[d.Serial()])
		// Seed HomeKit with the real state before anyone asks for it.
		if err := l.refresh(ctx); err != nil {
			log.Printf("%s (%s): not responding yet, so the Home app will show No Response: %v", d.Label(), d.Host, err)
		} else {
			log.Printf("%s (%s): %s", d.Label(), d.Host, l)
		}
		lights = append(lights, l)
		accessories = append(accessories, l.acc.A)
	}
	bridge.IdentifyFunc = func(*http.Request) {
		for _, l := range lights {
			go l.identify()
		}
	}

	server, err := hap.NewServer(store, bridge.A, accessories...)
	if err != nil {
		return err
	}
	server.Pin = opts.Code
	server.Addr = opts.Addr

	if server.IsPaired() {
		log.Printf("HomeKit bridge %q is paired and running", opts.Name)
	} else {
		log.Printf("HomeKit bridge %q is ready to pair. In the Home app tap + → Add Accessory → More options…, choose %q and enter %s",
			opts.Name, opts.Name, FormatCode(opts.Code))
	}

	go poll(ctx, lights, opts.Poll)
	if err := server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// light is one Elgato light exposed as a HomeKit lightbulb.
type light struct {
	dev         *elgato.Device
	acc         *accessory.Lightbulb
	on          *characteristic.On
	brightness  *characteristic.Brightness
	temperature *characteristic.ColorTemperature

	mu        sync.Mutex  // serializes device I/O, so a poll can't interleave with a write
	reachable atomic.Bool // whether the light answered the last poll
	described bool        // whether the accessory has the light's own name and model; guarded by mu
}

func newLight(d *elgato.Device, id uint64) *light {
	var info elgato.AccessoryInfo
	if d.Info != nil {
		info = *d.Info
	}
	acc := accessory.NewLightbulb(accessory.Info{
		Name:         accessoryName(d.Label()),
		Manufacturer: "Elgato",
		Model:        info.ProductName,
		SerialNumber: info.SerialNumber,
		Firmware:     revision(info.FirmwareVersion),
	})
	acc.Id = id

	l := &light{
		dev:         d,
		acc:         acc,
		on:          acc.Lightbulb.On,
		brightness:  characteristic.NewBrightness(),
		temperature: characteristic.NewColorTemperature(),
		described:   info.ProductName != "",
	}
	// HomeKit and Elgato both measure color temperature in mireds, so only the
	// range needs changing: HomeKit defaults to 140–500, the light does 143–344.
	l.temperature.SetMinValue(elgato.MinMireds)
	l.temperature.SetMaxValue(elgato.MaxMireds)
	_ = l.temperature.SetValue(elgato.MinMireds)
	acc.Lightbulb.AddC(l.brightness.C)
	acc.Lightbulb.AddC(l.temperature.C)

	l.on.OnSetRemoteValue(func(v bool) error {
		return l.set(func(s *elgato.LightState) {
			s.On = 0
			if v {
				s.On = 1
			}
		})
	})
	l.brightness.OnSetRemoteValue(func(v int) error {
		return l.set(func(s *elgato.LightState) { s.Brightness = elgato.ClampBrightness(v) })
	})
	l.temperature.OnSetRemoteValue(func(v int) error {
		return l.set(func(s *elgato.LightState) { s.Temperature = elgato.ClampMireds(v) })
	})

	// Reads come from the values the poll keeps current, but fail while the
	// light isn't answering, which the Home app shows as "No Response".
	for _, c := range []*characteristic.C{l.on.C, l.brightness.C, l.temperature.C} {
		c.ValueRequestFunc = func(*http.Request) (any, int) {
			if !l.reachable.Load() {
				return nil, hap.JsonStatusServiceCommunicationFailure
			}
			return c.Value(), 0
		}
	}
	acc.IdentifyFunc = func(*http.Request) { go l.identify() }
	return l
}

// set applies a change from HomeKit to the light. Returning an error makes the
// Home app report that the light didn't respond.
func (l *light) set(change func(*elgato.LightState)) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if _, err := l.dev.Apply(ctx, change); err != nil {
		log.Printf("%s: %v", l.dev.Label(), err)
		return err
	}
	return nil
}

// refresh reads the light and mirrors its state into HomeKit, which also
// pushes changes made elsewhere (Control Center, this CLI, the light's own
// button) to the Home app. HomeKit ignores a write that matches the value it
// already has, so keeping these values current is what lets every tap reach
// the light.
func (l *light) refresh(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	state, err := l.dev.GetState(ctx)
	if err == nil && len(state.Lights) == 0 {
		err = errors.New("the light reported no state")
	}
	if err != nil {
		l.reachable.Store(false)
		return err
	}
	l.reachable.Store(true)

	s := state.Lights[0]
	l.on.SetValue(s.On == 1)
	_ = l.brightness.SetValue(elgato.ClampBrightness(s.Brightness))
	_ = l.temperature.SetValue(elgato.ClampMireds(s.Temperature))
	if !l.described {
		l.describe(ctx)
	}
	return nil
}

// describe gives the accessory the light's own name, model and firmware, for
// a light that didn't answer when the bridge started. The Home app picks
// these up when you add the bridge. Call with mu held.
func (l *light) describe(ctx context.Context) {
	probe := *l.dev // FetchInfo caches onto the device, which other goroutines read
	info, err := probe.FetchInfo(ctx)
	if err != nil {
		return
	}
	l.acc.Info.Name.SetValue(accessoryName(probe.Label()))
	l.acc.Info.Model.SetValue(info.ProductName)
	l.acc.Info.FirmwareRevision.SetValue(revision(info.FirmwareVersion))
	l.described = true
	log.Printf("%s: identified as %s %q", l.dev.Label(), info.ProductName, probe.Label())
}

// identify blinks the light, which the Home app asks for during setup.
func (l *light) identify() {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := l.dev.Identify(ctx); err != nil {
		log.Printf("%s: identify: %v", l.dev.Label(), err)
	}
}

// String describes the state HomeKit currently holds for the light.
func (l *light) String() string {
	power := "off"
	if l.on.Value() {
		power = "on"
	}
	return fmt.Sprintf("%s, %d%%, %dK", power, l.brightness.Value(), elgato.MiredsToKelvin(l.temperature.Value()))
}

// poll refreshes every light on each tick until ctx is cancelled, logging
// when one stops or starts responding.
func poll(ctx context.Context, lights []*light, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		var wg sync.WaitGroup
		for _, l := range lights {
			wg.Go(func() {
				was := l.reachable.Load()
				err := l.refresh(ctx)
				switch {
				case ctx.Err() != nil:
				case err != nil && was:
					log.Printf("%s: not responding: %v", l.dev.Label(), err)
				case err == nil && !was:
					log.Printf("%s: responding again: %s", l.dev.Label(), l)
				}
			})
		}
		wg.Wait()
	}
}

// WaitForNetwork blocks until this machine has a LAN address. The bridge's
// mDNS responder only listens on the network interfaces that are up when it
// starts, so starting before Wi-Fi connects (at boot, say) would leave the
// Home app unable to find it.
func WaitForNetwork(ctx context.Context) error {
	logged := false
	for !hasLANAddress() {
		if !logged {
			log.Print("waiting for a network connection…")
			logged = true
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}

// hasLANAddress reports whether an interface that's up, isn't loopback and
// supports multicast has a routable IPv4 address.
func hasLANAddress() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil && !ipnet.IP.IsLinkLocalUnicast() {
				return true
			}
		}
	}
	return false
}
