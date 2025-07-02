package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/davenicholson-xyz/rewind/internal/ipc"
	"github.com/davenicholson-xyz/rewind/internal/watcher"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runWatcher(); err != nil {
			app.Logger.WithField("error", err).Error("Watcher failed")
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

func runWatcher() error {

	lm, err := watcher.NewWatchList()
	if err != nil {
		return err
	}

	wm, err := watcher.NewWatchManager(lm)
	if err != nil {
		return err
	}

	ipc, err := ipc.NewHandler(wm)
	if err != nil {
		return err
	}

	err = wm.Start()
	if err != nil {
		return err
	}
	defer wm.Stop()

	ipc.Start()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	app.Logger.Info("Watch manager started. Press Ctrl+C to stop.")
	<-sigChan // Block until we receive a signal
	app.Logger.Info("Received shutdown signal, stopping...")

	return nil

}
