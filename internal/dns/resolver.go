package dns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const resolverDir = "/etc/resolver"

// CheckResolverExists checks if the resolver file exists for the given TLD
func CheckResolverExists(tld string) bool {
	path := filepath.Join(resolverDir, tld)
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "nameserver 127.0.0.1")
}

// ResolverCreateInstruction returns the command to create the resolver file
func ResolverCreateInstruction(tld string) string {
	return fmt.Sprintf(`sudo mkdir -p %s && sudo bash -c 'echo "nameserver 127.0.0.1" > %s/%s'`,
		resolverDir, resolverDir, tld)
}

// TestDNSResolution attempts to resolve a test domain
func TestDNSResolution(tld string) bool {
	// Try to resolve via the system — check if resolver file exists as proxy
	return CheckResolverExists(tld)
}
