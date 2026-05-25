// Package planner turns a project directory into the work pier up should do.
package planner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshe-huli/pier/internal/compose"
	"github.com/eshe-huli/pier/internal/detect"
	"github.com/eshe-huli/pier/internal/pierfile"
)

type Source string

const (
	SourceCompose   Source = "compose"
	SourcePierfile  Source = "pierfile"
	SourceDetection Source = "detection"
	SourceNone      Source = "none"
)

// Plan is the low-level runtime plan for a project.
type Plan struct {
	Dir           string
	ProjectName   string
	Source        Source
	Pierfile      *pierfile.Pierfile
	Framework     *detect.Framework
	ComposeFile   *compose.ComposeFile
	InfraServices []compose.InfraService
	AppServices   []compose.AppService
	Apps          []AppPlan
	ServiceSpecs  []string
}

// AppPlan is the low-level runtime shape pier up executes for an app.
type AppPlan struct {
	Name         string
	ComposeName  string
	BuildContext string
	Dockerfile   string
	Image        string
	Port         int
	Env          map[string]string
	Volumes      []string
	Command      interface{}
	Entrypoint   interface{}
	UseEnvFile   bool
}

func PlanProject(dir string) (*Plan, error) {
	projectName := filepath.Base(filepath.Clean(dir))

	plan := &Plan{
		Dir:         dir,
		ProjectName: projectName,
		Source:      SourceNone,
	}

	if pierfile.Exists(dir) {
		pf, err := pierfile.Load(dir)
		if err != nil {
			return nil, fmt.Errorf("loading Pierfile: %w", err)
		}
		plan.Pierfile = pf
		if pf.Name != "" {
			plan.ProjectName = pf.Name
		}
	}

	if fw, err := detect.DetectFramework(dir); err == nil {
		plan.Framework = fw
	}

	if cf, err := compose.Parse(dir); err == nil {
		infraServices, appServices := compose.SeparateServices(cf)
		plan.Source = SourceCompose
		plan.ComposeFile = cf
		plan.InfraServices = normalizeInfra(infraServices)
		plan.AppServices = appServices
		plan.Apps = appPlansFromCompose(dir, plan.ProjectName, appServices)
		if len(plan.Apps) == 0 {
			plan.Apps = []AppPlan{defaultAppPlan(dir, plan.ProjectName, plan.Pierfile, plan.Framework)}
		}
		plan.ServiceSpecs = serviceSpecsFromInfra(plan.InfraServices)
		return plan, nil
	}

	plan.Apps = []AppPlan{defaultAppPlan(dir, plan.ProjectName, plan.Pierfile, plan.Framework)}

	if plan.Pierfile != nil && len(plan.Pierfile.Services) > 0 {
		plan.Source = SourcePierfile
		plan.ServiceSpecs = serviceSpecsFromPierfile(plan.Pierfile.Services)
		return plan, nil
	}

	detected, err := detect.DetectServices(dir)
	if err != nil {
		return nil, fmt.Errorf("detecting services: %w", err)
	}
	if len(detected) > 0 {
		plan.Source = SourceDetection
		plan.ServiceSpecs = serviceSpecsFromDetected(detected)
	}

	return plan, nil
}

// RuntimeEnv appends Pier's runtime database identity to service connection env.
func RuntimeEnv(projectName string, envOverrides []string) []string {
	dbName := strings.ReplaceAll(projectName, "-", "_")
	envs := make([]string, 0, len(envOverrides)+2)
	envs = append(envs, envOverrides...)
	envs = append(envs,
		fmt.Sprintf("DATABASE_NAME=%s", dbName),
		fmt.Sprintf("DB_DATABASE=%s", dbName),
	)
	return envs
}

func appPlansFromCompose(dir, projectName string, services []compose.AppService) []AppPlan {
	apps := make([]AppPlan, 0, len(services))
	for _, service := range services {
		appName := projectName
		if len(services) > 1 {
			appName = service.ComposeName
		}

		buildContext := ""
		if service.Build != "" {
			buildContext = resolvePath(dir, service.Build)
		}

		image := appName
		if service.Build == "" && service.Image != "" {
			image = service.Image
		}

		apps = append(apps, AppPlan{
			Name:         appName,
			ComposeName:  service.ComposeName,
			BuildContext: buildContext,
			Dockerfile:   resolveDockerfile(buildContext, service.Dockerfile),
			Image:        image,
			Port:         compose.ParseFirstPort(service.Ports),
			Env:          copyEnv(service.Environment),
			Volumes:      resolveBindVolumes(dir, service.Volumes),
			Command:      service.Command,
			Entrypoint:   service.Entrypoint,
			UseEnvFile:   service.Build != "",
		})
	}
	return apps
}

