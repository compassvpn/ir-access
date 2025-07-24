package fetch

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"
)

const (
	bgpToolsURL = "https://bgp.tools/table.jsonl"
	userAgent   = "compassvpn-prefix-fetcher bgp.tools"
	maxRetries  = 4
	retryDelay  = 1 * time.Second
)

// BGP route entry with its announcing ASN.
type Prefix struct {
	CIDR netip.Prefix `json:"CIDR"`
	ASN  int          `json:"ASN"`
}

// Downloads the full BGP table with exponential backoff on failures.
func fetchWithRetrySimple(client *http.Client) ([]Prefix, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		prefixes, err := fetchPrefixesSimple(client)
		if err == nil {
			return prefixes, nil
		}

		lastErr = err
		if attempt < maxRetries {
			delay := time.Duration(attempt) * retryDelay
			time.Sleep(delay)
		}
	}

	return nil, fmt.Errorf("all %d attempts failed: %w", maxRetries, lastErr)
}

// Streams and parses JSONL BGP data from bgp.tools.
func fetchPrefixesSimple(client *http.Client) ([]Prefix, error) {
	req, err := http.NewRequest("GET", bgpToolsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var prefixes []Prefix
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var prefix Prefix
		if err := json.Unmarshal([]byte(line), &prefix); err != nil {
			continue // Skip malformed lines
		}

		prefixes = append(prefixes, prefix)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return prefixes, nil
}

// Extracts prefixes announced by target ASNs and sorts by IP family.
func filterByASN(prefixes []Prefix, asns []int) ([]netip.Prefix, []netip.Prefix) {
	asnSet := make(map[int]bool)
	for _, asn := range asns {
		asnSet[asn] = true
	}

	var v4, v6 []netip.Prefix

	for _, prefix := range prefixes {
		if !asnSet[prefix.ASN] {
			continue
		}

		if prefix.CIDR.Addr().Is4() {
			v4 = append(v4, prefix.CIDR)
		} else if prefix.CIDR.Addr().Is6() {
			v6 = append(v6, prefix.CIDR)
		}
	}

	slices.SortFunc(v4, prefixCompare)
	slices.SortFunc(v6, prefixCompare)

	return v4, v6
}

// Comparison for deterministic prefix ordering.
func prefixCompare(a, b netip.Prefix) int {
	if c := cmp.Compare(a.Addr().BitLen(), b.Addr().BitLen()); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Bits(), b.Bits()); c != 0 {
		return c
	}
	return a.Addr().Compare(b.Addr())
}
