package cli

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
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
