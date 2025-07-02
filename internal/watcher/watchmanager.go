package watcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"slices"

	"github.com/davenicholson-xyz/rewind/app"
	"github.com/davenicholson-xyz/rewind/internal/database"
	"github.com/davenicholson-xyz/rewind/internal/events"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

type WatchManager struct {
	WatchList      *WatchList
	EventsNotifier *events.EventsNotifier
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	EventChan      chan fsnotify.Event // Exposed channel for consuming events
	startTime      time.Time           // Track when the manager started
	mu             sync.RWMutex        // Protect concurrent access to status fields
}

type WatchManagerStatus struct {
	IsRunning        bool                `json:"is_running"`
	TotalWatches     int                 `json:"total_watches"`
	TotalWatchedDirs int                 `json:"total_watched_dirs"`
	EventChannelSize int                 `json:"event_channel_size"`
	EventChannelCap  int                 `json:"event_channel_capacity"`
	ActiveGoroutines int                 `json:"active_goroutines"`
	StartTime        time.Time           `json:"start_time,omitzero"`
	UptimeDuration   string              `json:"uptime_duration,omitempty"`
	WatchDetails     []WatchStatusDetail `json:"watch_details"`
}

// WatchStatusDetail provides details about individual watches
type WatchStatusDetail struct {
	Path        string   `json:"path"`
	WatchDirs   []string `json:"watch_dirs"`
	DirCount    int      `json:"dir_count"`
	IgnoreCount int      `json:"ignore_count"`
}

func NewWatchManager(wl *WatchList) (*WatchManager, error) {
	en, err := events.NewEventsNotifier()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())

	wm := &WatchManager{
		WatchList:      wl,
		EventsNotifier: en,
		ctx:            ctx,
		cancel:         cancel,
		EventChan:      make(chan fsnotify.Event, 100), // Buffered channel for events
	}

	// Set up the callback so EventsNotifier can send events to WatchManager
	en.SetCallback(wm.sendEvent)

	app.Logger.WithField("count", len(wm.WatchList.Watches)).Debug("Retrieved projects")

	// Add all paths to the notifier
	for _, watch := range wm.WatchList.Watches {
		for _, path := range watch.WatchDirs {
			wm.EventsNotifier.AddPath(path)
		}
	}

	return wm, nil
}

func (wm *WatchManager) Start() error {
	app.Logger.Debug("Starting the watch manager")

	wm.mu.Lock()
	wm.startTime = time.Now()
	wm.mu.Unlock()

	// Start the events notifier in a separate goroutine
	wm.wg.Add(1)
	go func() {
		defer wm.wg.Done()
		if err := wm.EventsNotifier.Start(wm.ctx); err != nil && err != context.Canceled {
			app.Logger.WithError(err).Error("Events notifier stopped with error")
		}
	}()

	if err := wm.PerformInitialScan(); err != nil {
		app.Logger.WithError(err).Error("Could not complete initial scan")
	}

	return nil
}

// sendEvent is the callback that receives events from EventsNotifier
func (wm *WatchManager) sendEvent(event fsnotify.Event) {
	select {
	case wm.EventChan <- event:
		wm.handleEvent(event)
	case <-wm.ctx.Done():
		// Context cancelled, ignore event
	default:
		app.Logger.WithField("path", event.Name).Warn("Event channel full, dropping event")
	}
}

func (wm *WatchManager) handleEvent(event fsnotify.Event) {

	logger := app.Logger.WithField("path", event.Name)
	logger.Debug("Processing file system event")

	if watch, found := wm.WatchList.FindByPath(event.Name); found {
		logger.WithField("watch", watch.Path).Debug("Found .rewind watch")

		if watch.ShouldIgnore(event.Name) {
			logger.Info("Found in ignore list. Ignoring.")
			return
		}

		switch {
		case event.Op&fsnotify.Create == fsnotify.Create:
			logger.Debug("File created")
			wm.handleCreate(event.Name, watch)
		case event.Op&fsnotify.Write == fsnotify.Write:
			logger.Debug("File modified")
			wm.handleWrite(event.Name, watch)
		case event.Op&fsnotify.Remove == fsnotify.Remove:
			logger.Debug("File removed")
			wm.handleRemove(event.Name, watch)
		case event.Op&fsnotify.Rename == fsnotify.Rename:
			logger.Debug("File renamed")
			wm.handleRename(event.Name, watch)
		case event.Op&fsnotify.Chmod == fsnotify.Chmod:
			logger.Debug("File permissions changed")
			wm.handleChmod(event.Name, watch)
		}
	}

	return
}

