package homekit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/arventures/elgato/internal/elgato"
	"github.com/brutella/hap"
)

// keyLight is the light a fakeLight plays.
var keyLight = elgato.AccessoryInfo{
	ProductName:     "Elgato Key Light Air",
	SerialNumber:    "CW30K1A07261",
	DisplayName:     "Arvi's KeyLight",
	FirmwareVersion: "1.0.3",
}

// fakeLight serves the parts of the Elgato API the bridge uses.
type fakeLight struct {
	mu         sync.Mutex
	state      elgato.LightState
	identified int
}

func (f *fakeLight) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method + " " + r.URL.Path {
	case "GET /elgato/lights":
	case "PUT /elgato/lights":
		var p elgato.LightsPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil || len(p.Lights) != 1 {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		f.state = p.Lights[0]
	case "POST /elgato/identify":
		f.identified++
		return
	case "GET /elgato/accessory-info":
		_ = json.NewEncoder(w).Encode(keyLight)
		return
	default:
		http.NotFound(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(elgato.LightsPayload{NumberOfLights: 1, Lights: []elgato.LightState{f.state}})
}

func (f *fakeLight) snapshot() (elgato.LightState, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, f.identified
}

// serve starts f and returns a device pointing at it.
func serve(t *testing.T, f *fakeLight) (*elgato.Device, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	p, _ := strconv.Atoi(port)
	info := keyLight
	return &elgato.Device{Host: host, Port: p, Info: &info}, srv
}

// fromHome stands in for a write from the Home app: hap only passes a change
// on to the light when it arrives with a request.
var fromHome = httptest.NewRequest(http.MethodPut, "/characteristics", nil)

func TestLightMirrorsDevice(t *testing.T) {
	f := &fakeLight{state: elgato.LightState{On: 1, Brightness: 40, Temperature: 213}}
	d, _ := serve(t, f)
	l := newLight(d, 2)

	if err := l.refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !l.on.Value() || l.brightness.Value() != 40 || l.temperature.Value() != 213 {
		t.Errorf("HomeKit holds %s, want on, 40%%, 213 mireds", l)
	}
	if got := l.acc.Info.Name.Value(); got != "Arvi's KeyLight" {
		t.Errorf("accessory name = %q, want Arvi's KeyLight", got)
	}
}

func TestLightOfflineAtStartupGetsItsName(t *testing.T) {
	f := &fakeLight{state: elgato.LightState{On: 1, Brightness: 40, Temperature: 213}}
	d, _ := serve(t, f)
	// What the bridge knows about a cached light that didn't answer at startup.
	d.Name, d.Info = "arvi-s-keylight", &elgato.AccessoryInfo{SerialNumber: keyLight.SerialNumber}
	l := newLight(d, 2)
	if got := l.acc.Info.Name.Value(); got != "arvi s keylight" {
		t.Fatalf("name before the light answers = %q, want the config name", got)
	}

	if err := l.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	info := l.acc.Info
	if info.Name.Value() != "Arvi's KeyLight" || info.Model.Value() != "Elgato Key Light Air" || info.FirmwareRevision.Value() != "1.0.3" {
		t.Errorf("after the light answers: name %q, model %q, firmware %q; want Arvi's KeyLight, Elgato Key Light Air, 1.0.3",
			info.Name.Value(), info.Model.Value(), info.FirmwareRevision.Value())
	}
}

func TestHomeKitWritesReachDevice(t *testing.T) {
	f := &fakeLight{state: elgato.LightState{On: 0, Brightness: 40, Temperature: 213}}
	d, _ := serve(t, f)
	l := newLight(d, 2)
	if err := l.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	l.on.SetValueRequest(true, fromHome)
	l.brightness.SetValueRequest(75, fromHome)
	l.temperature.SetValueRequest(500, fromHome) // HomeKit's warmest; the light stops at 344
	l.identify()

	want := elgato.LightState{On: 1, Brightness: 75, Temperature: elgato.MaxMireds}
	got, identified := f.snapshot()
	if got != want {
		t.Errorf("light = %+v, want %+v", got, want)
	}
	if identified != 1 {
		t.Errorf("identify reached the light %d times, want 1", identified)
	}
}

func TestUnreachableLightShowsNoResponse(t *testing.T) {
	f := &fakeLight{state: elgato.LightState{On: 1, Brightness: 40, Temperature: 213}}
	d, srv := serve(t, f)
	l := newLight(d, 2)
	if err := l.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, code := l.on.ValueRequest(fromHome); code != 0 {
		t.Fatalf("read while reachable: status %d, want 0", code)
	}

	srv.Close()
	if err := l.refresh(context.Background()); err == nil {
		t.Fatal("refresh of a stopped light succeeded")
	}
	if _, code := l.on.ValueRequest(fromHome); code != hap.JsonStatusServiceCommunicationFailure {
		t.Errorf("read while unreachable: status %d, want %d", code, hap.JsonStatusServiceCommunicationFailure)
	}
	if _, code := l.brightness.SetValueRequest(10, fromHome); code != hap.JsonStatusServiceCommunicationFailure {
		t.Errorf("write while unreachable: status %d, want %d", code, hap.JsonStatusServiceCommunicationFailure)
	}
}
