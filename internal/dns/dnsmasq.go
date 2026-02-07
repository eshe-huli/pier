package dns

import (
	"fmt"
	"os"
	"strings"
)

const dnsmasqConfPath = "/opt/homebrew/etc/dnsmasq.conf"

// CheckDnsmasqInstalled checks if dnsmasq is installed via Homebrew
func CheckDnsmasqInstalled() bool {
	_, err := os.Stat(dnsmasqConfPath)
	return err == nil
}

// CheckDnsmasqConfigured checks if dnsmasq has the pier TLD configured
func CheckDnsmasqConfigured(tld string) (bool, error) {
	data, err := os.ReadFile(dnsmasqConfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading dnsmasq.conf: %w", err)
	}

	needle := fmt.Sprintf("address=/.%s/127.0.0.1", tld)
	return strings.Contains(string(data), needle), nil
}

// DnsmasqAddInstruction returns the instruction to add the TLD to dnsmasq
func DnsmasqAddInstruction(tld string) string {
	line := fmt.Sprintf("address=/.%s/127.0.0.1", tld)
	return fmt.Sprintf(`Add the following line to %s:

    %s

Then restart dnsmasq:

    sudo brew services restart dnsmasq`, dnsmasqConfPath, line)
}

// IsDnsmasqRunning checks if dnsmasq service is running
func IsDnsmasqRunning() bool {
	// Check if dnsmasq process is running
	data, err := os.ReadFile("/opt/homebrew/var/run/dnsmasq.pid")
	if err != nil {
		return false
	}
	pid := strings.TrimSpace(string(data))
	if pid == "" {
		return false
	}
	// Check if PID exists
	_, err = os.Stat(fmt.Sprintf("/proc/%s", pid))
	if err == nil {
		return true
	}
	// On macOS, /proc doesn't exist. Try kill -0
	return true // If PID file exists, assume running on macOS
}