func (wm *WatchManager) handleCreate(path string, watch *Watch) {
	info, err := os.Stat(path)
	if err != nil {
		app.Logger.WithField("path", path).WithField("error", err).Debug("Could not stat created item")
		return
	}

	if info.IsDir() {

		relPath, err := filepath.Rel(watch.Path, path)
		if err != nil {
			app.Logger.WithField("path", path).WithField("error", err).Warn("Failed to get relative path for created directory")
			return
		}

		if watch.ShouldIgnore(info.Name()) {
			app.Logger.WithField("path", relPath).Debug("Not watching newly created directory (ignored)")
			return
		}

		if err := wm.AddWatchDirectory(watch, relPath); err != nil {
			app.Logger.WithError(err).Warn("Failed to add new directory to watch")
		}

		app.Logger.WithField("watch", watch.Path).WithField("directory", relPath).Info("Added folder to watch list")
	} else {
		relPath, err := filepath.Rel(watch.Path, path)
		if err != nil {
			app.Logger.WithField("path", path).WithField("error", err).Warn("Failed to get relative path for created file")
			return
		}

		app.Logger.WithField("path", relPath).Info("File created - processing as potential edit")
		wm.ProcessFile(path, relPath, watch)
	}
}

func (wm *WatchManager) handleWrite(path string, watch *Watch) {

	relPath, err := filepath.Rel(watch.Path, path)
	if err != nil {
		app.Logger.WithField("path", path).WithField("error", err).Warn("Failed to get relative path for created directory")
		return
	}

	app.Logger.WithField("path", relPath).Info("File modified - processing as potential edit")
	wm.ProcessFile(path, relPath, watch)
}

func (wm *WatchManager) handleRemove(path string, watch *Watch) {
	relPath, err := filepath.Rel(watch.Path, path)
	if err != nil {
		app.Logger.WithField("path", path).WithField("error", err).Warn("Failed to get relative path for removed file")
		return
	}

	db, err := database.NewDatabaseManager(watch.Path)
	if err != nil {
		app.Logger.WithError(err).Warn("Could not initialise database for removed file")
		return
	}

	if err := db.Connect(); err != nil {
		app.Logger.WithError(err).Warn("Could not connect to database for removed file")
		return
	}
	defer db.Close()

	// Check if file exists in database
	latestVersion, err := db.GetLatestFileVersion(path)
	if err != nil {
		app.Logger.WithField("path", relPath).WithError(err).Error("Failed to check for existing file in database")
		return
	}

	if latestVersion == nil {
		app.Logger.WithField("path", relPath).Debug("File not tracked in database, ignoring deletion")
		return
	}

	// Mark the latest version as deleted instead of creating a new entry
	if err := db.MarkFileDeleted(path); err != nil {
		app.Logger.WithField("path", relPath).WithError(err).Error("Failed to mark file as deleted in database")
		return
	}

	app.Logger.WithField("path", relPath).WithField("version", latestVersion.VersionNumber).Info("File marked as deleted in database")
}

func (wm *WatchManager) handleRename(path string, watch *Watch) {
	relPath, err := filepath.Rel(watch.Path, path)
	if err != nil {
		app.Logger.WithField("path", path).WithField("error", err).Warn("Failed to get relative path for renamed file")
		return
	}

	// Check if the file still exists at the new location
	if _, err := os.Stat(path); err != nil {
		app.Logger.WithField("path", relPath).Debug("Renamed file no longer exists, ignoring")
		return
	}

	// Treat rename as a new file creation - use existing ProcessFile logic
	app.Logger.WithField("path", relPath).Info("File renamed - processing as new file")
	wm.ProcessFile(path, relPath, watch)
}

func (wm *WatchManager) handleChmod(path string, watch *Watch) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return // Skip if can't stat or is directory
	}

	// Check if this file is already being tracked
	db, err := database.NewDatabaseManager(watch.Path)
	if err != nil {
		return
	}
	defer db.Close()

	if err := db.Connect(); err != nil {
		return
	}

	// If file exists in database, treat chmod as potential content change
	if latestVersion, err := db.GetLatestFileVersion(path); err == nil && latestVersion != nil {
		relPath, _ := filepath.Rel(watch.Path, path)
		app.Logger.WithField("path", relPath).Info("CHMOD on tracked file - checking for changes")
		wm.ProcessFile(path, relPath, watch)
	}
}

