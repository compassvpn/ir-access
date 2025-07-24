package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"prefix-fetcher/fetch"
)

func main() {
	var (
		fetchIR  = flag.Bool("fetch-ir", false, "Fetch IP prefixes for Iran (IR)")
		fetchCN  = flag.Bool("fetch-cn", false, "Fetch IP prefixes for China (CN)")
		help     = flag.Bool("h", false, "Show help")
		helpLong = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help || *helpLong {
		showHelp()
		return
	}

	if !*fetchIR && !*fetchCN {
		fmt.Fprintf(os.Stderr, "Error: Please specify either --fetch-ir or --fetch-cn\n\n")
		showHelp()
		os.Exit(1)
	}

	if *fetchIR {
		if err := fetchAndSavePrefixes("IR"); err != nil {
			log.Fatalf("Failed to fetch Iran prefixes: %v", err)
		}
	}

	if *fetchCN {
		if err := fetchAndSavePrefixes("CN"); err != nil {
			log.Fatalf("Failed to fetch China prefixes: %v", err)
		}
	}
}

// fetchAndSavePrefixes fetches ASNs and prefixes for a country and saves to files
func fetchAndSavePrefixes(country string) error {
	fmt.Printf("Fetching prefixes for %s...\n", country)

	// Get ASNs from RIR
	asns, err := fetch.GetASNsForCountry(country)
	if err != nil {
		return fmt.Errorf("failed to get ASNs: %w", err)
	}

	fmt.Printf("Found %d ASNs for %s\n", len(asns), country)

	// Fetch BGP prefixes
	prefixes, err := fetch.GetPrefixesForASNs(asns)
	if err != nil {
		return fmt.Errorf("failed to get prefixes: %w", err)
	}

	fmt.Printf("Found %d IPv4 and %d IPv6 prefixes\n", len(prefixes.IPv4), len(prefixes.IPv6))

	// Save to files
	if err := fetch.SavePrefixesToFiles(country, prefixes); err != nil {
		return fmt.Errorf("failed to save prefixes: %w", err)
	}

	fmt.Printf("Prefixes saved successfully\n")
	return nil
}

func showHelp() {
	fmt.Println("prefix-fetcher - Fetch IP prefixes for countries")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ./prefix-fetcher --fetch-ir   Fetch prefixes for Iran")
	fmt.Println("  ./prefix-fetcher --fetch-cn   Fetch prefixes for China")
	fmt.Println("  ./prefix-fetcher -h, --help   Show this help")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ./prefix-fetcher --fetch-ir")
	fmt.Println("  ./prefix-fetcher --fetch-cn")
	fmt.Println("  ./prefix-fetcher --fetch-ir --fetch-cn")
	fmt.Println()
	fmt.Println("Output files:")
	fmt.Println("  ir_prefixes_v4.txt, ir_prefixes_v6.txt")
	fmt.Println("  cn_prefixes_v4.txt, cn_prefixes_v6.txt")
}
