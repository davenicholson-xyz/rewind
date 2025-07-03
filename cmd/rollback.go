package cmd

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
	"github.com/davenicholson-xyz/rewind/internal/database"
	"github.com/spf13/cobra"
)

// rollbackCmd represents the rollback command
var rollbackCmd = &cobra.Command{
	Use:   "rollback <file_path> [--version <version_number>]",
	Short: "View file versions or rollback to a specific version",
	Long: `View all versions of a file or rollback to a specific version.

When called with just a file path, displays a table of all versions.
When called with --version flag, rolls back the file to that version.

Examples:
  rewind rollback src/main.go                    # Show all versions
  rewind rollback src/main.go --version 3        # Rollback to version 3
  rewind rollback src/main.go --version 3 --confirm # Rollback with confirmation prompt`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runRollback(args[0]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var versionFlag int
var csvFlag bool
var jsonFlag bool
var rollbackConfirmFlag bool

func init() {
	rootCmd.AddCommand(rollbackCmd)

	rollbackCmd.Flags().IntVarP(&versionFlag, "version", "v", 0, "Version to rollback to")
	rollbackCmd.Flags().BoolVarP(&csvFlag, "csv", "c", false, "List file versions as CSV")
	rollbackCmd.Flags().BoolVarP(&jsonFlag, "json", "j", false, "List file versions as json")
	rollbackCmd.Flags().BoolVarP(&rollbackConfirmFlag, "confirm", "f", false, "Prompt for confirmation before rollback")
}

func runRollback(filePath string) error {
	// Get absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Find the rewind project root
	rewindRoot, err := findRewindRoot(absPath)
	if err != nil {
		return fmt.Errorf("not in a rewind project: %w", err)
	}

	// Connect to database
	db, err := database.NewDatabaseManager(rewindRoot)
	if err != nil {
		return fmt.Errorf("failed to create database manager: %w", err)
	}

	if err := db.Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// If version flag is set, perform rollback
	if versionFlag > 0 {
		return performRollback(db, absPath, versionFlag)
	}

	// Otherwise, display file versions
	return displayFileVersions(db, absPath)
}

func findRewindRoot(startPath string) (string, error) {
	currentPath := startPath
	if !filepath.IsAbs(currentPath) {
		var err error
		currentPath, err = filepath.Abs(currentPath)
		if err != nil {
			return "", err
		}
	}

	// If it's a file, start from its directory
	if info, err := os.Stat(currentPath); err == nil && !info.IsDir() {
		currentPath = filepath.Dir(currentPath)
	}

	for {
		rewindDir := filepath.Join(currentPath, ".rewind")
		if info, err := os.Stat(rewindDir); err == nil && info.IsDir() {
			return currentPath, nil
		}

		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			break // reached root
		}
		currentPath = parent
	}

	return "", fmt.Errorf("no .rewind directory found")
}

func displayFileVersions(db *database.DatabaseManager, filePath string) error {
	versions, err := db.GetFileVersions(filePath)
	if err != nil {
		return fmt.Errorf("failed to get file versions: %w", err)
	}

	if len(versions) == 0 {
		fmt.Printf("No versions found for file: %s\n", filePath)
		return nil
	}

	// Filter out deleted versions - only show active ones
	activeVersions := make([]*database.FileVersion, 0)
	for _, version := range versions {
		if !version.Deleted {
			activeVersions = append(activeVersions, version)
		}
	}

	if len(activeVersions) == 0 {
		fmt.Printf("No active versions found for file: %s\n", filePath)
		return nil
	}

	// Display in requested format
	if csvFlag {
		return displayAsCSV(activeVersions, filePath)
	}
	
	if jsonFlag {
		return displayAsJSON(activeVersions, filePath)
	}

	// Default to table format
	return displayAsTable(activeVersions, filePath)
}

func displayAsTable(versions []*database.FileVersion, filePath string) error {
	fmt.Printf("File versions for: %s\n\n", filePath)

	// Get current file size (latest version) for size difference calculation
	var currentSize int64
	if len(versions) > 0 {
		currentSize = versions[0].FileSize // versions are ordered by version number DESC
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tTIME\tSIZE\tSIZE DIFF\tHASH")
	fmt.Fprintln(w, "-------\t----\t----\t---------\t----")

	for _, version := range versions {
		// Format time as relative
		timeStr := humanize.Time(version.Timestamp)

		// Format size
		sizeStr := humanize.Bytes(uint64(version.FileSize))

		// Calculate and format size difference
		var sizeDiffStr string
		if version.FileSize == currentSize {
			sizeDiffStr = "(current)"
		} else {
			diff := currentSize - version.FileSize
			if diff > 0 {
				sizeDiffStr = "+" + humanize.Bytes(uint64(diff))
			} else {
				sizeDiffStr = "-" + humanize.Bytes(uint64(-diff))
			}
		}

		// Format hash (first 8 characters)
		hashStr := version.FileHash
		if len(hashStr) > 8 {
			hashStr = hashStr[:8] + "..."
		}

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
			version.VersionNumber,
			timeStr,
			sizeStr,
			sizeDiffStr,
			hashStr,
		)
	}

	return w.Flush()
}

func displayAsCSV(versions []*database.FileVersion, filePath string) error {
	// Get current file size for size difference calculation
	var currentSize int64
	if len(versions) > 0 {
		currentSize = versions[0].FileSize
	}

	writer := csv.NewWriter(os.Stdout)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"Version", "Timestamp", "Size", "SizeBytes", "SizeDiff", "SizeDiffBytes", "Hash", "FilePath"}); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, version := range versions {
		// Calculate size difference
		var sizeDiffStr string
		var sizeDiffBytes string
		if version.FileSize == currentSize {
			sizeDiffStr = "0"
			sizeDiffBytes = "0"
		} else {
			diff := currentSize - version.FileSize
			sizeDiffBytes = strconv.FormatInt(diff, 10)
			if diff > 0 {
				sizeDiffStr = "+" + humanize.Bytes(uint64(diff))
			} else {
				sizeDiffStr = "-" + humanize.Bytes(uint64(-diff))
			}
		}

		record := []string{
			strconv.Itoa(version.VersionNumber),
			version.Timestamp.Format("2006-01-02 15:04:05"),
			humanize.Bytes(uint64(version.FileSize)),
			strconv.FormatInt(version.FileSize, 10),
			sizeDiffStr,
			sizeDiffBytes,
			version.FileHash,
			filePath,
		}

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	return nil
}

