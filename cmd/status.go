package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show rewind watch status information",
	Long: `Display detailed information about the rewind watch daemon including:
- Running status and uptime
- Number of active watches and directories
- Event channel status
- Individual watch details`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runStatus(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func runStatus() error {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	// Try to get status from daemon via IPC
	response, err := sendStatusIPC(cwd)
	if err != nil {
		fmt.Printf("Cannot connect to rewind daemon: %v\n", err)
		fmt.Println("The rewind daemon may not be running. Try 'rewind watch' to start it.")
		return nil
	}

	// Parse and display the status
	return displayStatus(response)
}

func sendStatusIPC(path string) (string, error) {
	return sendIPCMessageWithResponse("status", path)
}

func displayStatus(statusJSON string) error {
	// Parse the status JSON
	var status map[string]interface{}
	if err := json.Unmarshal([]byte(statusJSON), &status); err != nil {
		return fmt.Errorf("failed to parse status JSON: %w", err)
	}

	// Display overall status
	fmt.Println("Rewind Watch Status")
	fmt.Println("==================")

	if isRunning, ok := status["is_running"].(bool); ok && isRunning {
		fmt.Println("Status: RUNNING")
	} else {
		fmt.Println("Status: STOPPED")
	}

	if totalWatches, ok := status["total_watches"].(float64); ok {
		fmt.Printf("Active Watches: %.0f\n", totalWatches)
	}

	if totalDirs, ok := status["total_watched_dirs"].(float64); ok {
		fmt.Printf("Watched Directories: %.0f\n", totalDirs)
	}

	if uptime, ok := status["uptime_duration"].(string); ok && uptime != "" {
		fmt.Printf("Uptime: %s\n", uptime)
	}

	// Display event channel info
	if channelSize, ok := status["event_channel_size"].(float64); ok {
		if channelCap, ok := status["event_channel_capacity"].(float64); ok {
			fmt.Printf("Event Channel: %.0f/%.0f\n", channelSize, channelCap)
		}
	}

	// Display watch details if available
	if watchDetails, ok := status["watch_details"].([]interface{}); ok && len(watchDetails) > 0 {
		fmt.Println("\nWatch Details")
		fmt.Println("=============")
		
		for i, detail := range watchDetails {
			if watchMap, ok := detail.(map[string]interface{}); ok {
				path := getString(watchMap, "path")
				dirCount := getFloat(watchMap, "dir_count")
				ignoreCount := getFloat(watchMap, "ignore_count")

				fmt.Printf("%d. %s\n", i+1, path)
				fmt.Printf("   Directories: %.0f\n", dirCount)
				fmt.Printf("   Ignore Patterns: %.0f\n", ignoreCount)

				// Show some watch directories if available
				if watchDirs, ok := watchMap["watch_dirs"].([]interface{}); ok && len(watchDirs) > 0 {
					fmt.Printf("   Sample Dirs: ")
					count := 0
					for _, dir := range watchDirs {
						if dirStr, ok := dir.(string); ok {
							if count > 0 {
								fmt.Print(", ")
							}
							// Show relative path or basename for readability
							if strings.HasPrefix(dirStr, path) {
								rel := strings.TrimPrefix(dirStr, path)
								if rel == "" {
									rel = "."
								} else {
									rel = strings.TrimPrefix(rel, "/")
								}
								fmt.Print(rel)
							} else {
								fmt.Print(dirStr)
							}
							count++
							if count >= 3 { // Limit to first 3 directories
								if len(watchDirs) > 3 {
									fmt.Printf(" (and %d more)", len(watchDirs)-3)
								}
								break
							}
						}
					}
					fmt.Println()
				}
				fmt.Println()
			}
		}
	}

	return nil
}

// Helper functions for safe type assertions
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0
}

func init() {
	rootCmd.AddCommand(statusCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// statusCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// statusCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
