package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eshe-huli/pier/internal/config"
	"github.com/eshe-huli/pier/internal/proxy"
)

var unproxyCmd = &cobra.Command{
	Use:   "unproxy <name>",
	Short: "Remove a bare-metal proxy route",
	Long: `Removes the domain routing for a bare-metal process.

Example:
  pier unproxy myapp    → removes myapp.dock`,
	Args: cobra.ExactArgs(1),
	RunE: runUnproxy,
}

func init() {
	rootCmd.AddCommand(unproxyCmd)
}

func runUnproxy(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if err := proxy.RemoveFileProxy(name); err != nil {
		return fmt.Errorf("removing proxy: %w", err)
	}

	domain := fmt.Sprintf("%s.%s", name, cfg.TLD)
	fmt.Println()
	success(fmt.Sprintf("%s removed", cyan(domain)))
	fmt.Println()

	return nil
}
