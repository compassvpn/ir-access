package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/carlmjohnson/versioninfo"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"

	"prefix-fetcher/fetch"
)

const appName = "prefix-fetcher"

var version string

func main() {
	fs := ff.NewFlagSet(appName)
	fetchIR := fs.BoolLong("fetch-ir", "Fetch Iranian IP prefixes from bgp.tools")
	fetchCN := fs.BoolLong("fetch-cn", "Fetch Chinese IP prefixes from bgp.tools")
	fetchRU := fs.BoolLong("fetch-ru", "Fetch Russian IP prefixes from bgp.tools")
	verbose := fs.Bool('v', "verbose", "Enable verbose logging")
	showVersion := fs.BoolLong("version", "Show version information")

	if err := ff.Parse(fs, os.Args[1:]); err != nil {
		if errors.Is(err, ff.ErrHelp) {
			fmt.Fprintf(os.Stderr, "%s\n", ffhelp.Flags(fs))
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *showVersion {
		if version == "" {
			version = versioninfo.Short()
		}
		fmt.Fprintf(os.Stderr, "%s\n", version)
		os.Exit(0)
	}

	logger := createLogger(*verbose)

	switch {
	case *fetchIR:
		fetch.FetchIR(logger)
	case *fetchCN:
		fetch.FetchCN(logger)
	case *fetchRU:
		fetch.FetchRU(logger)
	default:
		fmt.Fprintf(os.Stderr, "error: specify --fetch-ir, --fetch-cn, or --fetch-ru\n")
		fmt.Fprintf(os.Stderr, "%s\n", ffhelp.Flags(fs))
		os.Exit(1)
	}
}

func createLogger(verbose bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if verbose {
		opts.Level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
