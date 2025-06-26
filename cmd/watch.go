package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"slices"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

type WatchManager struct {
	RootDirectory string
	Monitored     []string
	Ignored       []string
	watcher       *fsnotify.Watcher
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

func NewWatchManager(rootDir string) (*WatchManager, error) {
	app.Logger.WithField("rootDir", rootDir).Info("Creating new WatchManager")

	ignorePatterns, err := loadIgnorePatterns(rootDir)
	if err != nil {
		app.Logger.WithField("rootDir", rootDir).WithField("error", err).Error("Failed to load ignore patterns")
		return nil, fmt.Errorf("failed to load ignore patterns: %w", err)
	}
	app.Logger.WithField("count", len(ignorePatterns)).WithField("patterns", ignorePatterns).Debug("Loaded ignore patterns")

	watchDirs, err := discoverWatchDirectories(rootDir, ignorePatterns)
	if err != nil {
		app.Logger.WithField("rootDir", rootDir).WithField("error", err).Error("Failed to discover directories")
		return nil, fmt.Errorf("failed to discover directories: %w", err)
	}
	app.Logger.WithField("count", len(watchDirs)).Info("Discovered watch directories")

	// Create fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		app.Logger.WithField("error", err).Error("Failed to create fsnotify watcher")
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WatchManager{
		RootDirectory: rootDir,
		Monitored:     watchDirs,
		Ignored:       ignorePatterns,
		watcher:       watcher,
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

func (wm *WatchManager) Start() error {
	app.Logger.Info("Starting file system watcher")

	// Add all monitored directories to the watcher
	for _, dir := range wm.Monitored {
		if err := wm.watcher.Add(dir); err != nil {
			app.Logger.WithField("directory", dir).WithField("error", err).Error("Failed to add directory to watcher")
			return fmt.Errorf("failed to add directory %s to watcher: %w", dir, err)
		}
		app.Logger.WithField("directory", dir).Debug("Added directory to watcher")
	}

	app.Logger.WithField("totalDirectories", len(wm.Monitored)).Info("All directories added to watcher")

	// Start the event processing goroutine
	wm.wg.Add(1)
	go wm.processEvents()

	return nil
}

func (wm *WatchManager) Stop() error {
	app.Logger.Info("Stopping file system watcher")

	// Cancel the context to signal goroutines to stop
	wm.cancel()

	// Close the watcher
	if err := wm.watcher.Close(); err != nil {
		app.Logger.WithField("error", err).Error("Error closing fsnotify watcher")
	}

	// Wait for all goroutines to finish
	wm.wg.Wait()

	app.Logger.Info("File system watcher stopped")
	return nil
}

func (wm *WatchManager) processEvents() {
	defer wm.wg.Done()
	app.Logger.Info("Started event processing goroutine")

	for {
		select {
		case <-wm.ctx.Done():
			app.Logger.Info("Event processing goroutine stopping")
			return

		case event, ok := <-wm.watcher.Events:
			if !ok {
				app.Logger.Warn("Watcher events channel closed")
				return
			}
			wm.handleEvent(event)

		case err, ok := <-wm.watcher.Errors:
			if !ok {
				app.Logger.Warn("Watcher errors channel closed")
				return
			}
			app.Logger.WithField("error", err).Error("File system watcher error")
		}
	}
}

func (wm *WatchManager) handleEvent(event fsnotify.Event) {
	// Get relative path for ignore checking
	relPath, err := filepath.Rel(wm.RootDirectory, event.Name)
	if err != nil {
		app.Logger.WithField("path", event.Name).WithField("error", err).Warn("Failed to get relative path for event")
		relPath = event.Name
	}

	// Check if the file/directory should be ignored
	fileName := filepath.Base(event.Name)
	if wm.shouldIgnoreEvent(relPath, fileName) {
		app.Logger.WithField("path", relPath).WithField("fileName", fileName).Debug("Ignoring event for ignored path")
		return
	}

	// Log the event with appropriate detail
	logger := app.Logger.WithField("path", event.Name).WithField("relPath", relPath).WithField("op", event.Op.String())

	switch {
	case event.Has(fsnotify.Create):
		logger.Info("File/directory created")
		wm.handleCreate(event.Name)
	case event.Has(fsnotify.Write):
		logger.Info("File modified")
	case event.Has(fsnotify.Remove):
		logger.Info("File/directory removed")
		wm.handleRemove(event.Name)
	case event.Has(fsnotify.Rename):
		logger.Info("File/directory renamed")
	case event.Has(fsnotify.Chmod):
		return
		// logger.Debug("File/directory permissions changed")
	default:
		logger.Debug("Unknown file system event")
	}
}

func (wm *WatchManager) handleCreate(path string) {
	// Check if the created item is a directory
	info, err := os.Stat(path)
	if err != nil {
		app.Logger.WithField("path", path).WithField("error", err).Debug("Could not stat created item")
		return
	}

	if info.IsDir() {
		// Get relative path for ignore checking
		relPath, err := filepath.Rel(wm.RootDirectory, path)
		if err != nil {
			app.Logger.WithField("path", path).WithField("error", err).Warn("Failed to get relative path for created directory")
			return
		}

		// Check if the new directory should be ignored
		if wm.shouldIgnoreEvent(relPath, info.Name()) {
			app.Logger.WithField("path", relPath).Debug("Not watching newly created directory (ignored)")
			return
		}

		// Add the new directory to the watcher
		if err := wm.watcher.Add(path); err != nil {
			app.Logger.WithField("path", path).WithField("error", err).Error("Failed to add newly created directory to watcher")
		} else {
			app.Logger.WithField("path", path).Info("Added newly created directory to watcher")
			wm.Monitored = append(wm.Monitored, path)
		}
	}
}

func (wm *WatchManager) handleRemove(path string) {
	// Remove from monitored directories if it was being watched
	for i, monitored := range wm.Monitored {
		if monitored == path {
			wm.Monitored = slices.Delete(wm.Monitored, i, i+1)
			app.Logger.WithField("path", path).Info("Removed deleted directory from monitored list")
			break
		}
	}
}

func (wm *WatchManager) shouldIgnoreEvent(relPath, fileName string) bool {
	return shouldIgnore(relPath, fileName, wm.Ignored)
}

// watchCmd represents the watch command
var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runWatcher(); err != nil {
			app.Logger.WithField("error", err).Error("Watcher failed")
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

func runWatcher() error {
	app.Logger.Info("Started watcher")

	cwd, err := os.Getwd()
	if err != nil {
		app.Logger.WithField("error", err).Error("Unable to get current directory")
		return fmt.Errorf("unable to get current directory: %w", err)
	}
	app.Logger.WithField("cwd", cwd).Debug("Current working directory")

	if err := checkRewindProject(cwd); err != nil {
		app.Logger.WithField("cwd", cwd).WithField("error", err).Error("Rewind project check failed")
		fmt.Println(err)
		os.Exit(1)
	}
	app.Logger.WithField("cwd", cwd).Info("Rewind project validated")

	wm, err := NewWatchManager(cwd)
	if err != nil {
		app.Logger.WithField("error", err).Error("Failed to create WatchManager")
		return err
	}
	defer wm.Stop()

	app.Logger.WithField("monitoredDirs", len(wm.Monitored)).WithField("ignoredPatterns", len(wm.Ignored)).Info("WatchManager created successfully")
	app.Logger.WithField("directories", wm.Monitored).Debug("Monitored directories")

	// Start watching
	if err := wm.Start(); err != nil {
		app.Logger.WithField("error", err).Error("Failed to start watcher")
		return err
	}

	app.Logger.Info("File system watcher is running. Press Ctrl+C to stop.")

	// Wait for interrupt signal
	select {
	case <-wm.ctx.Done():
		app.Logger.Info("Watcher context cancelled")
	}

	return nil
}

func checkRewindProject(cwd string) error {
	rewindPath := filepath.Join(cwd, ".rewind")
	app.Logger.WithField("path", rewindPath).Debug("Checking for .rewind directory")

	if _, err := os.Stat(rewindPath); err != nil {
		if os.IsNotExist(err) {
			app.Logger.WithField("cwd", cwd).Warn("Not in a Rewind project - .rewind directory not found")
			return fmt.Errorf("⛔ Not in a Rewind project")
		}
		app.Logger.WithField("path", rewindPath).WithField("error", err).Error("Error checking .rewind directory")
		return err
	}

	app.Logger.WithField("path", rewindPath).Debug(".rewind directory found")
	return nil
}

func loadIgnorePatterns(rootDir string) ([]string, error) {
	app.Logger.WithField("rootDir", rootDir).Debug("Loading ignore patterns")
	var patterns []string

	// Check for .rewind/ignore file
	rewindIgnorePath := filepath.Join(rootDir, ".rewind", "ignore")
	app.Logger.WithField("path", rewindIgnorePath).Debug("Checking for .rewind/ignore file")

	if rewindPatterns, err := readIgnoreFile(rewindIgnorePath); err == nil {
		patterns = append(patterns, rewindPatterns...)
		app.Logger.WithField("count", len(rewindPatterns)).Debug("Loaded patterns from .rewind/ignore")
	} else if !os.IsNotExist(err) {
		app.Logger.WithField("path", rewindIgnorePath).WithField("error", err).Error("Error reading .rewind/ignore")
		return nil, fmt.Errorf("error reading .rewind/ignore: %w", err)
	} else {
		app.Logger.WithField("path", rewindIgnorePath).Debug(".rewind/ignore file not found")
	}

	// Check for .rwignore file
	rwIgnorePath := filepath.Join(rootDir, ".rwignore")
	app.Logger.WithField("path", rwIgnorePath).Debug("Checking for .rwignore file")

	if rwPatterns, err := readIgnoreFile(rwIgnorePath); err == nil {
		patterns = append(patterns, rwPatterns...)
		app.Logger.WithField("count", len(rwPatterns)).Debug("Loaded patterns from .rwignore")
	} else if !os.IsNotExist(err) {
		app.Logger.WithField("path", rwIgnorePath).WithField("error", err).Error("Error reading .rwignore")
		return nil, fmt.Errorf("error reading .rwignore: %w", err)
	} else {
		app.Logger.WithField("path", rwIgnorePath).Debug(".rwignore file not found")
	}

	app.Logger.WithField("totalCount", len(patterns)).Info("Ignore patterns loaded")
	return patterns, nil
}

func readIgnoreFile(filePath string) ([]string, error) {
	app.Logger.WithField("path", filePath).Debug("Reading ignore file")

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

	if err := scanner.Err(); err != nil {
		app.Logger.WithField("path", filePath).WithField("error", err).Error("Error scanning ignore file")
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	app.Logger.WithField("path", filePath).WithField("patterns", len(patterns)).Debug("Successfully read ignore file")
	return patterns, nil
}

func discoverWatchDirectories(rootDir string, ignorePatterns []string) ([]string, error) {
	app.Logger.WithField("rootDir", rootDir).WithField("ignorePatterns", len(ignorePatterns)).Debug("Discovering watch directories")
	var watchDirs []string

	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			app.Logger.WithField("path", path).WithField("error", err).Warn("Error walking directory")
			return err
		}

		if !d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			app.Logger.WithField("path", path).WithField("rootDir", rootDir).WithField("error", err).Error("Error getting relative path")
			return err
		}

		if shouldIgnore(relPath, d.Name(), ignorePatterns) {
			app.Logger.WithField("path", relPath).WithField("name", d.Name()).Debug("Ignoring directory")
			return filepath.SkipDir
		}

		watchDirs = append(watchDirs, path)
		app.Logger.WithField("path", path).WithField("relPath", relPath).Debug("Added watch directory")
		return nil
	})

	if err != nil {
		app.Logger.WithField("rootDir", rootDir).WithField("error", err).Error("Error walking directory tree")
		return nil, fmt.Errorf("error walking directory tree: %w", err)
	}

	app.Logger.WithField("totalDirectories", len(watchDirs)).Info("Directory discovery completed")
	return watchDirs, nil
}

func shouldIgnore(relPath, name string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == relPath || pattern == name {
			app.Logger.WithField("relPath", relPath).WithField("name", name).WithField("pattern", pattern).Debug("Directory matched exact pattern")
			return true
		}

		if strings.HasSuffix(pattern, "/*") {
			prefix := strings.TrimSuffix(pattern, "/*")
			if strings.HasPrefix(relPath, prefix) {
				app.Logger.WithField("relPath", relPath).WithField("pattern", pattern).Debug("Directory matched wildcard pattern")
				return true
			}
		}

		if strings.HasPrefix(relPath, pattern) {
			app.Logger.WithField("relPath", relPath).WithField("pattern", pattern).Debug("Directory matched prefix pattern")
			return true
		}

		if strings.HasPrefix(pattern, "*.") {
			ext := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(name, ext) {
				app.Logger.WithField("name", name).WithField("pattern", pattern).Debug("Directory matched extension pattern")
				return true
			}
		}
	}

	return false
}

// EventDebouncer helps reduce duplicate events
type EventDebouncer struct {
	events map[string]time.Time
	mutex  sync.RWMutex
}

func NewEventDebouncer() *EventDebouncer {
	return &EventDebouncer{
		events: make(map[string]time.Time),
	}
}

func (d *EventDebouncer) ShouldProcess(filePath string, eventType string) bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	key := filePath + ":" + eventType
	now := time.Now()

	if lastTime, exists := d.events[key]; exists {
		if now.Sub(lastTime) < 100*time.Millisecond {
			return false
		}
	}

	d.events[key] = now

	if len(d.events) > 1000 {
		cutoff := now.Add(-5 * time.Second)
		for k, v := range d.events {
			if v.Before(cutoff) {
				delete(d.events, k)
			}
		}
	}

	return true
}