type FileVersionJSON struct {
	Version       int    `json:"version"`
	Timestamp     string `json:"timestamp"`
	TimestampUnix int64  `json:"timestamp_unix"`
	Size          string `json:"size"`
	SizeBytes     int64  `json:"size_bytes"`
	SizeDiff      string `json:"size_diff"`
	SizeDiffBytes int64  `json:"size_diff_bytes"`
	Hash          string `json:"hash"`
	FilePath      string `json:"file_path"`
}

type FileVersionsResponse struct {
	FilePath string            `json:"file_path"`
	Count    int               `json:"count"`
	Versions []FileVersionJSON `json:"versions"`
}

func displayAsJSON(versions []*database.FileVersion, filePath string) error {
	// Get current file size for size difference calculation
	var currentSize int64
	if len(versions) > 0 {
		currentSize = versions[0].FileSize
	}

	jsonVersions := make([]FileVersionJSON, len(versions))
	for i, version := range versions {
		// Calculate size difference
		var sizeDiffStr string
		var sizeDiffBytes int64
		if version.FileSize == currentSize {
			sizeDiffStr = "0"
			sizeDiffBytes = 0
		} else {
			diff := currentSize - version.FileSize
			sizeDiffBytes = diff
			if diff > 0 {
				sizeDiffStr = "+" + humanize.Bytes(uint64(diff))
			} else {
				sizeDiffStr = "-" + humanize.Bytes(uint64(-diff))
			}
		}

		jsonVersions[i] = FileVersionJSON{
			Version:       version.VersionNumber,
			Timestamp:     version.Timestamp.Format("2006-01-02 15:04:05"),
			TimestampUnix: version.Timestamp.Unix(),
			Size:          humanize.Bytes(uint64(version.FileSize)),
			SizeBytes:     version.FileSize,
			SizeDiff:      sizeDiffStr,
			SizeDiffBytes: sizeDiffBytes,
			Hash:          version.FileHash,
			FilePath:      filePath,
		}
	}

	response := FileVersionsResponse{
		FilePath: filePath,
		Count:    len(jsonVersions),
		Versions: jsonVersions,
	}

	encoder := json.NewEncoder(os.Stdout)
	return encoder.Encode(response)
}