func defaultAppPlan(dir, projectName string, pf *pierfile.Pierfile, fw *detect.Framework) AppPlan {
	port := 0
	env := map[string]string{}
	if pf != nil {
		port = pf.Port
		env = copyEnv(pf.Env)
	}
	if port == 0 && fw != nil {
		port = fw.Port
	}

	return AppPlan{
		Name:         projectName,
		BuildContext: resolvePath(dir, "."),
		Dockerfile:   existingDockerfile(dir),
		Image:        projectName,
		Port:         port,
		Env:          env,
		UseEnvFile:   true,
	}
}

func resolveBindVolumes(dir string, volumes []string) []string {
	resolved := make([]string, 0, len(volumes))
	for _, volume := range volumes {
		parts := strings.SplitN(volume, ":", 3)
		if len(parts) < 2 || !isBindMountSource(parts[0]) {
			continue
		}

		hostPath := parts[0]
		if !filepath.IsAbs(hostPath) {
			hostPath = resolvePath(dir, hostPath)
		}

		resolved = append(resolved, hostPath+":"+strings.Join(parts[1:], ":"))
	}
	return resolved
}

func isBindMountSource(source string) bool {
	return filepath.IsAbs(source) ||
		source == "." ||
		source == ".." ||
		strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		strings.Contains(source, string(filepath.Separator))
}

func resolveDockerfile(buildContext, dockerfile string) string {
	if dockerfile == "" {
		return ""
	}
	if filepath.IsAbs(dockerfile) {
		return filepath.Clean(dockerfile)
	}
	if buildContext == "" {
		return filepath.Clean(dockerfile)
	}
	return resolvePath(buildContext, dockerfile)
}

func existingDockerfile(dir string) string {
	path := filepath.Join(dir, "Dockerfile")
	if _, err := os.Stat(path); err == nil {
		return resolvePath(dir, "Dockerfile")
	}
	return ""
}

func copyEnv(env map[string]string) map[string]string {
	copied := make(map[string]string, len(env))
	for key, value := range env {
		copied[key] = value
	}
	return copied
}

func resolvePath(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	resolved, err := filepath.Abs(filepath.Join(base, path))
	if err != nil {
		return filepath.Clean(filepath.Join(base, path))
	}
	return resolved
}

func normalizeInfra(services []compose.InfraService) []compose.InfraService {
	normalized := make([]compose.InfraService, len(services))
	for i, service := range services {
		normalized[i] = service
		if normalized[i].Version == "" {
			normalized[i].Version = DefaultVersion(normalized[i].Name)
		}
	}
	return normalized
}

func serviceSpecsFromInfra(services []compose.InfraService) []string {
	specs := make([]string, 0, len(services))
	for _, service := range services {
		specs = append(specs, pierfile.FormatService(service.Name, service.Version))
	}
	return specs
}

func serviceSpecsFromPierfile(services []pierfile.ServiceEntry) []string {
	specs := make([]string, 0, len(services))
	for _, service := range services {
		version := service.Version
		if version == "" {
			version = DefaultVersion(service.Name)
		}
		specs = append(specs, pierfile.FormatService(service.Name, version))
	}
	return specs
}

func serviceSpecsFromDetected(services []detect.ServiceDep) []string {
	specs := make([]string, 0, len(services))
	for _, service := range services {
		version := service.Version
		if version == "" {
			version = DefaultVersion(service.Name)
		}
		specs = append(specs, pierfile.FormatService(service.Name, version))
	}
	return specs
}

func DefaultVersion(name string) string {
	defaults := map[string]string{
		"postgres":      "16",
		"redis":         "7",
		"mongo":         "7",
		"mysql":         "8",
		"minio":         "latest",
		"kafka":         "latest",
		"rabbitmq":      "3",
		"elasticsearch": "8",
		"mariadb":       "11",
		"memcached":     "latest",
	}
	if version, ok := defaults[name]; ok {
		return version
	}
	return "latest"
}
