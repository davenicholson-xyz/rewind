package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/davenicholson-xyz/rewind/internal/database"
	"github.com/spf13/cobra"
)

// tagCmd represents the tag command
var tagCmd = &cobra.Command{
	Use:   "tag <file_path> <tag_name> [--version <version_number>]",
	Short: "Add a tag to a file version",
	Long: `Add a descriptive tag to a specific file version to make it easier to find later.

By default, tags the latest version of the file. Use --version to tag a specific version.

Examples:
  rewind tag src/main.go "stable-release"           # Tag latest version
  rewind tag src/main.go "feature-complete" --version 5  # Tag version 5`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runTag(args[0], args[1]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var tagVersionFlag int

func init() {
	rootCmd.AddCommand(tagCmd)
	tagCmd.Flags().IntVarP(&tagVersionFlag, "version", "v", 0, "Version number to tag (defaults to latest)")
}

func runTag(filePath, tagName string) error {
	// Validate tag name
	if err := validateTagName(tagName); err != nil {
		return err
	}

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

	// Determine which version to tag
	var targetVersion int
	if tagVersionFlag > 0 {
		targetVersion = tagVersionFlag
	} else {
		// Get the latest version
		latestVersion, err := db.GetLatestFileVersion(absPath)
		if err != nil {
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		if latestVersion == nil {
			return fmt.Errorf("no versions found for file: %s", filePath)
		}
		targetVersion = latestVersion.VersionNumber
	}

	// Add the tag
	if err := db.AddTag(absPath, targetVersion, tagName); err != nil {
		return err
	}

	fmt.Printf("✓ Tagged version %d of %s as '%s'\n", targetVersion, filepath.Base(filePath), tagName)
	return nil
}

func validateTagName(tagName string) error {
	// Check if empty
	if strings.TrimSpace(tagName) == "" {
		return fmt.Errorf("tag name cannot be empty")
	}

	// Check length (reasonable limit)
	if len(tagName) > 100 {
		return fmt.Errorf("tag name too long (max 100 characters)")
	}

	// Check for problematic characters (optional - be permissive for now)
	if strings.Contains(tagName, "\n") || strings.Contains(tagName, "\r") {
		return fmt.Errorf("tag name cannot contain newlines")
	}

	return nil
}