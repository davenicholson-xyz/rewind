package cmd

import (
	"bufio"
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

var confirmFlag bool

// restoreCmd represents the restore command
var restoreCmd = &cobra.Command{
	Use:   "restore [file_path]",
	Short: "Restore deleted files",
	Long: `Restore files that have been deleted and are tracked in the database.

When called without arguments, lists all deleted files for selection.
When called with a file path, restores that specific deleted file.

Examples:
  rewind restore                         # List all deleted files for selection
  rewind restore src/deleted.go          # Restore specific deleted file
  rewind restore --confirm               # List deleted files with confirmation prompts
  rewind restore src/deleted.go --confirm # Restore with confirmation`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runRestore(args); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().BoolVarP(&confirmFlag, "confirm", "c", false, "Prompt for confirmation before restoring files")
}

func runRestore(args []string) error {
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

	// If file path provided, restore that specific file
	if len(args) == 1 {
		return restoreSpecificFile(db, args[0])
	}

	// Otherwise, list deleted files for selection
	return listAndSelectDeletedFile(db)
}

func restoreSpecificFile(db *database.DatabaseManager, filePath string) error {
	// Convert to absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Restore the file in database
	fileVersion, err := db.RestoreFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to restore file: %w", err)
	}

	// Copy the file from storage back to original location
	if err := copyFromStorage(fileVersion, absPath); err != nil {
		return fmt.Errorf("failed to copy file from storage: %w", err)
	}

	fmt.Printf("Successfully restored: %s (version %d)\n", filePath, fileVersion.VersionNumber)
	return nil
}

func listAndSelectDeletedFile(db *database.DatabaseManager) error {
	// Get all deleted files
	deletedFiles, err := db.GetAllDeletedFiles()
	if err != nil {
		return fmt.Errorf("failed to get deleted files: %w", err)
	}

	if len(deletedFiles) == 0 {
		fmt.Println("No deleted files found.")
		return nil
	}

	// Display deleted files in a table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tFile Path\tVersion\tDeleted\tSize")
	fmt.Fprintln(w, "--\t---------\t-------\t-------\t----")

	for i, fv := range deletedFiles {
		fmt.Fprintf(w, "%d\t%s\tv%d\t%s\t%s\n",
			i+1,
			fv.FilePath,
			fv.VersionNumber,
			fv.Timestamp.Format("2006-01-02 15:04:05"),
			humanize.Bytes(uint64(fv.FileSize)))
	}
	w.Flush()

	// Prompt for selection
	fmt.Printf("\nEnter the ID of the file to restore (1-%d), or 'q' to quit: ", len(deletedFiles))
	
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if strings.ToLower(input) == "q" {
		fmt.Println("Restore cancelled.")
		return nil
	}

	// Parse selection
	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(deletedFiles) {
		return fmt.Errorf("invalid selection: %s", input)
	}

	selectedFile := deletedFiles[selection-1]
	
	// Confirm restoration only if --confirm flag is set
	if confirmFlag {
		fmt.Printf("Restore %s (version %d)? [y/N]: ", selectedFile.FilePath, selectedFile.VersionNumber)
		
		confirm, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
			fmt.Println("Restore cancelled.")
			return nil
		}
	}

	// Perform restoration
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	originalPath := filepath.Join(wd, selectedFile.FilePath)
	
	// Restore the file in database
	fileVersion, err := db.RestoreFile(originalPath)
	if err != nil {
		return fmt.Errorf("failed to restore file: %w", err)
	}

	// Copy the file from storage back to original location
	if err := copyFromStorage(fileVersion, originalPath); err != nil {
		return fmt.Errorf("failed to copy file from storage: %w", err)
	}

	fmt.Printf("Successfully restored: %s (version %d)\n", selectedFile.FilePath, fileVersion.VersionNumber)
	return nil
}

func copyFromStorage(fv *database.FileVersion, targetPath string) error {
	// Ensure target directory exists
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Open source file from storage
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	
	storagePath := filepath.Join(wd, ".rewind", "versions", fv.StoragePath)
	srcFile, err := os.Open(storagePath)
	if err != nil {
		return fmt.Errorf("failed to open storage file: %w", err)
	}
	defer srcFile.Close()

	// Create target file
	dstFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer dstFile.Close()

	// Copy content
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}
