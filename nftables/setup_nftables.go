package nftables

import (
	"bufio"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ir-access/fetch"
)

const (
	nftablesConf   = "/etc/nftables.conf"
	sshdConfigPath = "/etc/ssh/sshd_config"
	defaultSSHPort = 22
)

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findSSHPort(l *slog.Logger) (uint16, error) {
	l.Info("finding SSH port")

	file, err := os.Open(sshdConfigPath)
	if err != nil {
		return 0, fmt.Errorf("could not open sshd configuration at %s: %w", sshdConfigPath, err)
	}
	defer file.Close()

	found := false
	port := uint16(0)
	re := regexp.MustCompile(`^Port\s+(\d+)`)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := re.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}

		m, err := strconv.ParseUint(match[1], 10, 16)
		if err != nil {
			l.Warn("fail to parse Port", "error", err)
			continue
		}

		port = uint16(m)
		found = true
		break
	}

	if !found {
		return 0, fmt.Errorf("couldn't find port in sshd configuration")
	}

	return port, nil
}

func readPrefixes(filePath string) ([]netip.Prefix, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error: file '%s' not found", filePath)
	}
	defer file.Close()

	var prefixes []netip.Prefix
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			prefix, err := netip.ParsePrefix(line)
			if err != nil {
				return nil, fmt.Errorf("failed parsing prefix: '%s': %w", line, err)
			}
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes, nil
}

func initializeNftablesConf(l *slog.Logger, sshdPort uint16) error {
	l.Info("initializing nftables configuration")

	ipv4Prefixes, err := readPrefixes(fetch.OutputFileV4) // Use fetch.OutputFileV4
	if err != nil {
		l.Error("failed to read prefixes file", "family", "v4", "error", err)
		ipv4Prefixes = []netip.Prefix{}
	}

	ipv6Prefixes, err := readPrefixes(fetch.OutputFileV6) // Use fetch.OutputFileV6
	if err != nil {
		l.Error("failed to read prefixes file", "family", "v6", "error", err)
		ipv6Prefixes = []netip.Prefix{}
	}

	if len(ipv4Prefixes) == 0 && len(ipv6Prefixes) == 0 {
		return fmt.Errorf("prefix lists empty")
	}

	// Render nftables config
	config, err := renderNftablesTemplate(sshdPort, ipv4Prefixes, ipv6Prefixes)
	if err != nil {
		return err
	}

	file, err := os.Create(nftablesConf)
	if err != nil {
		return fmt.Errorf("error creating nftables.conf: %w", err)
	}
	defer file.Close()

	if _, err = file.WriteString(config); err != nil {
		return fmt.Errorf("failed writing nftables.conf: %w", err)
	}

	return nil
}

func applyNftables(l *slog.Logger) error {
	l.Info("applying nftables configuration")

	if err := runCommand("nft", "-f", nftablesConf); err != nil {
		return fmt.Errorf("failed to apply nftables configuration: %w", err)
	}

	if err := runCommand("systemctl", "enable", "--now", "nftables"); err != nil {
		return fmt.Errorf("failed to enable and start nftables services: %w", err)
	}

	return nil
}

func verifyNftables(l *slog.Logger) error {
	l.Info("verifying nftables ruleset")
	return runCommand("nft", "list", "ruleset")
}

// StartSetupNftables is the main entry point for setting up nftables.
func StartSetupNftables(l *slog.Logger) error {
	sshdPort, err := findSSHPort(l)
	if err != nil {
		sshdPort = defaultSSHPort
		l.Warn(fmt.Sprintf("couldn't find port from sshd configuration file, using default %d", sshdPort))
	}

	if err := initializeNftablesConf(l, sshdPort); err != nil {
		return fmt.Errorf("failed to initialize nftables configuration: %w", err)
	}
	l.Info("nftables configuration initialized")

	time.Sleep(time.Second)

	if err := applyNftables(l); err != nil {
		return fmt.Errorf("failed to apply nftables configuration: %w", err)
	}
	l.Info("configuration applied and nftables service started")

	time.Sleep(time.Second)

	if err := verifyNftables(l); err != nil {
		return fmt.Errorf("failed to verify nftables ruleset: %w", err)
	}

	time.Sleep(time.Second)

	l.Info("nftables setup is complete. Your server is now Iran-Access-Only except for SSH port")
	l.Info("configuration is saved", "file", nftablesConf)

	return nil
}