func (wm *WatchManager) ProcessFile(filePath, relPath string, watch *Watch) (string, error) {

	db, err := database.NewDatabaseManager(watch.Path)
	if err != nil {
		app.Logger.WithError(err).Warn("Could not initialise database")
		return "", fmt.Errorf("Could not initialise database: %w", err)
	}

	if err := db.Connect(); err != nil {
		app.Logger.WithError(err).Warn("Could not connect to database")
		return "", fmt.Errorf("Could not connect to database: %w", err)
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	currentHash, err := database.CalculateFileHash(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to calculate file hash: %w", err)
	}

	latestVersion, err := db.GetLatestFileVersion(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get latest file version: %w", err)
	}

	if latestVersion == nil {
		app.Logger.WithField("path", relPath).Info("New file found during scan")

		if err := wm.addFileToDatabase(db, watch.Path, filePath, relPath, currentHash, fileInfo); err != nil {
			return "", fmt.Errorf("failed to add new file to database: %w", err)
		}

		return "new", nil
	}

	// Compare current hash with latest version hash
	if currentHash == latestVersion.FileHash {
		app.Logger.WithField("path", relPath).Debug("File unchanged - same hash as latest version")
		return "unchanged", nil
	}

	// File has changed - add new version
	app.Logger.WithField("path", relPath).Info("File changed - adding new version")
	if err := wm.addFileToDatabase(db, watch.Path, filePath, relPath, currentHash, fileInfo); err != nil {
		return "", fmt.Errorf("failed to add updated file to database: %w", err)
	}

	return "updated", nil
}

func (wm *WatchManager) addFileToDatabase(db *database.DatabaseManager, rootPath, filePath, relPath, fileHash string, fileInfo os.FileInfo) error {

	versionNumber, err := db.GetNextVersionNumber(filePath)
	if err != nil {
		return fmt.Errorf("failed to get next version number: %w", err)
	}

	// Create storage path
	storagePath := db.CreateStoragePath(filePath, versionNumber)
	fullStoragePath := filepath.Join(rootPath, ".rewind", "versions", storagePath)

	// Create storage directory if it doesn't exist (this now creates the full directory structure)
	storageDir := filepath.Dir(fullStoragePath)
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Copy file to storage location
	if err := wm.copyFile(filePath, fullStoragePath); err != nil {
		return fmt.Errorf("failed to copy file to storage: %w", err)
	}

	// Create file version record
	fileVersion := &database.FileVersion{
		FilePath:      relPath,
		VersionNumber: versionNumber,
		Timestamp:     time.Now(),
		FileHash:      fileHash,
		FileSize:      fileInfo.Size(),
		StoragePath:   storagePath,
	}

	// Add to database
	if err := db.AddFileVersion(fileVersion); err != nil {
		// Clean up the file if database insertion fails
		os.Remove(fullStoragePath)
		return fmt.Errorf("failed to add file version to database: %w", err)
	}

	app.Logger.WithFields(logrus.Fields{
		"path":        relPath,
		"version":     versionNumber,
		"size":        fileInfo.Size(),
		"storagePath": storagePath,
	}).Info("File version added to database")

	return nil
}

// copyFile copies a file from src to dst
func (wm *WatchManager) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
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

func (wm *WatchManager) AddWatch(path string) error {
	watch, err := wm.WatchList.AddWatch(path)
	if err != nil {
		return err
	}
	for _, path := range watch.WatchDirs {
		wm.EventsNotifier.AddPath(path)
	}
	return nil
}

func (wm *WatchManager) RemoveWatch(path string) error {
	app.Logger.WithField("path", path).Info("Removing watch from manager")

	// Log current state before removal
	app.Logger.WithField("currentWatches", len(wm.WatchList.Watches)).Debug("Current watches before removal")

	watch, err := wm.WatchList.RemoveWatch(path)
	if err != nil {
		return err
	}

	app.Logger.WithField("watchDirs", len(watch.WatchDirs)).Debug("Directories to remove from fsnotify")

	// Remove all watched directories from fsnotify with proper error handling
	removedCount := 0
	for _, dir := range watch.WatchDirs {
		app.Logger.WithField("dir", dir).Debug("Attempting to remove directory from fsnotify")
		if err := wm.EventsNotifier.Notifier.Remove(dir); err != nil {
			app.Logger.WithField("dir", dir).WithError(err).Error("Failed to remove directory from fsnotify watcher")
		} else {
			app.Logger.WithField("dir", dir).Debug("Successfully removed directory from fsnotify watcher")
			removedCount++
		}
	}

	app.Logger.WithField("path", path).WithField("removedDirs", removedCount).WithField("totalDirs", len(watch.WatchDirs)).Info("Watch removal completed")

	// Log current state after removal
	app.Logger.WithField("currentWatches", len(wm.WatchList.Watches)).Debug("Current watches after removal")

	return nil
}

// Stop gracefully shuts down the WatchManager
func (wm *WatchManager) Stop() error {
	app.Logger.Debug("Stopping watch manager")

	// Cancel context to stop all goroutines
	wm.cancel()

	// Close the events notifier
	if err := wm.EventsNotifier.Close(); err != nil {
		app.Logger.WithError(err).Error("Error closing events notifier")
	}

	// Close event channel
	close(wm.EventChan)

	// Wait for all goroutines to finish
	wm.wg.Wait()

	app.Logger.Debug("Watch manager stopped")
	return nil
}

func (wm *WatchManager) PerformInitialScan() error {
	app.Logger.Info("Starting initial file system scan")

	var totalFiles int
	var newFiles int
	var changedFiles int
	var unchangedFiles int

	// Scan each watch in the watch list
	for _, watch := range wm.WatchList.Watches {
		if !watch.Active {
			app.Logger.WithField("path", watch.Path).Debug("Skipping inactive watch during scan")
			continue
		}

		app.Logger.WithField("watch", watch.Path).Debug("Scanning watch directory")

		err := filepath.WalkDir(watch.Path, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				app.Logger.WithField("path", path).WithField("error", err).Warn("Error accessing file during scan")
				return nil // Continue with other files
			}

			// Check if file should be ignored using the watch's ignore logic
			if watch.ShouldIgnore(path) {
				if d.IsDir() {
					app.Logger.WithField("path", path).Debug("Skipping ignored directory and all its contents during scan")
					return filepath.SkipDir
				} else {
					app.Logger.WithField("path", path).Debug("Ignoring file during scan")
					return nil
				}
			}

			// Skip directories (we only process files)
			if d.IsDir() {
				return nil
			}

			totalFiles++

			// Get relative path for processing
			relPath, err := filepath.Rel(watch.Path, path)
			if err != nil {
				app.Logger.WithField("path", path).WithField("error", err).Warn("Failed to get relative path")
				relPath = path
			}

			// Process the file using the existing ProcessFile method
			action, err := wm.ProcessFile(path, relPath, watch)
			if err != nil {
				app.Logger.WithField("path", path).WithField("error", err).Error("Failed to process file during scan")
				return nil // Continue with other files
			}

			switch action {
			case "new":
				newFiles++
			case "updated":
				changedFiles++
			case "unchanged":
				unchangedFiles++
			}

			return nil
		})

		if err != nil {
			app.Logger.WithField("watch", watch.Path).WithField("error", err).Error("Error walking directory during scan")
		}
	}

	app.Logger.WithFields(logrus.Fields{
		"totalFiles":     totalFiles,
		"newFiles":       newFiles,
		"changedFiles":   changedFiles,
		"unchangedFiles": unchangedFiles,
	}).Info("Initial scan completed")

	return nil
}

