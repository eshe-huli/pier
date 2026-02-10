package dashboard

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshe-huli/pier/internal/config"
)

//go:embed static/*
var staticFiles embed.FS

const DashboardPort = 19191

// ServiceInfo represents a running service for the dashboard
type ServiceInfo struct {
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	URL      string `json:"url"`
	Type     string `json:"type"` // docker, linked, proxy
	Status   string `json:"status"`
	Uptime   string `json:"uptime,omitempty"`
	Port     string `json:"port,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// Handler returns an http.Handler for the full dashboard (static + API)
func Handler() http.Handler {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/services", handleServices)
	mux.HandleFunc("/api/health", handleHealth)

	// Static files
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	return mux
}

// Start launches the dashboard server on a fixed port
func Start() (int, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", DashboardPort))
	if err != nil {
		return 0, fmt.Errorf("dashboard port %d in use: %w", DashboardPort, err)
	}

	srv := &http.Server{
		Handler:      Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go srv.Serve(ln)
	return DashboardPort, nil
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": "0.2.0",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	services := []ServiceInfo{}

	// 1. Get routes from Traefik API
	traefikServices := getTraefikRoutes(cfg)
	services = append(services, traefikServices...)

	// 2. Get linked services (dev servers with PID files)
	linkedServices := getLinkedServices(cfg)
	services = append(services, linkedServices...)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"services": services,
		"tld":      cfg.TLD,
		"total":    len(services),
	})
}

func getTraefikRoutes(cfg *config.Config) []ServiceInfo {
	client := &http.Client{Timeout: 2 * time.Second}
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/api/http/routers", cfg.Traefik.Port+1)

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var routers []struct {
		Name     string `json:"name"`
		Rule     string `json:"rule"`
		Status   string `json:"status"`
		Provider string `json:"provider"`
		Service  string `json:"service"`
	}

	if err := json.Unmarshal(body, &routers); err != nil {
		return nil
	}

	var services []ServiceInfo
	for _, r := range routers {
		// Skip internal routes
		if strings.Contains(r.Name, "api@internal") ||
			strings.Contains(r.Name, "dashboard@internal") ||
			strings.Contains(r.Name, "acme") ||
			strings.Contains(r.Name, "pier-dashboard") {
			continue
		}

		domain := extractDomain(r.Rule)
		if domain == "" {
			continue
		}

		svcType := "docker"
		if strings.Contains(r.Provider, "file") {
			svcType = "proxy"
		}

		name := strings.Split(r.Name, "@")[0]
		// Clean up auto-generated names
		name = strings.TrimSuffix(name, "-pier")
		name = strings.TrimSuffix(name, "-router")

		services = append(services, ServiceInfo{
			Name:     name,
			Domain:   domain,
			URL:      fmt.Sprintf("http://%s", domain),
			Type:     svcType,
			Status:   r.Status,
			Provider: r.Provider,
		})
	}

	return services
}

func getLinkedServices(cfg *config.Config) []ServiceInfo {
	// Check for .pier/dev.pid files in common project directories
	// Also check the Traefik dynamic config for file-based routes
	dynamicDir := config.TraefikDynamicDir()
	entries, err := os.ReadDir(dynamicDir)
	if err != nil {
		return nil
	}

	var services []ServiceInfo
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}

		// Read the file to find any linked services
		data, err := os.ReadFile(filepath.Join(dynamicDir, e.Name()))
		if err != nil {
			continue
		}

		content := string(data)
		// Check if this is a linked (dev server) route by looking for localhost URLs
		if strings.Contains(content, "url: \"http://host.docker.internal") ||
			strings.Contains(content, "url: \"http://127.0.0.1") {

			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			domain := fmt.Sprintf("%s.%s", name, cfg.TLD)

			// Check if the service is actually responding
			status := "stopped"
			client := &http.Client{Timeout: 1 * time.Second}
			if resp, err := client.Get(fmt.Sprintf("http://%s", domain)); err == nil {
				resp.Body.Close()
				status = "running"
			}

			services = append(services, ServiceInfo{
				Name:   name,
				Domain: domain,
				URL:    fmt.Sprintf("http://%s", domain),
				Type:   "linked",
				Status: status,
			})
		}
	}

	return services
}

func extractDomain(rule string) string {
	// Parse Host(`domain.dock`) from Traefik rule
	idx := strings.Index(rule, "Host(`")
	if idx == -1 {
		return ""
	}
	rest := rule[idx+6:]
	end := strings.Index(rest, "`)")
	if end == -1 {
		return ""
	}
	return rest[:end]
}
