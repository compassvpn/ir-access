package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"

	"github.com/carlmjohnson/versioninfo"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"

	"ir-access/fetch"    // Import fetch package
	"ir-access/nftables" // Import nftables package
)

const appName = "ir-access"

var version string = ""

// Execute fetch operation
// func fetch(l *slog.Logger) {
// 	l.Info("fetching IP prefixes")
// 	startFetchPrefixes(l)
// }

// Execute setup operation
func setup(l *slog.Logger) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("setup only works on linux")
	}

	_, err := exec.LookPath("nft")
	if err != nil {
		return fmt.Errorf("nft command not found")
	}

	_, err = exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemctl command not found")
	}

	l.Info("running fetch operation before setup...")
	fetch.StartFetchPrefixes(l)          // Use fetch package
	err = nftables.StartSetupNftables(l) // Use nftables package
	if err != nil {
		return err // Return the error from StartSetupNftables
	}

	return nil
}

func main() {
	// Define command-line flags
	fs := ff.NewFlagSet(appName)
	fetchFlag := fs.Bool('f', "fetch", "Fetch all Iranian IP prefixes from bgp.tools.")
	setupFlag := fs.Bool('s', "setup", "Set up nftables rules to Iran-Access-Only except SSH port.")
	verboseFlag := fs.Bool('v', "verbose", "Enable verbose logging")
	versionFlag := fs.BoolLong("version", "displays version number")

	err := ff.Parse(fs, os.Args[1:])
	switch {
	case errors.Is(err, ff.ErrHelp):
		fmt.Fprintf(os.Stderr, "%s\n", ffhelp.Flags(fs))
		os.Exit(0)
	case err != nil:
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *versionFlag {
		if version == "" {
			version = versioninfo.Short()
		}
		fmt.Fprintf(os.Stderr, "%s\n", version)
		os.Exit(0)
	}

	l := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if *verboseFlag {
		l = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	// Execute operations based on flags
	switch {
	case *setupFlag:
		if err := setup(l); err != nil {
			fatal(l, err)
		}
	case *fetchFlag:
		// fetch(l) // Old call
		fetch.StartFetchPrefixes(l) // Use fetch package directly
	default:
		fmt.Fprintf(os.Stderr, "error: invalid or missing options\n")
		fmt.Fprintf(os.Stderr, "%s\n", ffhelp.Flags(fs))
		os.Exit(1)
	}
}

func fatal(l *slog.Logger, err error) {
	l.Error(err.Error())
	os.Exit(1)
}
