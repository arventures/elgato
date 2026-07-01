package elgato

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// DefaultPort is the port the Elgato REST API listens on.
const DefaultPort = 9123

// Device is a reachable Elgato light on the network.
type Device struct {
	Name string // friendly / display name
	Host string // IP address or hostname
	Port int
	Info *AccessoryInfo // populated by FetchInfo
}

// BaseURL returns the device's API root, e.g. http://10.0.0.11:9123.
func (d *Device) BaseURL() string {
	port := d.Port
	if port == 0 {
		port = DefaultPort
	}
	return "http://" + net.JoinHostPort(d.Host, strconv.Itoa(port))
}

// Label returns the best human-readable name for the device.
func (d *Device) Label() string {
	if d.Info != nil && d.Info.DisplayName != "" {
		return d.Info.DisplayName
	}
	if d.Name != "" {
		return d.Name
	}
	return d.Host
}

// Serial returns the device's serial number if known.
func (d *Device) Serial() string {
	if d.Info != nil {
		return d.Info.SerialNumber
	}
	return ""
}

// GetState fetches the current light state.
func (d *Device) GetState(ctx context.Context) (*LightsPayload, error) {
	var out LightsPayload
	if err := d.doJSON(ctx, http.MethodGet, "/elgato/lights", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetState writes a new light state and returns the device's response.
func (d *Device) SetState(ctx context.Context, p *LightsPayload) (*LightsPayload, error) {
	var out LightsPayload
	if err := d.doJSON(ctx, http.MethodPut, "/elgato/lights", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FetchInfo fetches accessory info and caches it on the device.
func (d *Device) FetchInfo(ctx context.Context) (*AccessoryInfo, error) {
	var info AccessoryInfo
	if err := d.doJSON(ctx, http.MethodGet, "/elgato/accessory-info", nil, &info); err != nil {
		return nil, err
	}
	d.Info = &info
	return &info, nil
}

// Apply reads the current state, mutates every light with fn, and writes it
// back. It is used by on/off/toggle/brightness/temperature so relative changes
// are computed against each light's real current value.
func (d *Device) Apply(ctx context.Context, fn func(*LightState)) (*LightsPayload, error) {
	state, err := d.GetState(ctx)
	if err != nil {
		return nil, err
	}
	for i := range state.Lights {
		fn(&state.Lights[i])
	}
	return d.SetState(ctx, state)
}

func (d *Device) doJSON(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.BaseURL()+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: unexpected status %s", method, path, resp.Status)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response from %s: %w", d.Host, err)
		}
	}
	return nil
}
