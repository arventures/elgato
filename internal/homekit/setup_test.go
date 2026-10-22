package homekit

import (
	"testing"

	"github.com/brutella/hap"
)

func TestSetupCode(t *testing.T) {
	store := hap.NewMemStore()

	code, err := SetupCode(store, "")
	if err != nil {
		t.Fatalf("SetupCode: %v", err)
	}
	if len(code) != 8 || hap.InvalidPins[code] {
		t.Fatalf("generated code %q: want 8 digits HomeKit accepts", code)
	}
	if again, _ := SetupCode(store, ""); again != code {
		t.Errorf("second run got %q, want the saved %q", again, code)
	}

	// A chosen code replaces the saved one, dashes and all.
	if got, err := SetupCode(store, "314-15-926"); err != nil || got != "31415926" {
		t.Fatalf("SetupCode(314-15-926) = %q, %v; want 31415926", got, err)
	}
	if again, _ := SetupCode(store, ""); again != "31415926" {
		t.Errorf("after choosing a code, next run got %q, want 31415926", again)
	}

	for _, bad := range []string{"1234", "abcdefgh", "123456789", "12345678", "000-00-000"} {
		if _, err := SetupCode(hap.NewMemStore(), bad); err == nil {
			t.Errorf("SetupCode(%q) succeeded, want an error", bad)
		}
	}
}

func TestPaired(t *testing.T) {
	store := hap.NewMemStore()
	if Paired(store) {
		t.Fatal("a new bridge reports paired")
	}
	_ = store.Set("3a4b.pairing", []byte("{}")) // how hap saves each paired controller
	if !Paired(store) {
		t.Error("a bridge with a saved pairing reports unpaired")
	}
}

func TestFormatCode(t *testing.T) {
	if got := FormatCode("31415926"); got != "314-15-926" {
		t.Errorf("FormatCode = %q, want 314-15-926", got)
	}
}

func TestAccessoryIDsAreStable(t *testing.T) {
	store := hap.NewMemStore()

	ids, err := accessoryIDs(store, []string{"B", "A"})
	if err != nil {
		t.Fatal(err)
	}
	if ids["B"] != 2 || ids["A"] != 3 {
		t.Fatalf("first run = %v, want B:2 A:3 (1 is the bridge)", ids)
	}

	// A restart with B gone and a new light C keeps A's ID and never reuses B's.
	ids, err = accessoryIDs(store, []string{"C", "A"})
	if err != nil {
		t.Fatal(err)
	}
	if ids["A"] != 3 || ids["C"] != 4 || ids["B"] != 2 {
		t.Errorf("second run = %v, want A:3 C:4 and B still 2", ids)
	}
}

func TestAccessoryName(t *testing.T) {
	cases := map[string]string{
		"Sam's KeyLight":      "Sam's KeyLight",
		"Sam’s KeyLight":      "Sam's KeyLight", // curly apostrophe from iOS
		"sam-s-keylight":      "sam s keylight", // config slug
		" Key_Light (Left)! ": "Key Light Left",
		"'Desk'":              "Desk",
		"Café Lamp 2":         "Café Lamp 2",
		"!!!":                 "Key Light",
		"":                    "Key Light",
	}
	for in, want := range cases {
		if got := accessoryName(in); got != want {
			t.Errorf("accessoryName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRevision(t *testing.T) {
	cases := map[string]string{
		"1.0.3":                "1.0.3",
		"0.3.0-SNAPSHOT-abc12": "0.3.0",
		"v0.2.0":               "0.2.0",
		"1.2.3.4":              "1.2.3",
		"7":                    "7",
		"dev":                  "1.0.0",
		"":                     "1.0.0",
	}
	for in, want := range cases {
		if got := revision(in); got != want {
			t.Errorf("revision(%q) = %q, want %q", in, got, want)
		}
	}
}
