package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/davenicholson-xyz/rewind/app"
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
	app.Logger.Debug("Listing all versioned files")

	versionedFiles, err := dbm.GetAllVersionedFiles()
	if err != nil {
		return fmt.Errorf("Could not retrieve file list from db: %w", err)
	}

	// TODO: Implement query to get all unique file paths from versions table
	// Query: SELECT DISTINCT file_path FROM versions ORDER BY file_path

	fmt.Println("📁 Files with available versions:")
	fmt.Printf("%+v", versionedFiles)

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
	app.Logger.WithFields(map[string]interface{}{
		"filePath": filePath,
		"version":  version,
	}).Info("Performing rollback")

	// Convert to absolute path if relative
	// absPath, err := filepath.Abs(filePath)
	_, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// TODO: Implement the following steps:
	// 1. Query database to get the version record
	//    Query: SELECT storage_path, file_hash, file_size FROM versions
	//           WHERE file_path = ? AND version_number = ?

	// 2. Check if version exists
	// 3. Verify stored file exists in .rewind/versions/
	// 4. Create backup of current file (optional)
	// 5. Copy stored version back to original location
	// 6. Verify integrity using file hash

	fmt.Printf("🔄 Rolling back %s to version %d\n", filePath, version)
	fmt.Println("(Implementation needed: restore file from storage)")

	return nil
}

// Helper functions you might need:

// getFileVersionRecord retrieves a specific version record from database
func getFileVersionRecord(dbm *app.DatabaseManager, filePath string, version int) (*app.FileVersion, error) {
	// TODO: Implement database query to get specific version
	return nil, fmt.Errorf("not implemented")
}

// copyStoredFileToOriginal copies a file from storage back to its original location
func copyStoredFileToOriginal(storagePath, originalPath string) error {
	// TODO: Implement file copy operation
	return fmt.Errorf("not implemented")
}

// verifyFileIntegrity checks if restored file matches expected hash
func verifyFileIntegrity(filePath, expectedHash string) error {
	// TODO: Calculate current file hash and compare with expected
	return fmt.Errorf("not implemented")
}

// createBackup creates a backup of the current file before rollback
func createBackup(filePath string) (string, error) {
	// TODO: Create timestamped backup of current file
	return "", fmt.Errorf("not implemented")
}
