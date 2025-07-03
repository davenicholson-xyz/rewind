package cmd

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/davenicholson-xyz/rewind/internal/ipc"
	"github.com/davenicholson-xyz/rewind/internal/watcher"
	"github.com/davenicholson-xyz/rewind/network"
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
		stop, _ := cmd.Flags().GetBool("stop")
		if stop {
			if err := stopWatcher(); err != nil {
				app.Logger.WithField("error", err).Error("Failed to stop watcher")
				os.Exit(1)
			}
			return
		}
		
		if err := runWatcher(); err != nil {
			app.Logger.WithField("error", err).Error("Watcher failed")
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
	watchCmd.Flags().BoolP("stop", "s", false, "Stop the rewind watch process")
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

	go ipc.Start()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	app.Logger.Info("Watch manager started. Press Ctrl+C to stop.")
	
	// Block until we receive a signal OR the watch manager context is cancelled
	select {
	case <-sigChan:
		app.Logger.Info("Received shutdown signal, stopping...")
	case <-wm.Context().Done():
		app.Logger.Info("Received stop command via IPC, stopping...")
	}

	return nil

}

func stopWatcher() error {
	app.Logger.Info("Stopping rewind watch process...")
	
	// Create stop message
	message := ipc.Message{
		Action: "stop",
		Path:   "",
	}
	
	messageJSON, err := json.Marshal(message)
	if err != nil {
		app.Logger.WithError(err).Error("Failed to marshal stop message")
		return err
	}
	
	// Send stop command via IPC
	response, err := network.SendToIPC("/tmp/rewind.sock", string(messageJSON))
	if err != nil {
		app.Logger.WithError(err).Error("Failed to send stop command")
		return err
	}
	
	// Parse response
	var ipcResponse ipc.Response
	if err := json.Unmarshal([]byte(response), &ipcResponse); err != nil {
		app.Logger.WithError(err).Error("Failed to parse stop response")
		return err
	}
	
	if ipcResponse.Success {
		app.Logger.Info("Stop command sent successfully")
	} else {
		app.Logger.WithField("message", ipcResponse.Message).Error("Stop command failed")
	}
	
	return nil
}

