package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDaemonPlistXML(t *testing.T) {
	plist := daemonPlistXML("/Users/a&b/go/bin/elgato", "arvi", "/Users/a&b", "/Users/a&b/Library/Logs/elgato-homekit.log")
	for _, want := range []string{
		"<string>ventures.ar.elgato.homekit</string>",
		"<string>/Users/a&amp;b/go/bin/elgato</string>\n\t\t<string>homekit</string>",
		"<key>UserName</key>\n\t<string>arvi</string>",
		"<key>HOME</key>\n\t\t<string>/Users/a&amp;b</string>",
		"<key>StandardErrorPath</key>\n\t<string>/Users/a&amp;b/Library/Logs/elgato-homekit.log</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist is missing %q:\n%s", want, plist)
		}
	}

	plutil, err := exec.LookPath("plutil")
	if err != nil {
		t.Skip("plutil not available to validate the plist")
	}
	path := filepath.Join(t.TempDir(), "job.plist")
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(plutil, "-lint", path).CombinedOutput(); err != nil {
		t.Errorf("plutil -lint: %v\n%s", err, out)
	}
}
