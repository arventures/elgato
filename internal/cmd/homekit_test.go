package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDaemonPlistXML(t *testing.T) {
	plist := daemonPlistXML("/Users/a&b/go/bin/elgato", []string{"--light", "key & left", "homekit"},
		"sam", "/Users/a&b", "/Users/a&b/Library/Logs/elgato-homekit.log")
	for _, want := range []string{
		"<string>ventures.ar.elgato.homekit</string>",
		"<array>\n\t\t<string>/Users/a&amp;b/go/bin/elgato</string>\n\t\t<string>--light</string>\n" +
			"\t\t<string>key &amp; left</string>\n\t\t<string>homekit</string>\n\t</array>",
		"<key>UserName</key>\n\t<string>sam</string>",
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

func TestDaemonArgsKeepInstallFlags(t *testing.T) {
	saved := opts
	t.Cleanup(func() { opts = saved })

	opts.host, opts.port, opts.light = "", 9123, ""
	if got := daemonArgs(); !slices.Equal(got, []string{"homekit"}) {
		t.Errorf("no flags: daemonArgs() = %q, want [homekit]", got)
	}

	opts.host, opts.port, opts.light = "192.168.1.50", 9124, "key-left"
	want := []string{"--host", "192.168.1.50", "--port", "9124", "--light", "key-left", "homekit"}
	if got := daemonArgs(); !slices.Equal(got, want) {
		t.Errorf("with flags: daemonArgs() = %q, want %q", got, want)
	}
}
