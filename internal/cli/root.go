package cli

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags
var Version = "0.2.0"

var (
	// Colors
	green   = color.New(color.FgGreen).SprintFunc()
	red     = color.New(color.FgRed).SprintFunc()
	yellow  = color.New(color.FgYellow).SprintFunc()
	cyan    = color.New(color.FgCyan).SprintFunc()
	bold    = color.New(color.Bold).SprintFunc()
	dim     = color.New(color.Faint).SprintFunc()
)

var rootCmd = &cobra.Command{
	Use:   "pier",
	Short: "🔩 Pier — Dev environment that just works",
	Long: `Pier — Dev environment that just works.
Like Laravel Valet, but for everyone.

  pier init              Detect framework + generate config
  pier up                Build, run, and route your project
  pier down              Stop a project
  pier ls                List everything running
  pier proxy app 3000    Route app.dock → localhost:3000`,
	Version: Version,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("🔩 Pier %s\n", cyan(Version))
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetVersionTemplate(fmt.Sprintf("🔩 Pier %s\n", Version))
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// success prints a green success message
func success(msg string) {
	fmt.Printf("  %s %s\n", green("✅"), msg)
}

// warn prints a yellow warning message
func warn(msg string) {
	fmt.Printf("  %s %s\n", yellow("⚠️"), msg)
}

// fail prints a red error message
func fail(msg string) {
	fmt.Printf("  %s %s\n", red("❌"), msg)
}

// info prints a cyan info message
func info(msg string) {
	fmt.Printf("  %s %s\n", cyan("ℹ"), msg)
}

// step prints a numbered step
func step(n int, msg string) {
	fmt.Printf("  %s %s\n", dim(fmt.Sprintf("[%d]", n)), msg)
}

// manual prints a command the user needs to run manually
func manual(cmd string) {
	fmt.Printf("      %s %s\n", yellow("→"), cyan(cmd))
}
