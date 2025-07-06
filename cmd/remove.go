package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/spf13/cobra"
)

// removeCmd represents the remove command
var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a rewind project from monitoring",
	Long: `Remove a rewind project from monitoring and delete its .rewind directory.
This stops the daemon from monitoring the project and permanently removes all 
version history and configuration files.

The removal process:
- Notifies the daemon to stop monitoring the project
- Removes the .rewind directory and all version files
- Requires confirmation unless --force flag is used

WARNING: This action is irreversible and will permanently delete all version history.

Examples:
  rewind remove          # Remove current directory project
  rewind remove ./path   # Remove specified directory project
  rewind remove --force  # Remove without confirmation`,
	Run: func(cmd *cobra.Command, args []string) {

		cwd, err := os.Getwd()
		if err != nil {
			return
		}

		err = sendIPCMessage("remove", cwd)
		if err != nil {
			return
		}

		targetDir, err := determineTargetDirectory(args)
		app.Logger.WithField("directory", targetDir).Debug("Target directory")
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if err := validateDirectory(targetDir); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		absTargetDir, err := filepath.Abs(targetDir)
		app.Logger.WithField("abs_directory", absTargetDir).Debug("Absolute target directory")
		if err != nil {
			fmt.Printf("Error getting absolute path: %v\n", err)
			os.Exit(1)
		}

		force, _ := cmd.Flags().GetBool("force")

		if !force {
			fmt.Printf("Are you sure you want to remove %s? (y/N): ", absTargetDir)
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				fmt.Printf("Error reading input: %v\n", err)
				os.Exit(1)
			}

			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Println("Operation cancelled.")
				return
			}
		}

		if err := os.RemoveAll(filepath.Join(absTargetDir, ".rewind")); err != nil {
			app.Logger.WithField("abs_directory", absTargetDir).Error("clould not remove .rewind directory")
			return
		}

		app.Logger.WithField("abs_directory", absTargetDir).Error("removed .rewind directory")

	},
}

func init() {
	rootCmd.AddCommand(removeCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// removeCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	removeCmd.Flags().BoolP("force", "f", false, "Don't confirm deleting of .rewind")
}
