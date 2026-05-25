package planner

import (
	"path/filepath"
	"testing"
)

func TestPlanProject_FrameworkFixtureMatrixRoutesToDockDomains(t *testing.T) {
	tests := []struct {
		name           string
		files          map[string]string
		wantFramework  string
		wantLanguage   string
		wantPort       int
		wantDomain     string
		wantServices   []string
		wantNoServices []string
	}{
		{
			name: "next-app",
			files: map[string]string{
				"package.json": `{"dependencies":{"next":"15.0.0","react":"19.0.0"}}`,
			},
			wantFramework: "nextjs",
			wantLanguage:  "node",
			wantPort:      3000,
			wantDomain:    "next-app.dock",
		},
		{
			name: "nestjs-api",
			files: map[string]string{
				"package.json": `{"dependencies":{"@nestjs/core":"^11.0.0","pg":"^8.0.0","ioredis":"^5.0.0"}}`,
			},
			wantFramework: "nestjs",
			wantLanguage:  "node",
			wantPort:      3000,
			wantDomain:    "nestjs-api.dock",
			wantServices:  []string{"postgres:16", "redis:7"},
		},
		{
			name: "astro-site",
			files: map[string]string{
				"package.json": `{"dependencies":{"astro":"^5.0.0"}}`,
			},
			wantFramework: "astro",
			wantLanguage:  "node",
			wantPort:      4321,
			wantDomain:    "astro-site.dock",
		},
		{
			name: "remix-web",
			files: map[string]string{
				"package.json": `{"dependencies":{"@remix-run/react":"^2.0.0"}}`,
			},
			wantFramework: "remix",
			wantLanguage:  "node",
			wantPort:      3000,
			wantDomain:    "remix-web.dock",
		},
		{
			name: "rails-app",
			files: map[string]string{
				"Gemfile": `source "https://rubygems.org"
gem "rails"
gem "pg"
`,
			},
			wantFramework: "rails",
			wantLanguage:  "ruby",
			wantPort:      3000,
			wantDomain:    "rails-app.dock",
			wantServices:  []string{"postgres:16"},
		},
		{
			name: "go-api",
			files: map[string]string{
				"go.mod": `module example.com/go-api

go 1.25
`,
				"main.go": "package main\n\nfunc main() {}\n",
			},
			wantFramework: "go",
			wantLanguage:  "go",
			wantPort:      8080,
			wantDomain:    "go-api.dock",
		},
		{
			name: "django-api",
			files: map[string]string{
				"requirements.txt": "django==5.0\npsycopg2-binary==2.9\nredis==5.0\n",
			},
			wantFramework: "django",
			wantLanguage:  "python",
			wantPort:      8000,
			wantDomain:    "django-api.dock",
			wantServices:  []string{"postgres:16", "redis:7"},
		},
		{
			name: "fastapi-api",
			files: map[string]string{
				"requirements.txt": "fastapi==0.115.0\nsqlalchemy==2.0\n",
			},
			wantFramework:  "fastapi",
			wantLanguage:   "python",
			wantPort:       8000,
			wantDomain:     "fastapi-api.dock",
			wantServices:   []string{"postgres:16"},
			wantNoServices: []string{"redis:7"},
		},
		{
			name: "laravel-api",
			files: map[string]string{
				"composer.json": `{"require":{"laravel/framework":"^11.0"}}`,
			},
			wantFramework: "laravel",
			wantLanguage:  "php",
			wantPort:      8000,
			wantDomain:    "laravel-api.dock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), tt.name)
			for name, content := range tt.files {
				writeFile(t, dir, name, content)
			}

			plan, err := PlanProject(dir)
			if err != nil {
				t.Fatalf("PlanProject returned error: %v", err)
			}

			if plan.ProjectName != tt.name {
				t.Fatalf("got project name %q, want %q", plan.ProjectName, tt.name)
			}
			if plan.Framework == nil {
				t.Fatalf("got nil framework, want %s", tt.wantFramework)
			}
			if plan.Framework.Name != tt.wantFramework {
				t.Fatalf("got framework %q, want %q", plan.Framework.Name, tt.wantFramework)
			}
			if plan.Framework.Language != tt.wantLanguage {
				t.Fatalf("got language %q, want %q", plan.Framework.Language, tt.wantLanguage)
			}
			if len(plan.Apps) != 1 {
				t.Fatalf("got %d app plans, want 1", len(plan.Apps))
			}

			app := plan.Apps[0]
			if app.Name != tt.name {
				t.Fatalf("got app name %q, want %q", app.Name, tt.name)
			}
			if app.Port != tt.wantPort {
				t.Fatalf("got app port %d, want %d", app.Port, tt.wantPort)
			}
			if app.Domain("dock") != tt.wantDomain {
				t.Fatalf("got domain %q, want %q", app.Domain("dock"), tt.wantDomain)
			}
			if app.Domain(".dock") != tt.wantDomain {
				t.Fatalf("got domain with dotted tld %q, want %q", app.Domain(".dock"), tt.wantDomain)
			}

			for _, service := range tt.wantServices {
				assertServiceSpec(t, plan.ServiceSpecs, service)
			}
			for _, service := range tt.wantNoServices {
				assertNoServiceSpec(t, plan.ServiceSpecs, service)
			}
		})
	}
}
