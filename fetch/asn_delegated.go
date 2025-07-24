package fetch

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Registry endpoint with its official delegated file URL.
type RIR struct {
	Name string
	URL  string
}

// Parsed entry from an RIR's pipe-delimited allocation file.
type DelegatedRecord struct {
	Registry string // RIR name
	CC       string // Country code
	Type     string // Record type (asn, ipv4, ipv6)
	Start    string // Start value
	Value    string // Count or size
	Date     string // Allocation date
	Status   string // Allocation status
}

// Downloads and parses ASN data from multiple RIR sources.
type MultiRIRASNFetcher struct {
	httpClient *http.Client
}

// Well-known RIR delegated file URLs
var (
	RIPE_NCC = RIR{
		Name: "RIPE NCC",
		URL:  "https://ftp.ripe.net/ripe/stats/delegated-ripencc-latest",
	}
	APNIC = RIR{
		Name: "APNIC",
		URL:  "https://ftp.apnic.net/stats/apnic/delegated-apnic-latest",
	}
	ARIN = RIR{
		Name: "ARIN",
		URL:  "https://ftp.arin.net/pub/stats/arin/delegated-arin-extended-latest",
	}
	LACNIC = RIR{
		Name: "LACNIC",
		URL:  "https://ftp.lacnic.net/pub/stats/lacnic/delegated-lacnic-latest",
	}
	AFRINIC = RIR{
		Name: "AFRINIC",
		URL:  "https://ftp.afrinic.net/pub/stats/afrinic/delegated-afrinic-latest",
	}
)

// Maps countries to their authoritative RIR based on geographical coverage.
var CountryToRIR = map[string]RIR{
	"IR": RIPE_NCC, // Iran - Middle East (RIPE NCC coverage)
	"CN": APNIC,    // China - Asia-Pacific (APNIC coverage)
}

func NewMultiRIRASNFetcher() *MultiRIRASNFetcher {
	return &MultiRIRASNFetcher{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (f *MultiRIRASNFetcher) FetchASNsForCountry(countryCode string) ([]int, error) {
	rir, exists := CountryToRIR[countryCode]
	if !exists {
		return nil, fmt.Errorf("no RIR mapping found for country code: %s", countryCode)
	}

	fmt.Printf("Fetching ASNs for %s from %s (%s)\n", countryCode, rir.Name, rir.URL)

	records, err := f.fetchDelegatedRecords(rir.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch delegated records from %s: %w", rir.Name, err)
	}

	asns := f.extractASNsForCountry(records, countryCode)
	sort.Ints(asns)

	fmt.Printf("Found %d ASNs for %s from %s\n", len(asns), countryCode, rir.Name)
	return asns, nil
}

// Downloads ASN data for both Iran and China in parallel.
func (f *MultiRIRASNFetcher) FetchIRAndCNASNs() (map[string][]int, error) {
	result := make(map[string][]int)

	countries := []string{"IR", "CN"}

	for _, country := range countries {
		asns, err := f.FetchASNsForCountry(country)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch ASNs for %s: %w", country, err)
		}
		result[country] = asns
	}

	return result, nil
}

func (f *MultiRIRASNFetcher) fetchDelegatedRecords(url string) ([]DelegatedRecord, error) {
	resp, err := f.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}

	return f.parseDelegatedFile(resp.Body)
}

// Parses the standard RIR delegated file format (pipe-delimited).
func (f *MultiRIRASNFetcher) parseDelegatedFile(reader io.Reader) ([]DelegatedRecord, error) {
	var records []DelegatedRecord
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip version and summary lines
		if strings.Contains(line, "|version|") || strings.Contains(line, "|summary|") {
			continue
		}

		record, err := f.parseDelegatedRecord(line)
		if err != nil {
			// Skip malformed lines rather than failing completely
			continue
		}

		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading delegated file: %w", err)
	}

	return records, nil
}

func (f *MultiRIRASNFetcher) parseDelegatedRecord(line string) (DelegatedRecord, error) {
	parts := strings.Split(line, "|")
	if len(parts) < 7 {
		return DelegatedRecord{}, fmt.Errorf("invalid record format: %s", line)
	}

	return DelegatedRecord{
		Registry: parts[0],
		CC:       parts[1],
		Type:     parts[2],
		Start:    parts[3],
		Value:    parts[4],
		Date:     parts[5],
		Status:   parts[6],
	}, nil
}

// Extracts valid public ASNs from delegated records, expanding ranges.
func (f *MultiRIRASNFetcher) extractASNsForCountry(records []DelegatedRecord, countryCode string) []int {
	var asns []int

	for _, record := range records {
		// Only process ASN records for the specified country
		if record.Type != "asn" || record.CC != countryCode {
			continue
		}

		// Skip if status indicates it's not a proper allocation
		if record.Status == "reserved" || record.Status == "available" {
			continue
		}

		startASN, err := strconv.Atoi(record.Start)
		if err != nil {
			continue
		}

		count, err := strconv.Atoi(record.Value)
		if err != nil {
			continue
		}

		// Expand ASN ranges and filter out private/reserved numbers
		for i := 0; i < count; i++ {
			asn := startASN + i
			if f.isValidPublicASN(asn) {
				asns = append(asns, asn)
			}
		}
	}

	return asns
}

// Validates ASN against IANA reservations and private ranges.
func (f *MultiRIRASNFetcher) isValidPublicASN(asn int) bool {
	// Filter out reserved and private ASN ranges
	// See: https://www.iana.org/assignments/as-numbers/as-numbers.xhtml

	if asn == 0 {
		return false // Reserved
	}
	if asn >= 64512 && asn <= 65534 {
		return false // Private Use 16-bit
	}
	if asn == 65535 {
		return false // Reserved
	}
	if asn >= 4200000000 && asn <= 4294967294 {
		return false // Private Use 32-bit
	}
	if asn == 4294967295 {
		return false // Reserved
	}

	// Valid public ASN ranges:
	// 1-64511 (16-bit public)
	// 131072-4199999999 (32-bit public, excluding private ranges)

	if asn >= 1 && asn <= 64511 {
		return true
	}
	if asn >= 131072 && asn <= 4199999999 {
		return true
	}

	return false
}

func GetRIRForCountry(countryCode string) (RIR, bool) {
	rir, exists := CountryToRIR[countryCode]
	return rir, exists
}

func GetSupportedCountries() []string {
	countries := make([]string, 0, len(CountryToRIR))
	for cc := range CountryToRIR {
		countries = append(countries, cc)
	}
	sort.Strings(countries)
	return countries
}
