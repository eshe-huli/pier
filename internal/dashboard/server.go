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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/registry"
)

//go:embed static/*
var staticFiles embed.FS

const DashboardPort = 19191

// ServiceInfo represents a running service for the dashboard
type ServiceInfo struct {
	Name      string `json:"name"`
	Domain    string `json:"domain"`
	URL       string `json:"url"`
	Type      string `json:"type"` // docker, linked, proxy
	Status    string `json:"status"`
	Uptime    string `json:"uptime,omitempty"`
	Port      string `json:"port,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Dir       string `json:"dir,omitempty"`
	Framework string `json:"framework,omitempty"`
	LastUsed  string `json:"lastUsed,omitempty"`
}

// Handler returns an http.Handler for the full dashboard (static + API)
func Handler() http.Handler {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/services", handleServices)
	mux.HandleFunc("/api/services/start", handleStartService)
	mux.HandleFunc("/api/services/stop", handleStopService)
	mux.HandleFunc("/api/projects", handleProjects)
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

	// Merge registry projects that aren't already in services (stopped projects)
	projects, _ := registry.Load()
	activeNames := map[string]bool{}
	for i, svc := range services {
		activeNames[svc.Name] = true
		// Enrich with registry metadata
		for _, p := range projects {
			if p.Name == svc.Name {
				services[i].Dir = p.Dir
				services[i].Framework = p.Framework
				services[i].LastUsed = p.LastUsed
				break
			}
		}
	}
	for _, p := range projects {
		if !activeNames[p.Name] {
			domain := fmt.Sprintf("%s.%s", p.Name, cfg.TLD)
			services = append(services, ServiceInfo{
				Name:      p.Name,
				Domain:    domain,
				URL:       fmt.Sprintf("http://%s", domain),
				Type:      p.Type,
				Status:    "stopped",
				Dir:       p.Dir,
				Framework: p.Framework,
				LastUsed:  p.LastUsed,
			})
		}
	}

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

func handleProjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch r.Method {
	case http.MethodGet:
		projects, err := registry.Load()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"projects": projects})

	case http.MethodDelete:
		var req struct{ Name string `json:"name"` }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" {
			http.Error(w, `{"error":"name required"}`, 400)
			return
		}
		_ = registry.Remove(req.Name)
		json.NewEncoder(w).Encode(map[string]string{"status": "removed"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, 405)
	}
}

func handleStartService(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name required"}`, 400)
		return
	}

	// Try registry first, then legacy links dir
	var meta config.LinkMeta
	projects, _ := registry.Load()
	found := false
	for _, p := range projects {
		if p.Name == req.Name {
			meta = config.LinkMeta{Name: p.Name, Dir: p.Dir, Port: p.Port, Command: p.Command}
			found = true
			break
		}
	}
	if !found {
		metaPath := filepath.Join(config.LinksDir(), req.Name+".json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			http.Error(w, `{"error":"unknown service — register it first with pier link or pier up"}`, 404)
			return
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			http.Error(w, `{"error":"corrupt link metadata"}`, 500)
			return
		}
	}

	if meta.Command == "" {
		http.Error(w, `{"error":"no dev command configured — start it manually"}`, 400)
		return
	}

	// Check if already running
	pidFile := filepath.Join(meta.Dir, ".pier", "dev.pid")
	if pidData, err := os.ReadFile(pidFile); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if pid > 0 {
			if proc, err := os.FindProcess(pid); err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					json.NewEncoder(w).Encode(map[string]interface{}{
						"status": "already_running", "pid": pid,
					})
					return
				}
			}
		}
	}

	// Start the dev server
	pierDir := filepath.Join(meta.Dir, ".pier")
	_ = os.MkdirAll(pierDir, 0755)
	logFile := filepath.Join(pierDir, "dev.log")

	log, err := os.Create(logFile)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"cannot create log: %s"}`, err), 500)
		return
	}

	parts := strings.Fields(meta.Command)
	c := exec.Command(parts[0], parts[1:]...)
	c.Dir = meta.Dir
	c.Stdout = log
	c.Stderr = log
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := c.Start(); err != nil {
		log.Close()
		http.Error(w, fmt.Sprintf(`{"error":"failed to start: %s"}`, err), 500)
		return
	}
	// Do NOT close log here — child process is still writing to it.
	// Close it when the child exits.

	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(c.Process.Pid)), 0644)
	go func() {
		_ = c.Wait()
		log.Close()
	}()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "started", "pid": c.Process.Pid, "command": meta.Command,
	})
}

func handleStopService(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name required"}`, 400)
		return
	}

	// Find project dir from registry or legacy links
	var projectDir string
	projects, _ := registry.Load()
	for _, p := range projects {
		if p.Name == req.Name {
			projectDir = p.Dir
			break
		}
	}
	if projectDir == "" {
		metaPath := filepath.Join(config.LinksDir(), req.Name+".json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			http.Error(w, `{"error":"unknown service"}`, 404)
			return
		}
		var meta config.LinkMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			http.Error(w, `{"error":"corrupt link metadata"}`, 500)
			return
		}
		projectDir = meta.Dir
	}

	pidFile := filepath.Join(projectDir, ".pier", "dev.pid")
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "not_running"})
		return
	}

	pid, _ := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
	_ = os.Remove(pidFile)

	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
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
