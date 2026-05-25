package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/docker"
	"github.com/eshe-huli/pier/internal/orchestrator"
)

var logsFollow bool
var logsTail string

var logsCmd = &cobra.Command{
	Use:   "logs [name]",
	Short: "View container logs",
	Long: `View logs from a running container.

Examples:
  pier logs myapp          Show logs
  pier logs myapp -f       Follow logs (stream)
  pier logs myapp --tail 50  Show last 50 lines`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().StringVar(&logsTail, "tail", "100", "Number of lines to show from the end")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	name, err := resolveProjectName(args)
	if err != nil {
		return err
	}

	if meta, found, err := orchestrator.ResolveLocalProcessMeta(name); err != nil {
		return err
	} else if found {
		logPath := orchestrator.LocalProcessLogPath(meta.Dir)
		if _, err := os.Stat(logPath); err == nil {
			return runTail(logPath)
		}
		if !docker.IsContainerRunning(context.Background(), name) {
			return fmt.Errorf("no process log found for %s at %s", name, logPath)
		}
	}

	dockerArgs := []string{"logs", "--tail", logsTail}
	if logsFollow {
		dockerArgs = append(dockerArgs, "-f")
	}
	dockerArgs = append(dockerArgs, name)

	c := exec.Command("docker", dockerArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func runTail(logPath string) error {
	tailArgs := []string{"-n", logsTail}
	if logsFollow {
		tailArgs = append(tailArgs, "-f")
	}
	tailArgs = append(tailArgs, logPath)

	c := exec.Command("tail", tailArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
