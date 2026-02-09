package runtime

import (
	"fmt"

	"github.com/eshe-huli/pier/internal/infra"
)

// BuildEnvOverrides generates environment variable overrides from shared services
func BuildEnvOverrides(services []infra.SharedService) []string {
	var envs []string

	for _, svc := range services {
		connEnv := infra.GetConnectionEnv(svc.Name, svc.Version)
		for k, v := range connEnv {
			envs = append(envs, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return envs
}
