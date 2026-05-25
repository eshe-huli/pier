package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPierUpDryRunPlansWithoutSideEffects(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, dir, "package.json", `{"dependencies":{"next":"15.0.0"}}`)
	chdir(t, dir)

	restore := setUpGlobalsForTest()
	defer restore()
	upDryRun = true

	output := captureStdout(t, func() {
		if err := runUp(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runUp returned error: %v", err)
		}
	})

	assertOutputContains(t, output, "Dry run: pier up would execute this plan")
	assertOutputContains(t, output, "Framework:")
	assertOutputContains(t, output, "nextjs")
	assertOutputContains(t, output, "No Docker, proxy, registry, manifest, or gitignore changes were made.")

	if _, err := os.Stat(filepath.Join(dir, ".pier", "manifest.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote gitignore: %v", err)
	}
}

func TestPierUpDryRunShowsProcessRuntime(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, dir, "package.json", `{"dependencies":{"next":"15.0.0"}}`)
	chdir(t, dir)

	restore := setUpGlobalsForTest()
	defer restore()
	upDryRun = true
	upRuntime = "process"

	output := captureStdout(t, func() {
		if err := runUp(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runUp returned error: %v", err)
		}
	})

	assertOutputContains(t, output, "Runtime:")
	assertOutputContains(t, output, "process")
	assertOutputContains(t, output, "command=npx next dev -p 3000")

	if _, err := os.Stat(filepath.Join(dir, ".pier", "manifest.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".pier", "registry.json")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote registry: %v", err)
	}
}

func TestPierRunDryRunPlansImageServicesAndRoute(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, dir, "Pierfile", "name: api\nservices:\n  - postgres:16\n")
	chdir(t, dir)

	restore := setRunGlobalsForTest()
	defer restore()
	runDryRun = true
	runImage = "node:20"
	runPort = 3000
	runEnvs = []string{"FEATURE_FLAG=true"}

	output := captureStdout(t, func() {
		if err := runRunCmd(&cobra.Command{}, []string{"api"}); err != nil {
			t.Fatalf("runRunCmd returned error: %v", err)
		}
	})

	assertOutputContains(t, output, "Dry run: pier run would execute this plan")
	assertOutputContains(t, output, "Container:")
	assertOutputContains(t, output, "api")
	assertOutputContains(t, output, "node:20")
	assertOutputContains(t, output, "api.dock")
	assertOutputContains(t, output, "postgres:16")
	assertOutputContains(t, output, "FEATURE_FLAG=true")

	if _, err := os.Stat(filepath.Join(home, ".pier", "registry.json")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote registry: %v", err)
	}
}

func setUpGlobalsForTest() func() {
	oldDetach := upDetach
	oldBuild := upBuild
	oldDryRun := upDryRun
	oldRuntime := upRuntime
	upDetach = true
	upBuild = false
	upDryRun = false
	upRuntime = "docker"
	return func() {
		upDetach = oldDetach
		upBuild = oldBuild
		upDryRun = oldDryRun
		upRuntime = oldRuntime
	}
}

func setRunGlobalsForTest() func() {
	oldImage := runImage
	oldPort := runPort
	oldEnvs := runEnvs
	oldBuild := runBuild
	oldServices := runServices
	oldDryRun := runDryRun
	runImage = ""
	runPort = 0
	runEnvs = nil
	runBuild = false
	runServices = nil
	runDryRun = false
	return func() {
		runImage = oldImage
		runPort = oldPort
		runEnvs = oldEnvs
		runBuild = oldBuild
		runServices = oldServices
		runDryRun = oldDryRun
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("changing cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldDir)
	})
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating test directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating stdout pipe: %v", err)
	}
	os.Stdout = writer

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, reader)
		close(done)
	}()

	fn()

	_ = writer.Close()
	os.Stdout = original
	<-done
	_ = reader.Close()

	return buf.String()
}

func assertOutputContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Fatalf("output missing %q:\n%s", want, output)
	}
}