func (wm *WatchManager) GetStatus() WatchManagerStatus {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	status := WatchManagerStatus{
		IsRunning:        wm.isRunning(),
		TotalWatches:     len(wm.WatchList.Watches),
		EventChannelSize: len(wm.EventChan),
		EventChannelCap:  cap(wm.EventChan),
		ActiveGoroutines: wm.getActiveGoroutineCount(),
	}

	// Calculate uptime if running
	if !wm.startTime.IsZero() {
		status.StartTime = wm.startTime
		status.UptimeDuration = time.Since(wm.startTime).Round(time.Second).String()
	}

	// Count total watched directories and collect watch details
	totalDirs := 0
	watchDetails := make([]WatchStatusDetail, 0, len(wm.WatchList.Watches))

	for _, watch := range wm.WatchList.Watches {
		dirCount := len(watch.WatchDirs)
		totalDirs += dirCount

		// Count ignore patterns (assuming Watch has an IgnorePatterns field)
		ignoreCount := 0
		if watch.IgnorePatterns != nil {
			ignoreCount = len(watch.IgnorePatterns)
		}

		detail := WatchStatusDetail{
			Path:        watch.Path,
			WatchDirs:   watch.WatchDirs,
			DirCount:    dirCount,
			IgnoreCount: ignoreCount,
		}
		watchDetails = append(watchDetails, detail)
	}

	status.TotalWatchedDirs = totalDirs
	status.WatchDetails = watchDetails

	return status
}

