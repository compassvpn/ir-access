package fetch

import (
	"fmt"
	"math/big"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"
)

// PrefixSet represents IPv4 and IPv6 prefixes for a country
type PrefixSet struct {
	IPv4 []netip.Prefix
	IPv6 []netip.Prefix
}

// GetASNsForCountry fetches ASN allocations for a country from its RIR
func GetASNsForCountry(country string) ([]int, error) {
	fetcher := NewMultiRIRASNFetcher()
	return fetcher.FetchASNsForCountry(country)
}

// GetPrefixesForASNs fetches BGP prefixes for the given ASNs
func GetPrefixesForASNs(asns []int) (*PrefixSet, error) {
	if len(asns) == 0 {
		return &PrefixSet{}, nil
	}

	// Create HTTP client
	client := &http.Client{Timeout: 30 * time.Second}

	// Fetch all BGP data
	bgpPrefixes, err := fetchWithRetrySimple(client)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch BGP data: %w", err)
	}

	// Filter and separate IPv4/IPv6
	ipv4, ipv6 := filterByASN(bgpPrefixes, asns)

	// Convert IPv4 prefixes to /24 blocks
	ipv4Blocks := convertToIPv4Blocks(ipv4)

	return &PrefixSet{
		IPv4: ipv4Blocks,
		IPv6: ipv6,
	}, nil
}

// convertToIPv4Blocks converts IPv4 prefixes to /24 blocks
func convertToIPv4Blocks(prefixes []netip.Prefix) []netip.Prefix {
	if len(prefixes) == 0 {
		return nil
	}

	// Use map to avoid duplicates
	blockSet := make(map[netip.Prefix]bool)

	for _, prefix := range prefixes {
		if !prefix.Addr().Is4() {
			continue
		}

		blocks := splitToBlocks(prefix)
		for _, block := range blocks {
			blockSet[block] = true
		}
	}

	// Convert to slice and sort
	result := make([]netip.Prefix, 0, len(blockSet))
	for block := range blockSet {
		result = append(result, block)
	}

	slices.SortFunc(result, prefixCompare)
	return result
}

// splitToBlocks converts a prefix to /24 blocks
func splitToBlocks(prefix netip.Prefix) []netip.Prefix {
	if prefix.Bits() >= 24 {
		// For /24 or smaller, create a single /24 block
		addr := prefix.Addr()
		bytes := addr.As4()
		bytes[3] = 0 // Zero out the last octet
		baseAddr, _ := netip.AddrFromSlice(bytes[:])
		block := netip.PrefixFrom(baseAddr, 24)
		return []netip.Prefix{block}
	}

	// For larger prefixes (e.g., /16, /20), split into /24 blocks
	blockCount := 1 << (24 - prefix.Bits())
	blocks := make([]netip.Prefix, blockCount)

	baseInt := ipToInt(prefix.Addr())
	increment := big.NewInt(256) // 2^8 = 256 addresses per /24

	for i := 0; i < blockCount; i++ {
		ip := intToIP(baseInt)
		blocks[i] = netip.PrefixFrom(ip, 24)
		baseInt.Add(baseInt, increment)
	}

	return blocks
}

// ipToInt converts an IP address to a big.Int
func ipToInt(ip netip.Addr) *big.Int {
	return big.NewInt(0).SetBytes(ip.AsSlice())
}

// intToIP converts a big.Int back to an IP address
func intToIP(ipInt *big.Int) netip.Addr {
	bytes := make([]byte, 4)
	ipInt.FillBytes(bytes)
	addr, _ := netip.AddrFromSlice(bytes)
	return addr
}

// SavePrefixesToFiles saves prefixes to country-specific files
func SavePrefixesToFiles(country string, prefixes *PrefixSet) error {
	countryLower := strings.ToLower(country)

	// Save IPv4 prefixes (now as /24 blocks)
	ipv4File := fmt.Sprintf("%s_prefixes_v4.txt", countryLower)
	if err := writePrefixesToFile(ipv4File, prefixes.IPv4); err != nil {
		return fmt.Errorf("failed to save IPv4 prefixes: %w", err)
	}
	fmt.Printf("IPv4 /24 blocks written to: %s (%d entries)\n", ipv4File, len(prefixes.IPv4))

	// Save IPv6 prefixes
	ipv6File := fmt.Sprintf("%s_prefixes_v6.txt", countryLower)
	if err := writePrefixesToFile(ipv6File, prefixes.IPv6); err != nil {
		return fmt.Errorf("failed to save IPv6 prefixes: %w", err)
	}
	fmt.Printf("IPv6 prefixes written to: %s (%d entries)\n", ipv6File, len(prefixes.IPv6))

	return nil
}

// writePrefixesToFile writes netip.Prefix slice to a file
func writePrefixesToFile(filename string, prefixes []netip.Prefix) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	for _, prefix := range prefixes {
		if _, err := file.WriteString(prefix.String() + "\n"); err != nil {
			return fmt.Errorf("failed to write prefix: %w", err)
		}
	}

	return nil
}
