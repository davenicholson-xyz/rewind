package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
  rewind rollback                    # List all files with versions
  rewind rollback myfile.txt         # Show versions for myfile.txt
  rewind rollback myfile.txt -v 3    # Restore myfile.txt to version 3`,
	Run: runRollback,
}

var versionFlag int

func init() {
	rootCmd.AddCommand(rollbackCmd)
	rollbackCmd.Flags().IntVarP(&versionFlag, "version", "v", 0, "Version to rollback to")
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
		// No file specified - list all files with versions
		if err := listAllVersionedFiles(dbm); err != nil {
			fmt.Printf("Error listing files: %v\n", err)
			os.Exit(1)
		}
	case 1:
		// File specified
		filePath := args[0]
		if versionFlag == 0 {
			// No version specified - show all versions for this file
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

// listAllVersionedFiles lists all files that have versions in the database
func listAllVersionedFiles(dbm *app.DatabaseManager) error {

	files, err := dbm.GetAllLatestFiles()
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No versioned files found.")
		return nil
	}

	// return nil
	// app.Logger.Debug("Listing all versioned files")

	// versionedFiles, err := dbm.GetAllVersionedFiles()
	// if err != nil {
	// 	return fmt.Errorf("Could not retrieve file list from db: %w", err)
	// }
	//
	fmt.Println("📁 Files with available versions:")
	// fmt.Printf("%+v", versionedFiles)

	for _, ver := range files {
		fmt.Printf("%+v\n", ver)
	}

	return nil
}

// listFileVersions shows all versions available for a specific file
func listFileVersions(dbm *app.DatabaseManager, filePath string, rootDir string) error {
	app.Logger.WithField("filePath", filePath).Debug("Listing versions for file")

	// Convert to absolute path if relative
	// absPath, err := filepath.Abs(filePath)
	_, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// TODO: Implement query to get all versions for this file
	// Query: SELECT version_number, timestamp, file_size, file_hash FROM versions
	//        WHERE file_path = ? ORDER BY version_number DESC

	fmt.Printf("📋 Versions for %s:\n", filePath)
	fmt.Println("(Implementation needed: query database for file versions)")

	return nil
}

// performRollback restores a file to a specific version
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
