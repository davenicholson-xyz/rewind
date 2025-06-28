package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/spf13/cobra"
)

const RewindDirName = ".rewind"
const EnvrcFileName = ".envrc"
const IgnoreFileName = ".rwignore"

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Initialize a new rewind project",
	Long: `Initialize a new rewind project in the specified directory or current directory.

This command creates a .rewind directory and sets up the necessary configuration files
for file watching and version control. If no directory is specified, the current
directory will be used.

By default, this will start the file watcher as a background daemon after initialization.

Examples:
  rewind init
  rewind init .
  rewind init /path/to/project
  rewind init my-project
  rewind init --foreground    # Start in foreground mode instead of daemon`,
	Run: runInit,
}

var createEnvrc bool
var createIgnore bool
var foregroundMode bool

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&createEnvrc, "envrc", "e", false, "Generate .envrc file")
	initCmd.Flags().BoolVarP(&createIgnore, "ignore", "i", false, "Generate ignore file")
	initCmd.Flags().BoolVarP(&foregroundMode, "foreground", "f", false, "Start watcher in foreground instead of daemon mode")
}

func runInit(cmd *cobra.Command, args []string) {
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

	fmt.Printf("✅ Rewind project initialized in %s\n", absTargetDir)

	// Change to the target directory before starting watcher
	if err := os.Chdir(absTargetDir); err != nil {
		fmt.Printf("Error: Could not change to project directory: %v\n", err)
		os.Exit(1)
	}

	// Start watcher based on mode
	if foregroundMode {
		fmt.Println("🚀 Starting watcher in foreground mode...")
		fmt.Println("Press Ctrl+C to stop the watcher.")
		runWatcherForeground(targetDir)
	} else {
		fmt.Println("🚀 Starting watcher daemon...")
		if err := startDaemonFromInit(absTargetDir); err != nil {
			fmt.Printf("Warning: Could not start daemon: %v\n", err)
			fmt.Println("You can start it manually with: rewind watch --daemon")
		}
	}
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

func initializeRewindProject(absTargetDir string) error {
	rewindDir := filepath.Join(absTargetDir, RewindDirName)

	if err := createRewindDirectory(rewindDir); err != nil {
		return fmt.Errorf("unable to create .rewind directory: %w", err)
	}
	app.Logger.WithField("dir", rewindDir).Info("Created .rewind directory")

	if createEnvrc {
		if err := createEnvrcFile(absTargetDir); err != nil {
			return fmt.Errorf("unable to create .envrc file: %w", err)
		}
		app.Logger.WithField("dir", filepath.Join(absTargetDir, EnvrcFileName)).Info("Created .envrc file")
	}

	if createIgnore {
		if err := createIgnoreFile(absTargetDir); err != nil {
			return fmt.Errorf("unable to create .rwignore file: %w", err)
		}
		app.Logger.WithField("dir", filepath.Join(absTargetDir, IgnoreFileName)).Info("Created .rwignore file")
	}

	dbm, err := app.NewDatabaseManager(absTargetDir)
	if err != nil {
		return fmt.Errorf("unable to create db manager: %w", err)
	}

	if err := dbm.InitDatabase(); err != nil {
		return fmt.Errorf("unable to initialize database: %w", err)
	}

	return nil
}

func hasRewindInParents(dir string) bool {
	currentDir := filepath.Dir(dir)

	for currentDir != filepath.Dir(currentDir) {
		rewindPath := filepath.Join(currentDir, RewindDirName)
		if _, err := os.Stat(rewindPath); err == nil {
			return true
		}
		currentDir = filepath.Dir(currentDir)
	}

	return false
}

func createEnvrcFile(dir string) error {
	envrcFile := filepath.Join(dir, EnvrcFileName)
	if _, err := os.Stat(envrcFile); err == nil {
		return nil // File already exists, skip creation
	}

	envrcContent := `# Auto-generated by rewind
if command -v rewind >/dev/null 2>&1; then
    # Start rewind daemon if not already running
    if ! rewind watch --status >/dev/null 2>&1; then
        rewind watch --daemon >/dev/null 2>&1
    fi
fi`

	return os.WriteFile(envrcFile, []byte(envrcContent), 0644)
}

func createIgnoreFile(dir string) error {
	ignoreFile := filepath.Join(dir, IgnoreFileName)
	if _, err := os.Stat(ignoreFile); err == nil {
		return nil
	}

	ignoreContent := `# Auto-generated by rewind
`

	return os.WriteFile(ignoreFile, []byte(ignoreContent), 0644)
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
.rewind
.rewind/
.rwignore
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
`

	ignoreFile := filepath.Join(dir, "ignore")
	return os.WriteFile(ignoreFile, []byte(ignoreContent), 0644)
}

// startDaemonFromInit starts the daemon during initialization
func startDaemonFromInit(cwd string) error {
	app.Logger.Info("Starting rewind watcher as daemon from init")

	// Check if daemon is already running
	if isDaemonRunning(cwd) {
		fmt.Println("✅ Rewind watcher is already running")
		return nil
	}

	// Get current executable path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create log file for daemon output
	logDir := filepath.Join(cwd, ".rewind", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	logFile := filepath.Join(logDir, "daemon.log")

	// Start the process in background
	cmd := exec.Command(executable, "watch")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "REWIND_DAEMON=1")

	// Redirect output to log file
	if logFileHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		cmd.Stdout = logFileHandle
		cmd.Stderr = logFileHandle
		defer logFileHandle.Close()
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Save PID file
	pidFile := getPidFilePath(cwd)
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Printf("✅ Rewind watcher started as daemon (PID: %d)\n", cmd.Process.Pid)
	fmt.Printf("📝 Logs: %s\n", logFile)
	fmt.Printf("💡 Use 'rewind status' to check daemon status\n")
	fmt.Printf("💡 Use 'rewind watch --stop' to stop the daemon\n")

	return nil
}

// // runWatcherForeground runs the watcher in foreground mode during init
// func runWatcherForeground() {
// 	if err := runWatcher(); err != nil {
// 		app.Logger.WithField("error", err).Error("Watcher failed")
// 		fmt.Printf("Error: %v\n", err)
// 		os.Exit(1)
// 	}
// }
