// Package planner turns a project directory into the work pier up should do.
package planner

import (
	"fmt"
	"path/filepath"

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
	ServiceSpecs  []string
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
		plan.ServiceSpecs = serviceSpecsFromInfra(plan.InfraServices)
		return plan, nil
	}

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
