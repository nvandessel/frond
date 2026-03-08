package cmd

import (
	"fmt"

	"github.com/nvandessel/frond/internal/driver"
	"github.com/nvandessel/frond/internal/state"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize frond state with an optional driver",
	Example: `  # Initialize with the default native driver
  frond init

  # Initialize with the Graphite driver
  frond init --driver graphite`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().String("driver", "", "Driver to use: native (default), graphite")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	driverName, _ := cmd.Flags().GetString("driver")

	// Validate the driver is known and its CLI is available.
	drv, err := driver.Resolve(driverName)
	if err != nil {
		return err
	}

	// Lock state.
	unlock, err := state.Lock(ctx)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer unlock()

	// ReadOrInit creates state if needed.
	s, err := state.ReadOrInit(ctx)
	if err != nil {
		return fmt.Errorf("reading state: %w", err)
	}

	// Set the driver if specified (or clear to native default).
	if driverName == "native" {
		driverName = ""
	}
	s.Driver = driverName

	if err := state.Write(ctx, s); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	if jsonOut {
		return printJSON(initResult{
			Driver: drv.Name(),
			Trunk:  s.Trunk,
		})
	}
	fmt.Printf("Initialized frond (driver: %s, trunk: %s)\n", drv.Name(), s.Trunk)
	return nil
}

type initResult struct {
	Driver string `json:"driver"`
	Trunk  string `json:"trunk"`
}
