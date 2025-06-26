package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// destroyCmd represents the destroy command
var destroyCmd = &cobra.Command{
	Use:   "destroy [directory]",
	Short: "Remove rewind project configuration",
	Long: `Remove rewind project configuration and cleanup associated files.

This command removes the .rewind directory and associated configuration files
from the specified directory or current directory.

Examples:
  rewind destroy
  rewind destroy .
  rewind destroy /path/to/project`,
	Run: runDestroy,
}

func init() {
	rootCmd.AddCommand(destroyCmd)
}

func runDestroy(cmd *cobra.Command, args []string) {
	targetDir, err := determineTargetDirectory(args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		fmt.Printf("Error getting absolute path: %v\n", err)
		os.Exit(1)
	}

	// Define files and directories to remove
	itemsToRemove := []string{
		filepath.Join(absTargetDir, ".rewind"),
		filepath.Join(absTargetDir, ".envrc"),
		filepath.Join(absTargetDir, ".rwignore"),
		// Add more files/directories as needed
		// filepath.Join(absTargetDir, "some-other-file"),
	}

	if err := removeRewindProject(itemsToRemove); err != nil {
		fmt.Printf("Error during cleanup: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully removed rewind project from %s\n", absTargetDir)
}

// removeRewindProject removes all specified files and directories
func removeRewindProject(itemsToRemove []string) error {
	var errors []error

	for _, item := range itemsToRemove {
		if err := removeItem(item); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to remove some items: %v", errors)
	}

	return nil
}

// removeItem removes a single file or directory
func removeItem(path string) error {
	// Check if the item exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Item doesn't exist, nothing to do
		return nil
	}

	// Remove the item (works for both files and directories)
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to remove %s: %w", path, err)
	}

	return nil
}

// Alternative approach with more control and confirmation
func removeRewindProjectWithConfirmation(itemsToRemove []string, force bool) error {
	if !force {
		fmt.Println("The following items will be removed:")
		for _, item := range itemsToRemove {
			if _, err := os.Stat(item); err == nil {
				fmt.Printf("  - %s\n", item)
			}
		}

		fmt.Print("Continue? (y/N): ")
		var response string
		fmt.Scanln(&response)

		if response != "y" && response != "Y" {
			return fmt.Errorf("operation cancelled")
		}
	}

	return removeRewindProject(itemsToRemove)
}

// Example with categorized removal (if you want to handle different types differently)
func removeRewindProjectCategorized(targetDir string) error {
	// Files to remove
	filesToRemove := []string{
		filepath.Join(targetDir, ".envrc"),
		filepath.Join(targetDir, "rewind.config"),
	}

	// Directories to remove
	dirsToRemove := []string{
		filepath.Join(targetDir, ".rewind"),
		filepath.Join(targetDir, "temp-rewind-data"),
	}

	// Remove files first
	for _, file := range filesToRemove {
		if err := removeFile(file); err != nil {
			return err
		}
	}

	// Remove directories
	for _, dir := range dirsToRemove {
		if err := removeDirectory(dir); err != nil {
			return err
		}
	}

	return nil
}

func removeFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to remove file %s: %w", path, err)
	}

	return nil
}

func removeDirectory(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to remove directory %s: %w", path, err)
	}

	return nil
}
