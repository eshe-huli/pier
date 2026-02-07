package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/eshe-huli/pier/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or modify Pier configuration",
	Long: `View or modify Pier configuration.

  pier config              Show all config
  pier config get <key>    Get a specific value
  pier config set <key> <value>  Set a value`,
	RunE: runConfig,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %s %s\n", dim("Config:"), dim(config.ConfigPath()))
	fmt.Println()
	fmt.Println(string(data))

	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	val, err := cfg.Get(args[0])
	if err != nil {
		return err
	}

	fmt.Println(val)
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	key := args[0]
	value := args[1]

	oldVal, _ := cfg.Get(key)

	if err := cfg.Set(key, value); err != nil {
		return err
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	if oldVal != value {
		success(fmt.Sprintf("%s: %s → %s", bold(key), dim(oldVal), green(value)))
		fmt.Println()
		if key == "tld" {
			info("Run 'pier restart' to apply TLD changes.")
			info("You may also need to update dnsmasq and /etc/resolver.")
		}
	} else {
		info(fmt.Sprintf("%s is already set to %s", key, value))
	}
	fmt.Println()

	return nil
}
