package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/detect"
	"github.com/eshe-huli/pier/internal/pierfile"
	"github.com/eshe-huli/pier/internal/proxy"
	"github.com/eshe-huli/pier/internal/registry"
)

// LocalProcessAdapter executes Pier plans as host processes behind file proxies.
type LocalProcessAdapter struct{}

func (LocalProcessAdapter) Name() string {
	return "process"
}

// BuildImage resolves the process port and returns without building an image.
func (LocalProcessAdapter) BuildImage(_ context.Context, spec AppSpec) (string, int, error) {
	port, _, err := localProcessPortAndFramework(spec)
	if err != nil {
		return "", 0, err
	}

	image := spec.Image
	if image == "" {
		image = spec.Name
	}
	return image, port, nil
}

// RunApp creates a host-process route, starts a dev command when known, and registers metadata.
func (adapter LocalProcessAdapter) RunApp(_ context.Context, spec AppSpec, _ string, port int, cfg *config.Config, envOverrides []string) error {
	if port == 0 {
		resolvedPort, _, err := localProcessPortAndFramework(spec)
		if err != nil {
			return err
		}
		port = resolvedPort
	}
	if spec.Name == "" {
		return fmt.Errorf("process runtime requires an app name")
	}
	if spec.Dir == "" {
		return fmt.Errorf("process runtime requires an app directory")
	}

	if err := os.MkdirAll(config.TraefikDynamicDir(), 0755); err != nil {
		return fmt.Errorf("creating proxy config directory: %w", err)
	}
	if proxy.FileProxyExists(spec.Name) {
		_ = proxy.RemoveFileProxy(spec.Name)
	}
	if err := proxy.CreateFileProxy(spec.Name, port, cfg.TLD); err != nil {
		return fmt.Errorf("creating process proxy: %w", err)
	}

	command := LocalProcessCommand(spec, port)
	framework := localProcessFrameworkName(spec)
	registerType := "proxy"
	if command != "" {
		registerType = "linked"
		if _, _, err := startLocalProcess(spec, command, port, envOverrides); err != nil {
			return err
		}
	}
	if spec.RegisterType != "" && spec.RegisterType != adapter.Name() {
		registerType = spec.RegisterType
	}

	if err := saveLocalProcessMeta(spec.Name, spec.Dir, port, command, framework, registerType); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not register local process: %v\n", err)
	}

	return nil
}

// DevCommandForFramework returns Pier's conventional host dev command for a detected framework.
func DevCommandForFramework(fw *detect.Framework, port int) string {
	if fw == nil {
		return ""
	}

	switch fw.Name {
	case "nextjs":
		return fmt.Sprintf("npx next dev -p %d", port)
	case "nuxt":
		return fmt.Sprintf("npx nuxi dev --port %d", port)
	case "astro":
		return fmt.Sprintf("npx astro dev --host 0.0.0.0 --port %d", port)
	case "remix":
		return "npm run dev"
	case "nestjs":
		return "npm run start:dev"
	case "express", "fastify":
		return "npm run dev"
	case "django":
		return fmt.Sprintf("python manage.py runserver 0.0.0.0:%d", port)
	case "fastapi":
		return fmt.Sprintf("uvicorn main:app --reload --port %d", port)
	case "flask":
		return fmt.Sprintf("flask run --port %d", port)
	case "go":
		return "go run ."
	case "rails":
		return fmt.Sprintf("rails server -p %d", port)
	case "phoenix":
		return "mix phx.server"
	case "laravel":
		return fmt.Sprintf("php artisan serve --port=%d", port)
	case "spring-boot":
		return "./mvnw spring-boot:run"
	case "rust":
		return "cargo run"
	default:
		return ""
	}
}

// LocalProcessCommand resolves the command process mode should start, if one is known.
func LocalProcessCommand(spec AppSpec, port int) string {
	if command := commandToShell(spec.Command); command != "" {
		return command
	}
	if command := pierfileCommand(spec.Dir); command != "" {
		return command
	}
	if spec.Framework != nil {
		return DevCommandForFramework(spec.Framework, port)
	}
	if fw, err := detect.DetectFramework(localProcessBuildDir(spec)); err == nil {
		return DevCommandForFramework(fw, port)
	}
	return ""
}

func localProcessPortAndFramework(spec AppSpec) (int, *detect.Framework, error) {
	port := spec.Port
	framework := spec.Framework

	if port == 0 {
		if pf, err := pierfile.Load(spec.Dir); err == nil && pf.Port > 0 {
			port = pf.Port
		}
	}
	if framework == nil {
		if fw, err := detect.DetectFramework(localProcessBuildDir(spec)); err == nil {
			framework = fw
		}
	}
	if port == 0 && framework != nil {
		port = framework.Port
	}
	if port == 0 {
		return 0, framework, fmt.Errorf("process runtime could not determine a port; set Pierfile port or use a detected framework")
	}

	return port, framework, nil
}

