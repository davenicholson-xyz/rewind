package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davenicholson-xyz/rewind/internal/database"
	"github.com/spf13/cobra"
)

// purgeCmd represents the purge command
var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Remove old versions to free up space",
	Long: `Remove old versions from the .rewind directory to free up space.
Uses either keep-last or older-than strategy to determine which versions to remove.
Tagged versions are always preserved.

Examples:
  rewind purge --keep-last 10        # Keep last 10 versions per file
  rewind purge --older-than 7d       # Remove versions older than 7 days
  rewind purge --older-than 2w       # Remove versions older than 2 weeks  
  rewind purge --older-than 1h       # Remove versions older than 1 hour
  rewind purge --max-size 1GB        # Keep total size under 1GB
  rewind purge --max-size 500MB      # Keep total size under 500MB
  rewind purge --dry-run --keep-last 3  # Show what would be removed`,
	Run: func(cmd *cobra.Command, args []string) {
		keepLast, _ := cmd.Flags().GetInt("keep-last")
		olderThan, _ := cmd.Flags().GetString("older-than")
		maxSize, _ := cmd.Flags().GetString("max-size")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		force, _ := cmd.Flags().GetBool("force")
		
		if err := runPurge(keepLast, olderThan, maxSize, dryRun, force); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func runPurge(keepLast int, olderThan string, maxSize string, dryRun bool, force bool) error {
	// Count how many strategies are specified
	strategyCount := 0
	if keepLast > 0 {
		strategyCount++
	}
	if olderThan != "" {
		strategyCount++
	}
	if maxSize != "" {
		strategyCount++
	}
	
	// Validate that exactly one strategy is specified
	if strategyCount == 0 {
		return fmt.Errorf("must specify one of --keep-last, --older-than, or --max-size")
	}
	if strategyCount > 1 {
		return fmt.Errorf("can only specify one of --keep-last, --older-than, or --max-size")
	}

	// Find .rewind directory
	rewindDir, err := findRewindDirectory()
	if err != nil {
		return fmt.Errorf("not in a rewind project: %w", err)
	}

	// Get project root (parent of .rewind)
	projectRoot := filepath.Dir(rewindDir)

	// Initialize database manager
	dbManager, err := database.NewDatabaseManager(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	
	// Initialize database connection
	if err := dbManager.InitDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer dbManager.Close()

	// Get versions to purge based on strategy
	var versionIDs []int64
	var strategy string

	if keepLast > 0 {
		if keepLast < 1 {
			return fmt.Errorf("keep-last must be at least 1")
		}
		versionIDs, err = dbManager.GetVersionsForPurge(keepLast)
		if err != nil {
			return fmt.Errorf("failed to get versions for purge: %w", err)
		}
		strategy = fmt.Sprintf("keeping last %d per file", keepLast)
	} else if olderThan != "" {
		// Parse older-than duration
		duration, err := parseDuration(olderThan)
		if err != nil {
			return fmt.Errorf("invalid older-than duration: %w", err)
		}
		
		cutoffTime := time.Now().Add(-duration)
		versionIDs, err = dbManager.GetVersionsForPurgeByAge(cutoffTime)
		if err != nil {
			return fmt.Errorf("failed to get versions for purge by age: %w", err)
		}
		strategy = fmt.Sprintf("older than %s", olderThan)
	} else if maxSize != "" {
		// Parse max-size limit
		sizeLimit, err := parseSize(maxSize)
		if err != nil {
			return fmt.Errorf("invalid max-size: %w", err)
		}
		
		versionIDs, err = dbManager.GetVersionsForPurgeBySize(sizeLimit)
		if err != nil {
			return fmt.Errorf("failed to get versions for purge by size: %w", err)
		}
		strategy = fmt.Sprintf("keeping total size under %s", maxSize)
	}

	if len(versionIDs) == 0 {
		fmt.Println("No versions to purge.")
		return nil
	}

	// Show what will be removed
	fmt.Printf("Found %d versions to purge (%s, preserving tagged versions)\n", 
		len(versionIDs), strategy)

	if dryRun {
		fmt.Println("Dry run - no files will be deleted")
		return nil
	}

	// Confirm deletion unless force flag is used
	if !force {
		fmt.Print("Continue with purge? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Purge cancelled")
			return nil
		}
	}

	// Execute purge
	if err := dbManager.RemoveVersions(versionIDs); err != nil {
		return fmt.Errorf("failed to remove versions: %w", err)
	}

	fmt.Printf("Successfully purged %d versions\n", len(versionIDs))
	return nil
}

// parseDuration parses duration strings like "7d", "2w", "1h", "30m"
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration format")
	}
	
	unit := s[len(s)-1:]
	valueStr := s[:len(s)-1]
	
	var value int
	if _, err := fmt.Sscanf(valueStr, "%d", &value); err != nil {
		return 0, fmt.Errorf("invalid duration value: %w", err)
	}
	
	switch unit {
	case "s":
		return time.Duration(value) * time.Second, nil
	case "m":
		return time.Duration(value) * time.Minute, nil
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "w":
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid duration unit: %s (use s, m, h, d, or w)", unit)
	}
}

