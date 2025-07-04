package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/davenicholson-xyz/rewind/internal/database"
	"github.com/davenicholson-xyz/rewind/internal/watcher"
	"github.com/spf13/cobra"
)

// Message represents the IPC message structure
type Message struct {
	Action string `json:"action"`
	Path   string `json:"path"`
}

// Response represents the IPC response structure
type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new rewind project",
	Long: `Initialize a new rewind project in the specified directory (or current directory).
This creates a .rewind directory with necessary configuration files and database schema.
The project is automatically added to the daemon's watch list.

The initialization process:
- Creates .rewind directory structure
- Initializes SQLite database with version schema
- Creates default ignore patterns file
- Notifies the daemon to start monitoring

Examples:
  rewind init          # Initialize in current directory
  rewind init ./path   # Initialize in specified directory`,
	Run: func(cmd *cobra.Command, args []string) {

		app.Logger.Info("Starting new rewind app")

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

		if err := checkExistingRewind(absTargetDir); err != nil {
			app.Logger.Error("Already inside rewind project")
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if err := initializeRewindProject(absTargetDir); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Perform initial scan of all files
		if err := performInitialScan(absTargetDir); err != nil {
			app.Logger.WithField("error", err).Error("Failed to perform initial scan")
			fmt.Printf("Warning: Failed to perform initial scan: %v\n", err)
			// Don't exit here as the project is still initialized
		}

		// Send IPC message after successful initialization
		if err := sendIPCMessage("add", absTargetDir); err != nil {
			app.Logger.WithField("error", err).Error("Failed to send IPC message")

			// Clean up the .rewind directory since daemon notification failed
			rewindDir := filepath.Join(absTargetDir, ".rewind")
			if cleanupErr := os.RemoveAll(rewindDir); cleanupErr != nil {
				app.Logger.WithField("cleanup_error", cleanupErr).Error("Failed to cleanup .rewind directory")
				fmt.Printf("Error: Failed to notify rewind daemon and failed to cleanup: %v (original error: %v)\n", cleanupErr, err)
			} else {
				app.Logger.Info("Cleaned up .rewind directory after IPC failure")
				fmt.Printf("Error: Failed to notify rewind daemon, cleaned up .rewind directory: %v\n", err)
			}
			os.Exit(1)
		} else {
			app.Logger.Info("Successfully notified rewind daemon")
		}

	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func determineTargetDirectory(args []string) (string, error) {
	if len(args) == 0 || args[0] == "." {
		return os.Getwd()
	}
	return args[0], nil
}

func validateDirectory(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("target directory does not exist: %s", dir)
	}
	return nil
}

func checkExistingRewind(absTargetDir string) error {
	rewindDir := filepath.Join(absTargetDir, ".rewind")
	if _, err := os.Stat(rewindDir); err == nil {
		return fmt.Errorf("rewind already initialized in this directory")
	}

	if hasRewindInParents(absTargetDir) {
		return fmt.Errorf("already inside a rewind project")
	}

	return nil
}

func hasRewindInParents(dir string) bool {
	currentDir := filepath.Dir(dir)

	for currentDir != filepath.Dir(currentDir) {
		rewindPath := filepath.Join(currentDir, ".rewind")
		if _, err := os.Stat(rewindPath); err == nil {
			return true
		}
		currentDir = filepath.Dir(currentDir)
	}

	return false
}

func initializeRewindProject(absTargetDir string) error {
	rewindDir := filepath.Join(absTargetDir, ".rewind")

	if err := createRewindDirectory(rewindDir); err != nil {
		return fmt.Errorf("unable to create .rewind directory: %w", err)
	}
	app.Logger.WithField("dir", rewindDir).Info("Created .rewind directory")

	dbm, err := database.NewDatabaseManager(absTargetDir)
	if err != nil {
		return fmt.Errorf("unable to create db manager: %w", err)
	}

	if err := dbm.InitDatabase(); err != nil {
		return fmt.Errorf("unable to initialize database: %w", err)
	}

	return nil
}

func createRewindDirectory(dir string) error {
	if err := os.Mkdir(dir, 0755); err != nil {
		return err
	}

	ignoreContent := `# Auto-generated by rewind
.git
.git/*
node_modules
node_modules/*
.DS_Store
*.tmp
*.log
*~
*.swp
*.swo
.*.swp
.*.swo
#*#
.#*
*.tmp
*.*~
.vscode/*
*.zip
*.tar
*.tar.gz
*.tgz
*.tar.bz2
*.tbz2
*.tar.xz
*.txz
*.gz
*.bz2
*.xz
*.7z
*.rar
*.lz
*.lzma
*.Z
`

	ignoreFile := filepath.Join(dir, "ignore")
	return os.WriteFile(ignoreFile, []byte(ignoreContent), 0644)
}

// performInitialScan scans all files in the target directory and adds them to the database
func performInitialScan(targetDir string) error {
	app.Logger.Info("Performing initial scan of all files")
	
	// Create a watch for the target directory
	watch := &watcher.Watch{
		Path:   targetDir,
		Active: true,
	}
	
	// Load ignore patterns manually
	ignorePatterns, err := loadIgnorePatterns(targetDir)
	if err != nil {
		return fmt.Errorf("failed to load ignore patterns: %w", err)
	}
	watch.IgnorePatterns = ignorePatterns
	
	// Discover watch directories manually
	watchDirs, err := discoverWatchDirectories(watch)
	if err != nil {
		return fmt.Errorf("failed to discover watch directories: %w", err)
	}
	watch.WatchDirs = watchDirs
	
	// Create a temporary watch list with just this prepared watch
	tempWatchList := &watcher.WatchList{
		Watches: []*watcher.Watch{watch},
	}
	
	// Create a watch manager
	watchManager, err := watcher.NewWatchManager(tempWatchList)
	if err != nil {
		return fmt.Errorf("failed to create watch manager: %w", err)
	}
	
	// Perform the initial scan
	if err := watchManager.PerformInitialScan(); err != nil {
		return fmt.Errorf("failed to perform initial scan: %w", err)
	}
	
	app.Logger.Info("Initial scan completed successfully")
	return nil
}

// loadIgnorePatterns loads ignore patterns from .rewind/ignore and .rwignore files
func loadIgnorePatterns(rootDir string) ([]string, error) {
	app.Logger.WithField("rootDir", rootDir).Debug("Loading ignore patterns")
	
	patterns := []string{".rewind", ".rewind/*"}
	
	// Check for .rewind/ignore file
	rewindIgnorePath := filepath.Join(rootDir, ".rewind", "ignore")
	if rewindPatterns, err := readIgnoreFile(rewindIgnorePath); err == nil {
		patterns = append(patterns, rewindPatterns...)
		app.Logger.WithField("count", len(rewindPatterns)).Debug("Loaded patterns from .rewind/ignore")
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("error reading .rewind/ignore: %w", err)
	}
	
	// Check for .rwignore file
	rwIgnorePath := filepath.Join(rootDir, ".rwignore")
	if rwPatterns, err := readIgnoreFile(rwIgnorePath); err == nil {
		patterns = append(patterns, rwPatterns...)
		app.Logger.WithField("count", len(rwPatterns)).Debug("Loaded patterns from .rwignore")
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("error reading .rwignore: %w", err)
	}
	
	app.Logger.WithField("totalCount", len(patterns)).Info("Ignore patterns loaded")
	return patterns, nil
}

// readIgnoreFile reads patterns from an ignore file
func readIgnoreFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var patterns []string
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}
	
	return patterns, scanner.Err()
}

// discoverWatchDirectories discovers all directories that should be watched
func discoverWatchDirectories(watch *watcher.Watch) ([]string, error) {
	app.Logger.WithField("rootDir", watch.Path).Debug("Discovering watch directories")
	var watchDirs []string
	
	err := filepath.WalkDir(watch.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			app.Logger.WithField("path", path).WithField("error", err).Warn("Error walking directory")
			return nil // Continue with other directories
		}
		
		if !d.IsDir() {
			return nil
		}
		
		// Use the Watch's ShouldIgnore method
		if watch.ShouldIgnore(path) {
			relPath, _ := filepath.Rel(watch.Path, path)
			app.Logger.WithField("path", relPath).Debug("Ignoring directory")
			return filepath.SkipDir
		}
		
		watchDirs = append(watchDirs, path)
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("error walking directory tree: %w", err)
	}
	
	app.Logger.WithField("totalDirectories", len(watchDirs)).Info("Directory discovery completed")
	return watchDirs, nil
}
