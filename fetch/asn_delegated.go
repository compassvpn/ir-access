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

// RIR represents a Regional Internet Registry
type RIR struct {
	Name string
	URL  string
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

// Country to RIR mapping based on geographical coverage
var CountryToRIR = map[string]RIR{
	"IR": RIPE_NCC, // Iran - Middle East (RIPE NCC coverage)
	"CN": APNIC,    // China - Asia-Pacific (APNIC coverage)
}

// DelegatedRecord represents a single record from RIR delegated file
type DelegatedRecord struct {
	Registry string // ripe, apnic, arin, etc.
	CC       string // country code (IR, CN)
	Type     string // asn, ipv4, ipv6
	Start    string // starting number/address
	Value    string // count/size
	Date     string // allocation date (YYYYMMDD)
	Status   string // allocated, assigned, etc.
}

// MultiRIRASNFetcher handles fetching ASNs from multiple RIR delegated files
type MultiRIRASNFetcher struct {
	httpClient *http.Client
}

// NewMultiRIRASNFetcher creates a new multi-RIR ASN fetcher
func NewMultiRIRASNFetcher() *MultiRIRASNFetcher {
	return &MultiRIRASNFetcher{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// FetchASNsForCountry fetches ASN allocations for a specific country
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

// FetchIRAndCNASNs fetches ASN allocations for both Iran and China
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

// fetchDelegatedRecords downloads and parses delegated records from RIR
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

// parseDelegatedFile parses the RIR delegated file format
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

		// Parse delegated record
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

// parseDelegatedRecord parses a single delegated record line
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

// extractASNsForCountry extracts ASN numbers for a specific country from delegated records
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

		// Parse starting ASN
		startASN, err := strconv.Atoi(record.Start)
		if err != nil {
			continue
		}

		// Parse count of ASNs
		count, err := strconv.Atoi(record.Value)
		if err != nil {
			continue
		}

		// Add all ASNs in the range
		for i := 0; i < count; i++ {
			asn := startASN + i
			// Filter out private ASNs and invalid ranges
			if f.isValidPublicASN(asn) {
				asns = append(asns, asn)
			}
		}
	}

	return asns
}

// isValidPublicASN checks if an ASN is a valid public ASN
func (f *MultiRIRASNFetcher) isValidPublicASN(asn int) bool {
	// Filter out reserved and private ASN ranges
	// See: https://www.iana.org/assignments/as-numbers/as-numbers.xhtml

	// Reserved ranges to exclude:
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

// GetRIRForCountry returns the RIR responsible for a country
func GetRIRForCountry(countryCode string) (RIR, bool) {
	rir, exists := CountryToRIR[countryCode]
	return rir, exists
}

// GetSupportedCountries returns the list of supported country codes
func GetSupportedCountries() []string {
	countries := make([]string, 0, len(CountryToRIR))
	for cc := range CountryToRIR {
		countries = append(countries, cc)
	}
	sort.Strings(countries)
	return countries
}
