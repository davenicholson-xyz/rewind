package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"

	"github.com/davenicholson-xyz/rewind/internal/database"
	"github.com/spf13/cobra"
)

var diffVersionFlag int
var noColorFlag bool

// diffCmd represents the diff command
var diffCmd = &cobra.Command{
	Use:   "diff <file_path> [--version <version_number>]",
	Short: "Show colored diff between current file and a previous version",
	Long: `Show a colored diff between the current file and a previous version.

By default, compares the current file with the previous version (latest - 1).
Use --version to specify a different version to compare against.

Examples:
  rewind diff src/main.go                    # Compare current with previous version
  rewind diff src/main.go --version 3        # Compare current with version 3
  rewind diff src/main.go --version 3 --no-color # Plain diff output`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDiff(args[0]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
	
	diffCmd.Flags().IntVarP(&diffVersionFlag, "version", "v", 0, "Version to compare against (default: previous version)")
	diffCmd.Flags().BoolVarP(&noColorFlag, "no-color", "n", false, "Disable colored output")
}

func runDiff(filePath string) error {
	// Get absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if current file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", filePath)
	}

	// Initialize database
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	db, err := database.NewDatabaseManager(wd)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	if err := db.Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Get the version to compare against
	var compareVersion *database.FileVersion
	if diffVersionFlag > 0 {
		// Use specified version
		compareVersion, err = db.GetFileVersion(absPath, diffVersionFlag)
		if err != nil {
			return fmt.Errorf("failed to get version %d: %w", diffVersionFlag, err)
		}
		if compareVersion == nil {
			return fmt.Errorf("version %d not found", diffVersionFlag)
		}
	} else {
		// Get previous version (latest - 1)
		compareVersion, err = getPreviousVersion(db, absPath)
		if err != nil {
			return err
		}
	}

	// Check if compare version is deleted
	if compareVersion.Deleted {
		return fmt.Errorf("cannot diff against deleted version %d", compareVersion.VersionNumber)
	}

	// Read current file content
	currentContent, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read current file: %w", err)
	}

	// Read compare version content
	compareContent, err := readVersionContent(wd, compareVersion)
	if err != nil {
		return fmt.Errorf("failed to read version %d content: %w", compareVersion.VersionNumber, err)
	}

	// Generate and display diff
	return displayDiff(filePath, string(compareContent), string(currentContent), compareVersion.VersionNumber)
}

func getPreviousVersion(db *database.DatabaseManager, filePath string) (*database.FileVersion, error) {
	// Get all versions for the file
	versions, err := db.GetFileVersions(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file versions: %w", err)
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found for file")
	}

	// Filter out deleted versions
	activeVersions := make([]*database.FileVersion, 0)
	for _, version := range versions {
		if !version.Deleted {
			activeVersions = append(activeVersions, version)
		}
	}

	if len(activeVersions) < 2 {
		return nil, fmt.Errorf("need at least 2 versions to show diff (found %d active versions)", len(activeVersions))
	}

	// Return the second most recent version (previous version)
	return activeVersions[1], nil
}

func readVersionContent(rootDir string, version *database.FileVersion) ([]byte, error) {
	storagePath := filepath.Join(rootDir, ".rewind", "versions", version.StoragePath)
	return os.ReadFile(storagePath)
}

func displayDiff(filename, oldContent, newContent string, oldVersion int) error {
	// Generate unified diff
	edits := myers.ComputeEdits(span.URIFromPath(""), oldContent, newContent)
	unified := gotextdiff.ToUnified(fmt.Sprintf("version %d", oldVersion), "current", oldContent, edits)
	diffText := fmt.Sprint(unified)

	if noColorFlag {
		// Output plain diff
		fmt.Print(diffText)
		return nil
	}

	// Apply syntax highlighting to the diff
	return displayColoredDiff(diffText, filename)
}

func displayColoredDiff(diffText, filename string) error {
	// ANSI color codes
	const (
		reset     = "\033[0m"
		red       = "\033[31m"
		green     = "\033[32m"
		cyan      = "\033[36m"
		gray      = "\033[90m"
		bold      = "\033[1m"
	)

	scanner := bufio.NewScanner(strings.NewReader(diffText))
	
	for scanner.Scan() {
		line := scanner.Text()
		
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			// File headers in cyan
			fmt.Printf("%s%s%s%s\n", cyan, bold, line, reset)
		} else if strings.HasPrefix(line, "@@") {
			// Hunk headers in gray
			fmt.Printf("%s%s%s\n", gray, line, reset)
		} else if strings.HasPrefix(line, "+") {
			// Added lines in green
			fmt.Printf("%s%s%s\n", green, line, reset)
		} else if strings.HasPrefix(line, "-") {
			// Removed lines in red
			fmt.Printf("%s%s%s\n", red, line, reset)
		} else {
			// Context lines (unchanged)
			fmt.Println(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading diff: %w", err)
	}

	return nil
}