func localProcessBuildDir(spec AppSpec) string {
	buildDir := spec.Dir
	if spec.BuildCtx != "" {
		buildDir = spec.BuildCtx
		if !filepath.IsAbs(buildDir) {
			buildDir = filepath.Join(spec.Dir, buildDir)
		}
	}
	return buildDir
}

func localProcessFrameworkName(spec AppSpec) string {
	if spec.Framework != nil {
		return spec.Framework.Name
	}
	if fw, err := detect.DetectFramework(localProcessBuildDir(spec)); err == nil {
		return fw.Name
	}
	return ""
}

func commandToShell(command interface{}) string {
	switch cmd := command.(type) {
	case string:
		return strings.TrimSpace(cmd)
	case []interface{}:
		parts := make([]string, 0, len(cmd))
		for _, item := range cmd {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	case []string:
		return strings.TrimSpace(strings.Join(cmd, " "))
	default:
		return ""
	}
}

func pierfileCommand(dir string) string {
	pf, err := pierfile.Load(dir)
	if err != nil {
		return ""
	}
	for _, service := range pf.Services {
		if command := strings.TrimSpace(service.Command); command != "" {
			return command
		}
	}
	return ""
}

func startLocalProcess(spec AppSpec, command string, port int, envOverrides []string) (int, string, error) {
	pierDir := filepath.Join(spec.Dir, ".pier")
	if err := os.MkdirAll(pierDir, 0755); err != nil {
		return 0, "", fmt.Errorf("creating .pier directory: %w", err)
	}

	pidFile := LocalProcessPIDFile(spec.Dir)
	logFile := LocalProcessLogPath(spec.Dir)
	killExistingLocalProcess(pidFile)

	log, err := os.Create(logFile)
	if err != nil {
		return 0, "", fmt.Errorf("creating process log file: %w", err)
	}

	cmd := exec.Command("sh", "-lc", command)
	cmd.Dir = spec.Dir
	cmd.Env = localProcessEnv(os.Environ(), spec, envOverrides, port)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		log.Close()
		return 0, "", fmt.Errorf("starting local process: %w", err)
	}

	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		log.Close()
		return 0, "", fmt.Errorf("writing process pid file: %w", err)
	}

	go func() {
		_ = cmd.Wait()
		log.Close()
	}()

	return cmd.Process.Pid, logFile, nil
}

// LocalProcessPIDFile returns the managed PID file for a host-process project.
func LocalProcessPIDFile(dir string) string {
	return filepath.Join(dir, ".pier", "dev.pid")
}

// LocalProcessLogPath returns the managed log file for a host-process project.
func LocalProcessLogPath(dir string) string {
	return filepath.Join(dir, ".pier", "dev.log")
}

// ResolveLocalProcessMeta looks up the process/proxy metadata for a known project.
func ResolveLocalProcessMeta(name string) (config.LinkMeta, bool, error) {
	projects, err := registry.Load()
	if err != nil {
		return config.LinkMeta{}, false, fmt.Errorf("loading registry: %w", err)
	}
	for _, project := range projects {
		if project.Name == name && isLocalProcessProject(project) {
			return config.LinkMeta{
				Name:    project.Name,
				Dir:     project.Dir,
				Port:    project.Port,
				Command: project.Command,
			}, true, nil
		}
	}

	meta, found, err := readLegacyLinkMeta(name)
	if err != nil {
		return config.LinkMeta{}, false, err
	}
	return meta, found, nil
}

// ListLocalProcessMetas returns deduplicated process/proxy metadata.
func ListLocalProcessMetas() ([]config.LinkMeta, error) {
	seen := map[string]bool{}
	var metas []config.LinkMeta

	projects, err := registry.Load()
	if err != nil {
		return nil, fmt.Errorf("loading registry: %w", err)
	}
	for _, project := range projects {
		if !isLocalProcessProject(project) || project.Name == "" {
			continue
		}
		seen[project.Name] = true
		metas = append(metas, config.LinkMeta{
			Name:    project.Name,
			Dir:     project.Dir,
			Port:    project.Port,
			Command: project.Command,
		})
	}

	entries, err := os.ReadDir(config.LinksDir())
	if err != nil {
		if os.IsNotExist(err) {
			return metas, nil
		}
		return nil, fmt.Errorf("reading links directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if seen[name] {
			continue
		}
		meta, found, err := readLegacyLinkMeta(name)
		if err != nil {
			return nil, err
		}
		if found {
			seen[name] = true
			metas = append(metas, meta)
		}
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Name < metas[j].Name
	})
	return metas, nil
}

// IsLocalProcessRunning checks whether the managed PID still points at a process.
func IsLocalProcessRunning(dir string) (int, bool) {
	data, err := os.ReadFile(LocalProcessPIDFile(dir))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return pid, false
	}
	return pid, true
}

