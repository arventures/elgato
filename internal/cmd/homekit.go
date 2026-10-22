package cmd

import (
	"context"
	"errors"
	"fmt"
	"html"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/arventures/elgato/internal/config"
	"github.com/arventures/elgato/internal/elgato"
	"github.com/arventures/elgato/internal/homekit"
	"github.com/brutella/hap"
	"github.com/spf13/cobra"
)

// The launchd daemon that runs the bridge in the background on macOS.
const (
	daemonLabel = "ventures.ar.elgato.homekit"
	daemonPlist = "/Library/LaunchDaemons/" + daemonLabel + ".plist"
)

func newHomeKitCmd(version string) *cobra.Command {
	var (
		code   string
		listen string
		poll   time.Duration
	)
	cmd := &cobra.Command{
		Use:   "homekit",
		Short: "Add your lights to the Apple Home app (runs a HomeKit bridge)",
		Long: "homekit runs a HomeKit bridge that puts your lights in the Apple Home app as\n" +
			"dimmable bulbs with color temperature, for Siri, scenes and automations.\n\n" +
			"It runs until stopped. To keep it running in the background, starting at\n" +
			"boot, use `elgato homekit install` (macOS).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			err := runHomeKit(ctx, version, code, listen, poll)
			if ctx.Err() != nil {
				return nil // stopped with Ctrl-C or by launchd
			}
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&code, "code", "", "8-digit setup code for pairing (default: random, saved on first run)")
	f.StringVar(&listen, "listen", ":51827", "address the HomeKit server listens on")
	f.DurationVar(&poll, "poll", 3*time.Second, "how often to sync each light's state into HomeKit")

	cmd.AddCommand(newHomeKitInstallCmd(), newHomeKitUninstallCmd())
	return cmd
}

func runHomeKit(ctx context.Context, version, code, listen string, poll time.Duration) error {
	if err := homekit.WaitForNetwork(ctx); err != nil {
		return err
	}
	devices, err := homekitDevices(ctx)
	if err != nil {
		return err
	}
	store := hap.NewFsStore(homekitDir())
	code, err = homekit.SetupCode(store, code)
	if err != nil {
		return err
	}
	return homekit.Run(ctx, store, devices, homekit.Options{
		Name:    "Elgato Bridge",
		Version: version,
		Code:    code,
		Addr:    listen,
		Poll:    poll,
	})
}

// homekitDir holds the bridge's pairing keys, setup code and accessory IDs,
// next to the config file: ~/.config/elgato/homekit.
func homekitDir() string {
	return filepath.Join(filepath.Dir(config.Path()), "homekit")
}

// homekitDevices picks the lights to put in HomeKit, honoring --host and
// --light. Unlike resolveDevices it keeps cached lights that aren't answering
// right now: a light that's unplugged when the bridge starts shows "No
// Response" in the Home app instead of vanishing from it, along with its room
// and automations.
func homekitDevices(ctx context.Context) ([]*elgato.Device, error) {
	if opts.host != "" {
		d := &elgato.Device{Host: opts.host, Port: opts.port}
		infoCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		// The serial number is the light's identity in HomeKit, so it's required.
		if _, err := d.FetchInfo(infoCtx); err != nil {
			return nil, fmt.Errorf("%s: %w", opts.host, err)
		}
		return []*elgato.Device{d}, nil
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	found, err := discover(ctx, cfg)
	if err != nil {
		return nil, err
	}
	online := make(map[string]*elgato.Device, len(found))
	for _, d := range found {
		online[d.Serial()] = d
	}

	var devices []*elgato.Device
	seen := make(map[string]bool)
	for _, name := range slices.Sorted(maps.Keys(cfg.Lights)) {
		l := cfg.Lights[name]
		if l.Serial == "" || seen[l.Serial] {
			continue
		}
		d := online[l.Serial]
		if d == nil {
			// Discovery missed it: use the cached address, taking the light's
			// name and model from it if it answers.
			d = &elgato.Device{Name: name, Host: l.Host, Port: l.Port}
			infoCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, _ = d.FetchInfo(infoCtx)
			cancel()
			if d.Serial() != l.Serial {
				d.Info = &elgato.AccessoryInfo{SerialNumber: l.Serial}
			}
		}
		if opts.light != "" && !nameMatches(name, l.Serial, d.Host, opts.light) && !strings.EqualFold(d.Label(), opts.light) {
			continue
		}
		seen[l.Serial] = true
		devices = append(devices, d)
	}
	if len(devices) == 0 {
		return nil, errNoLights
	}
	return devices, nil
}

func newHomeKitInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Run the bridge in the background, starting at boot (macOS)",
		Long: "install sets the bridge up as a launchd daemon that runs as you, so it needs\n" +
			"your password once. It's a daemon rather than a login item on purpose: macOS's\n" +
			"Local Network privacy blocks background agents from reaching your lights, while\n" +
			"daemons are allowed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "darwin" {
				return errors.New("install is for macOS; elsewhere, run `elgato homekit` under your service manager")
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			u, err := user.Current()
			if err != nil {
				return err
			}
			logPath := filepath.Join(u.HomeDir, "Library", "Logs", "elgato-homekit.log")
			// Create the log as you, before launchd opens it as root.
			if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				f.Close()
			}

			tmp, err := os.CreateTemp("", daemonLabel+"-*.plist")
			if err != nil {
				return err
			}
			defer os.Remove(tmp.Name())
			if _, err := tmp.WriteString(daemonPlistXML(exe, u.Username, u.HomeDir, logPath)); err != nil {
				return err
			}
			if err := tmp.Close(); err != nil {
				return err
			}

			fmt.Fprintln(stdout, "Adding a launchd daemon needs admin rights, so sudo may ask for your password.")
			if err := sudo("install", "-m", "644", "-o", "root", "-g", "wheel", tmp.Name(), daemonPlist); err != nil {
				return err
			}
			bootout() // replace a running copy from an earlier install
			if err := sudo("launchctl", "bootstrap", "system", daemonPlist); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Installed. The bridge is running and will start at every boot.\n")
			fmt.Fprintf(stdout, "Its log, including the setup code for the Home app: %s\n", logPath)
			return nil
		},
	}
}

func newHomeKitUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the background bridge and remove it (macOS)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "darwin" {
				return errors.New("uninstall is for macOS")
			}
			fmt.Fprintln(stdout, "Removing the launchd daemon needs admin rights, so sudo may ask for your password.")
			bootout()
			if err := sudo("rm", "-f", daemonPlist); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Uninstalled. Pairing data stays in %s; delete it to start over.\n", homekitDir())
			return nil
		},
	}
}

// daemonPlistXML renders the launchd job that runs `elgato homekit` as user.
// HOME is set explicitly so the bridge finds ~/.config/elgato.
func daemonPlistXML(exe, username, home, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>homekit</string>
	</array>
	<key>UserName</key>
	<string>%s</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>HOME</key>
		<string>%s</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%[5]s</string>
	<key>StandardErrorPath</key>
	<string>%[5]s</string>
</dict>
</plist>
`, html.EscapeString(daemonLabel), html.EscapeString(exe), html.EscapeString(username), html.EscapeString(home), html.EscapeString(logPath))
}

// sudo runs an admin command, letting sudo prompt for your password.
func sudo(args ...string) error {
	c := exec.Command("sudo", args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("sudo %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// bootout stops the daemon if it's loaded. It fails harmlessly when it isn't,
// so its output is dropped.
func bootout() {
	c := exec.Command("sudo", "launchctl", "bootout", "system/"+daemonLabel)
	c.Stdin = os.Stdin
	_ = c.Run()
}
