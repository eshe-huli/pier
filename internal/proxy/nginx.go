package proxy

import (
	"fmt"
	"os"

	"github.com/eshe-huli/pier/internal/config"
)

// GenerateNginxConfig generates the nginx server block for the pier TLD
func GenerateNginxConfig(cfg *config.Config) error {
	conf := fmt.Sprintf(`# Pier — managed by pier CLI
# Do not edit manually. Use: pier config

server {
    listen 127.0.0.1:80;
    server_name *.%s;

    location / {
        proxy_pass http://127.0.0.1:%d;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_request_buffering off;

        # WebSocket support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
`, cfg.TLD, cfg.Traefik.Port)

	if err := os.MkdirAll(config.NginxDir(), 0755); err != nil {
		return fmt.Errorf("creating nginx directory: %w", err)
	}

	if err := os.WriteFile(config.NginxConfigPath(), []byte(conf), 0644); err != nil {
		return fmt.Errorf("writing nginx config: %w", err)
	}

	return nil
}

// NginxSymlinkInstruction returns the command to symlink the nginx config
func NginxSymlinkInstruction() string {
	return fmt.Sprintf("sudo ln -sf %s /opt/homebrew/etc/nginx/servers/pier.conf && sudo nginx -s reload",
		config.NginxConfigPath())
}

// IsNginxConfigLinked checks if the nginx config is symlinked
func IsNginxConfigLinked() bool {
	target := "/opt/homebrew/etc/nginx/servers/pier.conf"
	info, err := os.Lstat(target)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// IsNginxRunning checks if nginx process is running (basic check)
func IsNginxRunning() bool {
	_, err := os.Stat("/opt/homebrew/var/run/nginx.pid")
	return err == nil
}
