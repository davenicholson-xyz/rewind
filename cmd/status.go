// cmd/status.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show rewind daemon status and statistics",
	Long: `Show the current status of the rewind daemon including:
- Running status and PID
- Number of monitored files
- Recent activity
- Storage usage`,
	Run: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	if err := checkRewindProject(cwd); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("📊 Rewind Status")
	fmt.Println("================")

	// Check daemon status
	if isDaemonRunning(cwd) {
		pidFile := getPidFilePath(cwd)
		pidData, _ := os.ReadFile(pidFile)
		fmt.Printf("🟢 Daemon: Running (PID: %s)\n", string(pidData))

		// Show log file location
		logFile := filepath.Join(cwd, ".rewind", "logs", "daemon.log")
		if _, err := os.Stat(logFile); err == nil {
			fmt.Printf("📝 Logs: %s\n", logFile)

			// Show recent log entries
			showRecentLogs(logFile, 5)
		}
	} else {
		fmt.Println("🔴 Daemon: Not running")
	}

	// Database statistics
	showDatabaseStats(cwd)

	// Storage usage
	showStorageUsage(cwd)
}

func showRecentLogs(logFile string, lines int) {
	// This is a simplified version - you might want to use a proper tail implementation
	fmt.Printf("\n📋 Recent Activity (last %d entries):\n", lines)
	fmt.Println("---")

	// For now, just show that logs exist
	if info, err := os.Stat(logFile); err == nil {
		fmt.Printf("Log file size: %d bytes\n", info.Size())
		fmt.Printf("Last modified: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
	}
}

func showDatabaseStats(cwd string) {
	dbm, err := app.NewDatabaseManager(cwd)
	if err != nil {
		fmt.Printf("Error connecting to database: %v\n", err)
		return
	}
	defer dbm.Close()

	if err := dbm.Connect(); err != nil {
		fmt.Printf("Error connecting to database: %v\n", err)
		return
	}

	// Get all latest files
	files, err := dbm.GetAllLatestFiles()
	if err != nil {
		fmt.Printf("Error getting file stats: %v\n", err)
		return
	}

	fmt.Printf("\n📁 Files:\n")
	fmt.Printf("  Total tracked files: %d\n", len(files))

	if len(files) > 0 {
		// Calculate total size and find most recent
		var totalSize int64
		var mostRecent time.Time

		for _, file := range files {
			totalSize += file.FileSize
			if file.Timestamp.After(mostRecent) {
				mostRecent = file.Timestamp
			}
		}

		fmt.Printf("  Total size: %s\n", humanize.Bytes(uint64(totalSize)))
		fmt.Printf("  Last change: %s\n", humanize.Time(mostRecent))
	}
}

func showStorageUsage(cwd string) {
	versionsDir := filepath.Join(cwd, ".rewind", "versions")

	var totalSize int64
	var fileCount int

	filepath.Walk(versionsDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
			fileCount++
		}
		return nil
	})

	fmt.Printf("\n💾 Storage:\n")
	fmt.Printf("  Stored versions: %d files\n", fileCount)
	fmt.Printf("  Storage used: %s\n", humanize.Bytes(uint64(totalSize)))
	fmt.Printf("  Storage used: %s\n", humanize.Bytes(uint64(totalSize)))
	fmt.Printf("  Location: %s\n", versionsDir)
}
