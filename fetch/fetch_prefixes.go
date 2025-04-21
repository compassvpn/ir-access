package fetch

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"sync"
	"time"
)

const (
	Url          = "https://bgp.tools/table.jsonl"
	UserAgent    = "irgfw bgp.tools - contact@irgfw.report"
	OutputFileV4 = "ir_prefixes_v4.txt"
	OutputFileV6 = "ir_prefixes_v6.txt"
	Retries      = 4
)

// Prefix structure for JSON parsing
type Prefix struct {
	CIDR netip.Prefix `json:"CIDR"`
	ASN  int          `json:"ASN"`
}

// Fetch prefixes with retry logic
func fetchPrefixesWithRetry(l *slog.Logger, url string, client *http.Client, retries int) ([]Prefix, error) {
	var prefixes []Prefix
	var err error
	for i := 0; i < retries; i++ {
		prefixes, err = fetchPrefixes(l, url, client)
		if err == nil {
			return prefixes, nil
		}
		l.Warn("fetch failed", "attempt", i+1, "max", retries, "error", err)
		time.Sleep(2 * time.Second) // Backoff between retries
	}
	return nil, fmt.Errorf("all fetch attempts failed: %w", err)
}

// Fetches prefixes from the URL
func fetchPrefixes(l *slog.Logger, url string, client *http.Client) ([]Prefix, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	var prefixes []Prefix
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var prefix Prefix
		if err := json.Unmarshal(scanner.Bytes(), &prefix); err != nil {
			l.Warn("skipping invalid JSON line", "error", err)
			continue
		}
		prefixes = append(prefixes, prefix)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return prefixes, nil
}

// Filters prefixes by ASN
func filterPrefixesByASN(prefixes []Prefix, asns []int) ([]netip.Prefix, []netip.Prefix) {
	asnSet := make(map[int]struct{})
	for _, asn := range asns {
		asnSet[asn] = struct{}{}
	}

	var v4Prefixes, v6Prefixes []netip.Prefix
	for _, prefix := range prefixes {
		if _, exists := asnSet[prefix.ASN]; exists {
			if prefix.CIDR.Addr().Is4() {
				v4Prefixes = append(v4Prefixes, prefix.CIDR)
			} else if prefix.CIDR.Addr().Is6() {
				v6Prefixes = append(v6Prefixes, prefix.CIDR)
			}
		}
	}

	return v4Prefixes, v6Prefixes
}

// Converts IPv4 prefixes to /24 blocks and writes them to a channel
func processPrefixTo24(prefix netip.Prefix) []netip.Prefix {
	ip := prefix.Addr()
	prefixLen := prefix.Bits()

	if prefixLen == 24 {
		return []netip.Prefix{prefix}
	}

	ipInt := ipToInt(ip)
	numBlocks := 1 << (24 - prefixLen) // Calculate the number of /24 blocks
	splitPrefixes := make([]netip.Prefix, numBlocks)
	for i := 0; i < numBlocks; i++ {
		ip := intToIP(ipInt)
		splitPrefixes[i] = netip.PrefixFrom(ip, 24)
		incrementIPBy24(ipInt)
	}

	return splitPrefixes
}

func ipToInt(ip netip.Addr) *big.Int {
	ipInt := big.NewInt(0)
	ipInt.SetBytes(ip.AsSlice())
	return ipInt
}

func intToIP(ipInt *big.Int) netip.Addr {
	ipBytes := make([]byte, 4)
	ipInt.FillBytes(ipBytes)
	ip, _ := netip.AddrFromSlice(ipBytes)
	return ip
}

func incrementIPBy24(ipInt *big.Int) {
	increment := big.NewInt(1 << 8) // 256 for /24
	ipInt.Add(ipInt, increment)
}

// https://cs.opensource.google/go/go/+/refs/tags/go1.23.5:src/net/netip/netip.go;l=1312
func prefixCompare(a, b netip.Prefix) int {
	if c := cmp.Compare(a.Addr().BitLen(), b.Addr().BitLen()); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Bits(), b.Bits()); c != 0 {
		return c
	}
	return a.Addr().Compare(b.Addr())
}

// Writes sorted prefixes to a file
func writePrefixesToFileV4(l *slog.Logger, prefixes []netip.Prefix, outputFile string) error {
	if len(prefixes) == 0 {
		l.Info("no prefixes to write", "family", "v4")
		return nil
	}

	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer outFile.Close()

	writer := bufio.NewWriter(outFile)
	defer writer.Flush()

	uniquePrefixes := make(map[netip.Prefix]struct{})
	for _, prefix := range prefixes {
		split := processPrefixTo24(prefix)
		for _, p := range split {
			uniquePrefixes[p] = struct{}{}
		}
	}

	sortedPrefixes := make([]netip.Prefix, 0, len(uniquePrefixes))
	for prefix := range uniquePrefixes {
		sortedPrefixes = append(sortedPrefixes, prefix)
	}

	slices.SortFunc(sortedPrefixes, prefixCompare)
	for _, prefix := range sortedPrefixes {
		if _, err := writer.WriteString(prefix.String() + "\n"); err != nil {
			return fmt.Errorf("writing to file: %w", err)
		}
	}

	l.Info("wrote IPv4 /24 prefixes to file", "count", len(sortedPrefixes), "file", outputFile)
	return nil
}

// Writes IPv6 prefixes to a file without modification
func writePrefixesToFileV6(l *slog.Logger, prefixes []netip.Prefix, outputFile string) error {
	if len(prefixes) == 0 {
		l.Info("no prefixes to write", "family", "v6")
		return nil
	}

	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer outFile.Close()

	writer := bufio.NewWriter(outFile)
	defer writer.Flush()

	sortedPrefixes := make([]netip.Prefix, len(prefixes))
	copy(sortedPrefixes, prefixes)
	slices.SortFunc(sortedPrefixes, prefixCompare)

	for _, prefix := range sortedPrefixes {
		if _, err := writer.WriteString(prefix.String() + "\n"); err != nil {
			return fmt.Errorf("writing to file: %w", err)
		}
	}

	l.Info("wrote IPv6 prefixes to file", "count", len(sortedPrefixes), "file", outputFile)
	return nil
}

// StartFetchPrefixes is the main entry point for fetching and processing prefixes.
func StartFetchPrefixes(l *slog.Logger) {
	l.Info("starting processing for ASNs", "asns", AsnsToFilter())

	client := &http.Client{}
	prefixes, err := fetchPrefixesWithRetry(l, Url, client, Retries)
	if err != nil {
		l.Error("fetching prefixes failed", "error", err)
		return
	}

	v4Prefixes, v6Prefixes := filterPrefixesByASN(prefixes, AsnsToFilter())

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := writePrefixesToFileV4(l, v4Prefixes, OutputFileV4); err != nil {
			l.Error("writing IPv4 prefixes", "error", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := writePrefixesToFileV6(l, v6Prefixes, OutputFileV6); err != nil {
			l.Error("writing IPv6 prefixes", "error", err)
		}
	}()

	wg.Wait()
	l.Info("prefix fetching and writing complete")
}