// StartLocalProcessMeta starts a registered host-process project and restores its route.
func StartLocalProcessMeta(meta config.LinkMeta, cfg *config.Config) (int, string, error) {
	if meta.Name == "" {
		return 0, "", fmt.Errorf("process metadata is missing name")
	}
	if meta.Dir == "" {
		return 0, "", fmt.Errorf("process metadata for %s is missing directory", meta.Name)
	}
	if meta.Port == 0 {
		return 0, "", fmt.Errorf("process metadata for %s is missing port", meta.Name)
	}
	if strings.TrimSpace(meta.Command) == "" {
		return 0, "", fmt.Errorf("process metadata for %s has no command; start it manually on port %d", meta.Name, meta.Port)
	}

	if err := os.MkdirAll(config.TraefikDynamicDir(), 0755); err != nil {
		return 0, "", fmt.Errorf("creating proxy config directory: %w", err)
	}
	if proxy.FileProxyExists(meta.Name) {
		_ = proxy.RemoveFileProxy(meta.Name)
	}
	if err := proxy.CreateFileProxy(meta.Name, meta.Port, cfg.TLD); err != nil {
		return 0, "", fmt.Errorf("creating process proxy: %w", err)
	}

	return startLocalProcess(AppSpec{Name: meta.Name, Dir: meta.Dir, Port: meta.Port}, meta.Command, meta.Port, nil)
}

// StopLocalProcessMeta stops the process group for a registered host-process project.
func StopLocalProcessMeta(meta config.LinkMeta) (bool, error) {
	if meta.Dir == "" {
		return false, fmt.Errorf("process metadata for %s is missing directory", meta.Name)
	}
	pidFile := LocalProcessPIDFile(meta.Dir)
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading process pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		_ = os.Remove(pidFile)
		return false, fmt.Errorf("invalid process pid for %s", meta.Name)
	}

	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return false, fmt.Errorf("stopping local process %s: %w", meta.Name, err)
	}
	_ = os.Remove(pidFile)
	return true, nil
}

func killExistingLocalProcess(pidFile string) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = os.Remove(pidFile)
}

func localProcessEnv(base []string, spec AppSpec, envOverrides []string, port int) []string {
	runtimeEnv := []string{fmt.Sprintf("PORT=%d", port)}
	appEnv := envMapEntries(spec.Env)

	layers := [][]string{base, runtimeEnv}
	if spec.RuntimeEnvLast {
		layers = append(layers, appEnv, spec.ExtraEnv, envOverrides)
	} else {
		layers = append(layers, envOverrides, appEnv, spec.ExtraEnv)
	}
	return mergeEnv(layers...)
}

func envMapEntries(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	entries := make([]string, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, fmt.Sprintf("%s=%s", key, env[key]))
	}
	return entries
}

func mergeEnv(layers ...[]string) []string {
	order := []string{}
	values := map[string]string{}
	for _, layer := range layers {
		for _, entry := range layer {
			key, _, ok := strings.Cut(entry, "=")
			if !ok || key == "" {
				continue
			}
			if _, exists := values[key]; !exists {
				order = append(order, key)
			}
			values[key] = entry
		}
	}

	merged := make([]string, 0, len(order))
	for _, key := range order {
		merged = append(merged, values[key])
	}
	return merged
}

func saveLocalProcessMeta(name, dir string, port int, command string, framework string, registerType string) error {
	linksDir := config.LinksDir()
	if err := os.MkdirAll(linksDir, 0755); err != nil {
		return fmt.Errorf("creating links directory: %w", err)
	}

	meta := config.LinkMeta{Name: name, Dir: dir, Port: port, Command: command}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling link metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(linksDir, name+".json"), data, 0644); err != nil {
		return fmt.Errorf("writing link metadata: %w", err)
	}

	if err := registry.Register(registry.Project{
		Name:      name,
		Dir:       dir,
		Port:      port,
		Command:   command,
		Type:      registerType,
		Framework: framework,
	}); err != nil {
		return fmt.Errorf("registering project: %w", err)
	}

	return nil
}

func isLocalProcessProject(project registry.Project) bool {
	switch project.Type {
	case "linked", "proxy", "process":
		return true
	default:
		return project.Command != ""
	}
}

func readLegacyLinkMeta(name string) (config.LinkMeta, bool, error) {
	metaPath := filepath.Join(config.LinksDir(), name+".json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return config.LinkMeta{}, false, nil
		}
		return config.LinkMeta{}, false, fmt.Errorf("reading link metadata: %w", err)
	}
	var meta config.LinkMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return config.LinkMeta{}, false, fmt.Errorf("parsing link metadata: %w", err)
	}
	if meta.Name == "" {
		meta.Name = name
	}
	return meta, true, nil
}
