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
	bgpToolsURL = "https://bgp.tools/table.jsonl"
	userAgent   = "compassvpn-prefix-fetcher bgp.tools"
	maxRetries  = 4
	retryDelay  = 1 * time.Second
)

// Config holds country-specific fetch settings.
type Config struct {
	Country      string
	ASNFunc      func() []int
	OutputFileV4 string
	OutputFileV6 string
}

// Prefix represents a BGP route with its ASN.
type Prefix struct {
	CIDR netip.Prefix `json:"CIDR"`
	ASN  int          `json:"ASN"`
}

// Gets Iranian IP prefixes.
func FetchIR(l *slog.Logger) {
	config := Config{
		Country:      "IR",
		ASNFunc:      IRASN,
		OutputFileV4: "ir_prefixes_v4.txt",
		OutputFileV6: "ir_prefixes_v6.txt",
	}
	fetchPrefixesForCountry(l, config)
}

// Gets Chinese IP prefixes.
func FetchCN(l *slog.Logger) {
	config := Config{
		Country:      "CN",
		ASNFunc:      CNASN,
		OutputFileV4: "cn_prefixes_v4.txt",
		OutputFileV6: "cn_prefixes_v6.txt",
	}
	fetchPrefixesForCountry(l, config)
}

// Does the complete fetch workflow.
func fetchPrefixesForCountry(l *slog.Logger, cfg Config) {
	asns := cfg.ASNFunc()
	l.Info("fetching prefixes", "country", cfg.Country, "asn_count", len(asns))

	client := &http.Client{Timeout: 30 * time.Second}
	prefixes, err := fetchWithRetry(l, client)
	if err != nil {
		l.Error("fetch failed", "error", err)
		return
	}

	v4Prefixes, v6Prefixes := filterByASN(prefixes, asns)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := writeV4Prefixes(l, v4Prefixes, cfg.OutputFileV4); err != nil {
			l.Error("write IPv4 failed", "error", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := writeV6Prefixes(l, v6Prefixes, cfg.OutputFileV6); err != nil {
			l.Error("write IPv6 failed", "error", err)
		}
	}()

	wg.Wait()
	l.Info("fetch complete", "country", cfg.Country)
}

// Tries multiple times with backoff.
func fetchWithRetry(l *slog.Logger, client *http.Client) ([]Prefix, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		prefixes, err := fetchPrefixes(l, client)
		if err == nil {
			return prefixes, nil
		}

		lastErr = err
		if attempt < maxRetries {
			delay := time.Duration(attempt) * retryDelay
			l.Warn("fetch retry", "attempt", attempt, "delay", delay, "error", err)
			time.Sleep(delay)
		}
	}

	return nil, fmt.Errorf("all %d attempts failed: %w", maxRetries, lastErr)
}

// Downloads BGP data from bgp.tools.
func fetchPrefixes(l *slog.Logger, client *http.Client) ([]Prefix, error) {
	req, err := http.NewRequest("GET", bgpToolsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var prefixes []Prefix
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		var prefix Prefix
		if err := json.Unmarshal(scanner.Bytes(), &prefix); err != nil {
			l.Debug("invalid JSON line", "error", err)
			continue
		}
		prefixes = append(prefixes, prefix)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	return prefixes, nil
}

// Keeps only prefixes from target ASNs.
func filterByASN(prefixes []Prefix, asns []int) ([]netip.Prefix, []netip.Prefix) {
	asnSet := make(map[int]struct{}, len(asns))
	for _, asn := range asns {
		asnSet[asn] = struct{}{}
	}

	var v4, v6 []netip.Prefix
	for _, prefix := range prefixes {
		if _, exists := asnSet[prefix.ASN]; !exists {
			continue
		}

		if prefix.CIDR.Addr().Is4() {
			v4 = append(v4, prefix.CIDR)
		} else if prefix.CIDR.Addr().Is6() {
			v6 = append(v6, prefix.CIDR)
		}
	}

	return v4, v6
}

// Converts prefixes to /24 blocks.
func splitToBlocks(prefix netip.Prefix) []netip.Prefix {
	if prefix.Bits() == 24 {
		return []netip.Prefix{prefix}
	}

	ipInt := ipToInt(prefix.Addr())
	blockCount := 1 << (24 - prefix.Bits())
	blocks := make([]netip.Prefix, blockCount)

	for i := 0; i < blockCount; i++ {
		ip := intToIP(ipInt)
		blocks[i] = netip.PrefixFrom(ip, 24)
		ipInt.Add(ipInt, big.NewInt(256))
	}

	return blocks
}

func ipToInt(ip netip.Addr) *big.Int {
	return big.NewInt(0).SetBytes(ip.AsSlice())
}

func intToIP(ipInt *big.Int) netip.Addr {
	bytes := make([]byte, 4)
	ipInt.FillBytes(bytes)
	ip, _ := netip.AddrFromSlice(bytes)
	return ip
}

func prefixCompare(a, b netip.Prefix) int {
	if c := cmp.Compare(a.Addr().BitLen(), b.Addr().BitLen()); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Bits(), b.Bits()); c != 0 {
		return c
	}
	return a.Addr().Compare(b.Addr())
}

// Saves IPv4 prefixes as /24 blocks.
func writeV4Prefixes(l *slog.Logger, prefixes []netip.Prefix, filename string) error {
	if len(prefixes) == 0 {
		l.Info("no IPv4 prefixes")
		return nil
	}

	uniqueBlocks := make(map[netip.Prefix]struct{})
	for _, prefix := range prefixes {
		for _, block := range splitToBlocks(prefix) {
			uniqueBlocks[block] = struct{}{}
		}
	}

	sortedBlocks := make([]netip.Prefix, 0, len(uniqueBlocks))
	for block := range uniqueBlocks {
		sortedBlocks = append(sortedBlocks, block)
	}
	slices.SortFunc(sortedBlocks, prefixCompare)

	return writeToFile(l, sortedBlocks, filename, "IPv4")
}

// Saves IPv6 prefixes unchanged.
func writeV6Prefixes(l *slog.Logger, prefixes []netip.Prefix, filename string) error {
	if len(prefixes) == 0 {
		l.Info("no IPv6 prefixes")
		return nil
	}

	sorted := make([]netip.Prefix, len(prefixes))
	copy(sorted, prefixes)
	slices.SortFunc(sorted, prefixCompare)

	return writeToFile(l, sorted, filename, "IPv6")
}

// Saves prefixes to disk.
func writeToFile(l *slog.Logger, prefixes []netip.Prefix, filename, family string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, prefix := range prefixes {
		if _, err := writer.WriteString(prefix.String() + "\n"); err != nil {
			return fmt.Errorf("write failed: %w", err)
		}
	}

	l.Info("prefixes written", "family", family, "count", len(prefixes), "file", filename)
	return nil
}