// parseSize parses size strings like "1GB", "500MB", "2TB"
func parseSize(s string) (int64, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid size format")
	}
	
	// Handle different unit lengths (B, KB, MB, GB, TB)
	var unit string
	var valueStr string
	
	if len(s) >= 3 && s[len(s)-2:] == "KB" {
		unit = "KB"
		valueStr = s[:len(s)-2]
	} else if len(s) >= 3 && s[len(s)-2:] == "MB" {
		unit = "MB"
		valueStr = s[:len(s)-2]
	} else if len(s) >= 3 && s[len(s)-2:] == "GB" {
		unit = "GB"
		valueStr = s[:len(s)-2]
	} else if len(s) >= 3 && s[len(s)-2:] == "TB" {
		unit = "TB"
		valueStr = s[:len(s)-2]
	} else if len(s) >= 2 && s[len(s)-1:] == "B" {
		// Check if the part before B contains only digits/decimal
		valueStr = s[:len(s)-1]
		var testValue float64
		n, err := fmt.Sscanf(valueStr, "%f", &testValue)
		if err != nil || n != 1 {
			return 0, fmt.Errorf("invalid size unit: %s (use B, KB, MB, GB, or TB)", s)
		}
		// Additional check: make sure the entire string was consumed
		testStr := fmt.Sprintf("%g", testValue)
		if testStr != valueStr && fmt.Sprintf("%.0f", testValue) != valueStr {
			return 0, fmt.Errorf("invalid size unit: %s (use B, KB, MB, GB, or TB)", s)
		}
		unit = "B"
	} else {
		return 0, fmt.Errorf("invalid size unit (use B, KB, MB, GB, or TB)")
	}
	
	var value float64
	if _, err := fmt.Sscanf(valueStr, "%f", &value); err != nil {
		return 0, fmt.Errorf("invalid size value: %w", err)
	}
	
	switch unit {
	case "B":
		return int64(value), nil
	case "KB":
		return int64(value * 1024), nil
	case "MB":
		return int64(value * 1024 * 1024), nil
	case "GB":
		return int64(value * 1024 * 1024 * 1024), nil
	case "TB":
		return int64(value * 1024 * 1024 * 1024 * 1024), nil
	default:
		return 0, fmt.Errorf("invalid size unit: %s (use B, KB, MB, GB, or TB)", unit)
	}
}

func findRewindDirectory() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	dir := currentDir
	for {
		rewindDir := filepath.Join(dir, ".rewind")
		if info, err := os.Stat(rewindDir); err == nil && info.IsDir() {
			return rewindDir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf(".rewind directory not found")
}

func init() {
	rootCmd.AddCommand(purgeCmd)

	purgeCmd.Flags().IntP("keep-last", "k", 0, "Number of versions to keep per file")
	purgeCmd.Flags().StringP("older-than", "t", "", "Remove versions older than specified duration (e.g., 7d, 2w, 1h)")
	purgeCmd.Flags().StringP("max-size", "s", "", "Keep total size under specified limit (e.g., 1GB, 500MB)")
	purgeCmd.Flags().BoolP("dry-run", "n", false, "Show what would be removed without actually deleting")
	purgeCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
}