func performRollback(db *database.DatabaseManager, filePath string, targetVersion int) error {
	// Sanity check 1: Get target version from database
	targetVersionData, err := db.GetFileVersion(filePath, targetVersion)
	if err != nil {
		return fmt.Errorf("failed to get target version: %w", err)
	}
	if targetVersionData == nil {
		return fmt.Errorf("version %d not found for file", targetVersion)
	}

	// Sanity check 2: Ensure target version is not deleted
	if targetVersionData.Deleted {
		return fmt.Errorf("cannot rollback to deleted version %d", targetVersion)
	}

	// Sanity check 3: Get current latest version
	latestVersion, err := db.GetLatestFileVersion(filePath)
	if err != nil {
		return fmt.Errorf("failed to get latest version: %w", err)
	}
	if latestVersion == nil {
		return fmt.Errorf("no versions found for file")
	}

	// Sanity check 4: Check if we're already at the target version
	if latestVersion.VersionNumber == targetVersion {
		return fmt.Errorf("file is already at version %d", targetVersion)
	}

	// Sanity check 5: Check if current file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("current file does not exist: %s", filePath)
	}

	// Sanity check 6: Find the rewind root to get storage path
	rewindRoot, err := findRewindRoot(filePath)
	if err != nil {
		return fmt.Errorf("failed to find rewind root: %w", err)
	}

	// Sanity check 7: Check if stored version file exists
	storedVersionPath := filepath.Join(rewindRoot, ".rewind", "versions", targetVersionData.StoragePath)
	if _, err := os.Stat(storedVersionPath); os.IsNotExist(err) {
		return fmt.Errorf("stored version file not found: %s", storedVersionPath)
	}

	// Show confirmation prompt only if --confirm is used
	if rollbackConfirmFlag {
		if !confirmRollback(latestVersion, targetVersionData, filePath) {
			fmt.Println("Rollback cancelled.")
			return nil
		}
	}

	// Check if current file differs from latest database version and version it if needed
	currentHash, err := database.CalculateFileHash(filePath)
	if err != nil {
		return fmt.Errorf("failed to calculate current file hash: %w", err)
	}

	// If current file is different from latest version, save it first
	if currentHash != latestVersion.FileHash {
		fmt.Println("Current file differs from latest version, saving current state...")
		if err := saveCurrentFileAsNewVersion(db, filePath, rewindRoot); err != nil {
			return fmt.Errorf("failed to save current file state: %w", err)
		}
	}

	// Perform the rollback by copying the stored version
	if err := copyFile(storedVersionPath, filePath); err != nil {
		return fmt.Errorf("failed to restore file: %w", err)
	}

	fmt.Printf("✓ File restored to version %d\n", targetVersion)
	fmt.Printf("✓ Rollback completed successfully\n")
	return nil
}

func confirmRollback(currentVersion, targetVersion *database.FileVersion, filePath string) bool {
	fmt.Printf("Rolling back %s from version %d to version %d\n", 
		filepath.Base(filePath), currentVersion.VersionNumber, targetVersion.VersionNumber)
	fmt.Printf("  Current: %s (modified %s)\n", 
		humanize.Bytes(uint64(currentVersion.FileSize)), humanize.Time(currentVersion.Timestamp))
	fmt.Printf("  Target:  %s (modified %s)\n", 
		humanize.Bytes(uint64(targetVersion.FileSize)), humanize.Time(targetVersion.Timestamp))
	fmt.Printf("\nContinue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func saveCurrentFileAsNewVersion(db *database.DatabaseManager, filePath, rewindRoot string) error {
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat current file: %w", err)
	}

	// Calculate hash
	currentHash, err := database.CalculateFileHash(filePath)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	// Get next version number
	versionNumber, err := db.GetNextVersionNumber(filePath)
	if err != nil {
		return fmt.Errorf("failed to get next version number: %w", err)
	}

	// Create storage path
	storagePath := db.CreateStoragePath(filePath, versionNumber)
	fullStoragePath := filepath.Join(rewindRoot, ".rewind", "versions", storagePath)

	// Create storage directory if it doesn't exist
	storageDir := filepath.Dir(fullStoragePath)
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Copy current file to storage location
	if err := copyFile(filePath, fullStoragePath); err != nil {
		return fmt.Errorf("failed to copy file to storage: %w", err)
	}

	// Get relative path for database
	relPath, err := filepath.Rel(rewindRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	// Create file version record
	fileVersion := &database.FileVersion{
		FilePath:      relPath,
		VersionNumber: versionNumber,
		Timestamp:     fileInfo.ModTime(),
		FileHash:      currentHash,
		FileSize:      fileInfo.Size(),
		StoragePath:   storagePath,
		Deleted:       false,
	}

	// Add to database
	if err := db.AddFileVersion(fileVersion); err != nil {
		// Clean up the file if database insertion fails
		os.Remove(fullStoragePath)
		return fmt.Errorf("failed to add file version to database: %w", err)
	}

	fmt.Printf("✓ Current state saved as version %d\n", versionNumber)
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Sync to ensure data is written to disk
	if err := destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}