// isRunning checks if the WatchManager is currently running
func (wm *WatchManager) isRunning() bool {
	select {
	case <-wm.ctx.Done():
		return false
	default:
		return !wm.startTime.IsZero()
	}
}

// getActiveGoroutineCount returns the number of active goroutines managed by WatchManager
func (wm *WatchManager) getActiveGoroutineCount() int {
	// This is a simple count based on what we know about our goroutines
	// In a more complex implementation, you might want to track this more precisely
	if wm.isRunning() {
		return 1 // The main EventsNotifier goroutine
	}
	return 0
}

func (wm *WatchManager) AddWatchDirectory(watch *Watch, relPath string) error {
	// Convert relative path to absolute path
	absPath := filepath.Join(watch.Path, relPath)

	logger := app.Logger.WithField("watchPath", watch.Path).WithField("relPath", relPath).WithField("absPath", absPath)
	logger.Debug("Adding directory to existing watch")

	// Validate the new directory exists and is within the watch path
	if err := wm.validateWatchDirectory(absPath, watch); err != nil {
		return fmt.Errorf("invalid directory: %w", err)
	}

	// Check if directory is already being watched
	if slices.Contains(watch.WatchDirs, absPath) {
		logger.Debug("Directory already being watched")
		return nil // Already watching, no error
	}

	// Check if the directory should be ignored
	if watch.ShouldIgnore(absPath) {
		logger.Debug("Directory matches ignore patterns, not adding")
		return fmt.Errorf("directory matches ignore patterns: %s", relPath)
	}

	// Add to the watch's directory list
	watch.WatchDirs = append(watch.WatchDirs, absPath)

	// Add to the event notifier
	if err := wm.EventsNotifier.AddPath(absPath); err != nil {
		// Remove from watch dirs if fsnotify fails
		watch.WatchDirs = watch.WatchDirs[:len(watch.WatchDirs)-1]
		return fmt.Errorf("failed to add directory to event notifier: %w", err)
	}

	logger.Info("Successfully added directory to watch")
	return nil
}

// RemoveWatchDirectory removes a single directory from an existing watch
func (wm *WatchManager) RemoveWatchDirectory(watch *Watch, relPath string) error {
	// Convert relative path to absolute path
	absPath := filepath.Join(watch.Path, relPath)

	logger := app.Logger.WithField("watchPath", watch.Path).WithField("relPath", relPath).WithField("absPath", absPath)
	logger.Debug("Removing directory from existing watch")

	// Find and remove the directory from the watch
	found := false
	newWatchDirs := make([]string, 0, len(watch.WatchDirs))
	for _, dir := range watch.WatchDirs {
		if dir != absPath {
			newWatchDirs = append(newWatchDirs, dir)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("directory not found in watch: %s", relPath)
	}

	// Update the watch dirs
	watch.WatchDirs = newWatchDirs

	// Remove from event notifier
	if err := wm.EventsNotifier.RemovePath(absPath); err != nil {
		logger.WithError(err).Warn("Failed to remove directory from event notifier")
		// Continue anyway since we've already updated the watch
	}

	logger.Info("Successfully removed directory from watch")
	return nil
}

// validateWatchDirectory validates that a directory is suitable for watching
func (wm *WatchManager) validateWatchDirectory(dirPath string, watch *Watch) error {
	// Check if directory exists
	info, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", dirPath)
		}
		return fmt.Errorf("failed to stat directory: %w", err)
	}

	// Check if it's actually a directory
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dirPath)
	}

	// Ensure the directory is within the watch path (security check)
	relPath, err := filepath.Rel(watch.Path, dirPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("directory is outside watch path: %s", dirPath)
	}

	return nil
}
