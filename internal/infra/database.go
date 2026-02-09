package infra

import (
	"fmt"
	"os/exec"
	"strings"
)

// CreateDatabase creates a database in a shared postgres/mysql instance
func CreateDatabase(serviceName, version, dbName string) error {
	// Sanitize: replace dashes with underscores for DB names
	dbName = strings.ReplaceAll(dbName, "-", "_")
	cname := containerName(serviceName, version)

	var cmd *exec.Cmd
	switch serviceName {
	case "postgres":
		// Simple: try to create, ignore "already exists" error
		sql := fmt.Sprintf(`CREATE DATABASE %s;`, dbName)
		cmd = exec.Command("docker", "exec", cname, "psql", "-U", "pier", "-h", "localhost", "-c", sql)
	case "mysql":
		sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", dbName)
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
