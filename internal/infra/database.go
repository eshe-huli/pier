package infra

import (
	"fmt"
	"os/exec"
	"strings"
)

// CreateDatabase creates a database in a shared postgres/mysql instance
func CreateDatabase(serviceName, version, dbName string) error {
	// Sanitize: replace dashes with underscores, strip dangerous chars
	dbName = strings.ReplaceAll(dbName, "-", "_")
	dbName = sanitizeIdentifier(dbName)
	if dbName == "" {
		return fmt.Errorf("invalid database name after sanitization")
	}
	cname := containerName(serviceName, version)

	var cmd *exec.Cmd
	switch serviceName {
	case "postgres":
		// Use quoted identifier to prevent SQL injection
		sql := fmt.Sprintf(`CREATE DATABASE "%s";`, strings.ReplaceAll(dbName, `"`, `""`))
		cmd = exec.Command("docker", "exec", cname, "psql", "-U", "pier", "-h", "localhost", "-c", sql)
	case "mysql":
		// Backtick-quoted identifier with escaping
		escaped := strings.ReplaceAll(dbName, "`", "``")
		sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", escaped)
		cmd = exec.Command("docker", "exec", cname, "mysql", "-uroot", "-ppier", "-e", sql)
	default:
		return fmt.Errorf("CreateDatabase not supported for service: %s", serviceName)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore "already exists" errors
		if strings.Contains(string(out), "already exists") {
			return nil
		}
		return fmt.Errorf("creating database '%s' in %s: %s\n%s", dbName, cname, err, string(out))
	}

	return nil
}

// sanitizeIdentifier removes characters that aren't safe for database identifiers.
func sanitizeIdentifier(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
