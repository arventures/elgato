package homekit

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"unicode"

	"github.com/brutella/hap"
)

// Keys the bridge keeps in the hap store, next to hap's own pairing data.
const (
	codeKey = "setup-code"
	idsKey  = "accessory-ids"
)

// SetupCode returns the 8-digit code for pairing the bridge in the Home app:
// code if given, else the one saved by an earlier run, else a new random one.
// It saves the result, so the code stays the same across restarts.
func SetupCode(store hap.Store, code string) (string, error) {
	code = strings.ReplaceAll(code, "-", "")
	if code == "" {
		if b, err := store.Get(codeKey); err == nil {
			code = string(b)
		}
	}
	if code == "" {
		var err error
		if code, err = randomCode(); err != nil {
			return "", err
		}
	}
	if len(code) != 8 || strings.Trim(code, "0123456789") != "" {
		return "", fmt.Errorf("invalid setup code %q: want 8 digits, like 123-45-678", code)
	}
	if hap.InvalidPins[code] {
		return "", fmt.Errorf("setup code %s is too easy to guess, so HomeKit won't accept it", FormatCode(code))
	}
	return code, store.Set(codeKey, []byte(code))
}

// FormatCode formats a setup code the way the Home app shows it: 123-45-678.
func FormatCode(code string) string {
	if len(code) != 8 {
		return code
	}
	return code[:3] + "-" + code[3:5] + "-" + code[5:]
}

func randomCode() (string, error) {
	for {
		n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
		if err != nil {
			return "", err
		}
		if code := fmt.Sprintf("%08d", n); !hap.InvalidPins[code] {
			return code, nil
		}
	}
}

// accessoryIDs gives each serial number a HomeKit accessory ID that never
// changes, so the Home app keeps a light's room, name and automations across
// restarts and as other lights come and go. ID 1 is the bridge itself.
func accessoryIDs(store hap.Store, serials []string) (map[string]uint64, error) {
	ids := map[string]uint64{}
	if b, err := store.Get(idsKey); err == nil {
		if err := json.Unmarshal(b, &ids); err != nil {
			return nil, fmt.Errorf("reading saved accessory IDs: %w", err)
		}
	}

	next := uint64(2)
	for _, id := range ids {
		next = max(next, id+1)
	}
	changed := false
	for _, s := range serials {
		if _, ok := ids[s]; !ok {
			ids[s] = next
			next++
			changed = true
		}
	}
	if changed {
		b, err := json.Marshal(ids)
		if err != nil {
			return nil, err
		}
		if err := store.Set(idsKey, b); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// accessoryName adapts s to HomeKit's naming rules: only letters, numbers,
// spaces and apostrophes, starting and ending with a letter or number.
func accessoryName(s string) string {
	s = strings.NewReplacer("’", "'", "‘", "'").Replace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' {
			return r
		}
		return ' '
	}, s)
	s = strings.TrimFunc(strings.Join(strings.Fields(s), " "), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if s == "" {
		return "Key Light"
	}
	return s
}

var revisionPattern = regexp.MustCompile(`^\d+(\.\d+){0,2}`)

// revision adapts a version to HomeKit's firmware format, x[.y[.z]]: "0.3.0"
// stays as is, "0.3.0-rc1" becomes "0.3.0", and a dev build becomes "1.0.0".
func revision(version string) string {
	if r := revisionPattern.FindString(strings.TrimPrefix(version, "v")); r != "" {
		return r
	}
	return "1.0.0"
}
