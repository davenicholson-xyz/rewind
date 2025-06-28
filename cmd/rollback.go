package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// rollbackCmd represents the rollback command
var rollbackCmd = &cobra.Command{
	Use:   "rollback [file]",
	Short: "Rollback files to previous versions",
	Long: `Rollback files to previous versions stored by rewind.

If no file is specified, lists all files that have versions available.
If a file is specified without a version, shows all available versions for that file.
If a file and version are specified, restores the file to that version.

Examples:
  rewind rollback myfile.txt         # Show versions for myfile.txt as a table
  rewind rollback myfile.txt -c      # List versions for myfile.txt as CSV
	rewind rollback myfile.txt -
  rewind rollback myfile.txt -v 3    # Restore myfile.txt to version 3`,
	Run: runRollback,
}

var versionFlag int
var csvFlag bool
var plainFlag bool

func init() {
	rootCmd.AddCommand(rollbackCmd)
	rollbackCmd.Flags().IntVarP(&versionFlag, "version", "v", 0, "Version to rollback to")
	rollbackCmd.Flags().BoolVarP(&csvFlag, "csv", "c", false, "List file versions as CSV")
	rollbackCmd.Flags().BoolVarP(&plainFlag, "plain", "p", false, "List file versions as plain text")
}

func runRollback(cmd *cobra.Command, args []string) {
	app.Logger.Info("Starting rollback command")

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	// Check if we're in a rewind project
	if err := checkRewindProject(cwd); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Initialize database manager
	dbm, err := app.NewDatabaseManager(cwd)
	if err != nil {
		fmt.Printf("Error creating database manager: %v\n", err)
		os.Exit(1)
	}
	defer dbm.Close()

	if err := dbm.Connect(); err != nil {
		fmt.Printf("Error connecting to database: %v\n", err)
		os.Exit(1)
	}

	// Handle different scenarios based on arguments
	switch len(args) {
	case 0:
		fmt.Println("Error: Please specify a file")
		os.Exit(1)
	case 1:
		// File specified
		filePath := args[0]

		if csvFlag {
			if err := listFileVersionsCSV(dbm, filePath); err != nil {
				fmt.Printf("Error listing file versions: %v\n", err)
				os.Exit(1)
			}
		} else if plainFlag {
			if err := listFileVersionsPlain(dbm, filePath); err != nil {
				fmt.Printf("Error listing file versions: %v\n", err)
				os.Exit(1)
			}
		} else if versionFlag == 0 {
			// No version specified - show interactive UI
			if err := listFileVersions(dbm, filePath, cwd); err != nil {
				fmt.Printf("Error listing file versions: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Version specified - perform rollback
			if err := performRollback(dbm, filePath, versionFlag, cwd); err != nil {
				fmt.Printf("Error performing rollback: %v\n", err)
				os.Exit(1)
			}
		}
	default:
		fmt.Println("Error: Too many arguments. Please specify at most one file.")
		os.Exit(1)
	}
}

func listFileVersionsTable(dbm *app.DatabaseManager, filePath string, rootDir string) error {
	app.Logger.WithField("filePath", filePath).Debug("Listing versions for file as table")
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	fileVersions, err := dbm.GetFileVersions(absPath)
	if err != nil {
		return fmt.Errorf("failed to get db entry: %w", err)
	}
	if len(fileVersions) == 0 {
		fmt.Println("No versions found for file:", filePath)
		return nil
	}
	app.Logger.WithField("version count", len(fileVersions)).Info("Found file versions in db")

	// Create a new tabwriter
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print header
	fmt.Fprintln(w, "VERSION\tTIME\tHASH\tSIZE")
	fmt.Fprintln(w, "-------\t----\t----\t----")

	// Print each version
	for _, fv := range fileVersions {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n",
			fv.VersionNumber,
			app.TimeAgo(fv.Timestamp),
			fv.FileHash[:6],
			app.BytesToHuman(fv.FileSize))
	}

	// Flush the writer to ensure all data is written
	return w.Flush()
}

func listFileVersionsPlain(dbm *app.DatabaseManager, filePath string) error {
	app.Logger.WithField("filePath", filePath).Debug("Listing versions for file as CSV")

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	fileVersions, err := dbm.GetFileVersions(absPath)
	if err != nil {
		return fmt.Errorf("failed to get db entry: %w", err)
	}

	if len(fileVersions) == 0 {
		fmt.Println("No versions found for file:", filePath)
		return nil
	}

	app.Logger.WithField("version count", len(fileVersions)).Info("Found file versions in db")

	// Print CSV header
	// fmt.Println("version,timestamp,filehash,filesize,storagepath")

	// Print each version as CSV
	for _, fv := range fileVersions {
		fmt.Printf("%d %s %s %s %s\n",
			fv.VersionNumber,
			app.TimeAgo(fv.Timestamp),
			fv.FileHash,
			app.BytesToHuman(fv.FileSize),
			fv.StoragePath)
	}

	return nil
}

// listFileVersionsCSV outputs file versions in CSV format
func listFileVersionsCSV(dbm *app.DatabaseManager, filePath string) error {
	app.Logger.WithField("filePath", filePath).Debug("Listing versions for file as CSV")

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	fileVersions, err := dbm.GetFileVersions(absPath)
	if err != nil {
		return fmt.Errorf("failed to get db entry: %w", err)
	}

	if len(fileVersions) == 0 {
		fmt.Println("No versions found for file:", filePath)
		return nil
	}

	app.Logger.WithField("version count", len(fileVersions)).Info("Found file versions in db")

	// Print CSV header
	// fmt.Println("version,timestamp,filehash,filesize,storagepath")

	// Print each version as CSV
	for _, fv := range fileVersions {
		fmt.Printf("%d,%s,%s,%d,%s\n",
			fv.VersionNumber,
			fv.Timestamp.Format("2006-01-02T15:04:05Z07:00"), // ISO 8601 format
			fv.FileHash,
			fv.FileSize,
			fv.StoragePath)
	}

	return nil
}

// listFileVersions shows interactive UI for file versions
func listFileVersions(dbm *app.DatabaseManager, filePath string, rootDir string) error {
	app.Logger.WithField("filePath", filePath).Debug("Listing versions for file")

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	fileVersions, err := dbm.GetFileVersions(absPath)
	if err != nil {
		return fmt.Errorf("failed to get db entry: %w", err)
	}

	if len(fileVersions) == 0 {
		fmt.Println("No versions found for file:", filePath)
		return nil
	}

	app.Logger.WithField("version count", len(fileVersions)).Info("Found file version in db")

	listFileVersionsTable(dbm, absPath, rootDir)

	return nil
}

func performRollback(dbm *app.DatabaseManager, filePath string, version int, rootDir string) error {
	app.Logger.WithFields(map[string]any{
		"filePath": filePath,
		"version":  version,
	}).Info("Performing rollback")

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	fileVersion, err := dbm.GetFileVersion(absPath, version)
	if err != nil {
		return fmt.Errorf("failed to get db entry: %w", err)
	}

	if fileVersion == nil {
		return fmt.Errorf("version %d not found for file %s", version, filePath)
	}

	app.Logger.WithFields(logrus.Fields{"id": fileVersion.ID, "filePath": fileVersion.FilePath}).Info("Found file version in db")

	storedVersionPath := filepath.Join(rootDir, ".rewind", "versions", fileVersion.StoragePath)

	app.Logger.WithFields(map[string]any{
		"originalPath": absPath,
		"storedPath":   storedVersionPath,
		"version":      version,
	}).Debug("Paths for rollback")

	if _, err := os.Stat(storedVersionPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("stored version file not found at %s", storedVersionPath)
		}
		return fmt.Errorf("failed to access stored version file: %w", err)
	}

	if err := copyStoredFileToOriginal(storedVersionPath, absPath); err != nil {
		return fmt.Errorf("failed to restore file: %w", err)
	}

	if err := verifyFileIntegrity(absPath, fileVersion.FileHash); err != nil {
		return fmt.Errorf("file integrity check failed after restore: %w", err)
	}

	fmt.Printf("✅ Successfully rolled back %s to version %d\n", filePath, version)

	return nil
}

func copyStoredFileToOriginal(storagePath, originalPath string) error {
	sourceFile, err := os.Open(storagePath)
	if err != nil {
		return fmt.Errorf("failed to open stored file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(originalPath)
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

func verifyFileIntegrity(filePath, expectedHash string) error {
	actualHash, err := app.CalculateFileHash(filePath)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	if actualHash != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

func createBackup(filePath string) (string, error) {
	// Check if original file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", nil // No file to backup
	}

	// Create backup with timestamp
	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.backup_%s", filePath, timestamp)

	sourceFile, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	backupFile, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer backupFile.Close()

	if _, err := io.Copy(backupFile, sourceFile); err != nil {
		return "", fmt.Errorf("failed to copy file for backup: %w", err)
	}

	return backupPath, nil
}
