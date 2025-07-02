package watcher

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/davenicholson-xyz/rewind/app"
)

type WatchList struct {
	ListPath string
	Watches  []*Watch
}

func NewWatchList() (*WatchList, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	listPath := filepath.Join(homeDir, ".config", "rewind", "watchlist.json")

	wl := &WatchList{ListPath: listPath}

	// Load existing watches
	loadedWatches, err := wl.LoadWatchlist()
	if err != nil {
		return nil, err
	}

	// Prepare each watch and collect the valid ones
	var validWatches []*Watch

	for i := range loadedWatches {
		preparedWatch, err := wl.prepareWatch(&loadedWatches[i])
		if err != nil {
			app.Logger.WithField("path", loadedWatches[i].Path).WithError(err).Warn("Dropping watch due to preparation failure")
			continue
		}
		validWatches = append(validWatches, preparedWatch)
	}

	// Set the prepared watches
	wl.Watches = validWatches

	watchesToSave := make([]Watch, len(validWatches))
	for i, watch := range validWatches {
		watchesToSave[i] = *watch // Dereference the pointer
	}

	// Save the updated watchlist (this will remove any watches that failed preparation)
	if err := wl.SaveWatchlist(watchesToSave); err != nil {
		return nil, fmt.Errorf("failed to save prepared watchlist: %w", err)
	}

	return wl, nil
}

func (wl *WatchList) FindByPath(path string) (*Watch, bool) {

	if root, found := wl.FindRewindRoot(path); found {
		for i := range wl.Watches {
			if wl.Watches[i].Path == root {
				return wl.Watches[i], true
			}
		}
	}

	return nil, false

}

func (wl *WatchList) FindRewindRoot(path string) (string, bool) {
	if !filepath.IsAbs(path) {
		return "", false
	}

	currentDir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		currentDir = filepath.Dir(path)
	}

	for {
		// Check if current directory contains .rewind
		rewindPath := filepath.Join(currentDir, ".rewind")
		if info, err := os.Stat(rewindPath); err == nil && info.IsDir() {
			return currentDir, true
		}

		// Move to parent directory
		parentDir := filepath.Dir(currentDir)

		// If we've reached the root or can't go further up, stop
		if parentDir == currentDir {
			break
		}

		currentDir = parentDir
	}

	// No .rewind directory found
	return "", false
}

func (wl *WatchList) LoadWatchlist() ([]Watch, error) {
	app.Logger.WithField("path", wl.ListPath).Info("Loading watchlist from configuration")

	if _, err := os.Stat(wl.ListPath); os.IsNotExist(err) {
		app.Logger.Info("Watchlist file not found, returning empty list")
		return []Watch{}, nil
	}

	data, err := os.ReadFile(wl.ListPath)
	if err != nil {
		app.Logger.WithError(err).Error("Failed to read watchlist configuration file")
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var watches []Watch
	if err := json.Unmarshal(data, &watches); err != nil {
		app.Logger.WithError(err).Error("Failed to parse watchlist configuration")
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	app.Logger.WithField("count", len(watches)).Info("Successfully loaded watchlist")
	return watches, nil
}

// AddWatch adds a new watch to the configuration file
func (wl *WatchList) AddWatch(path string) (*Watch, error) {
	logger := app.Logger.WithField("path", path)
	logger.Info("Adding watch to configuration")

	for _, watch := range wl.Watches {
		if watch.Path == path {
			logger.Info("Watch already exists in memory")
			return nil, fmt.Errorf("watch already exists for path: %s", path)
		}
	}

	// Load existing watchlist
	watches, err := wl.LoadWatchlist()
	if err != nil {
		return nil, fmt.Errorf("failed to load existing watchlist: %w", err)
	}

	// Check if watch already exists
	for _, watch := range watches {
		if watch.Path == path {
			logger.Info("Watch already exists in configuration")
			return nil, fmt.Errorf("watch already exists for path: %s", path)
		}
	}

	// Validate that the path exists and is a directory
	if err := wl.validateWatchPath(path); err != nil {
		logger.Info("watch path does not exist")
		return nil, fmt.Errorf("invalid watch path: %w", err)
	}

	// Create new watch
	newWatch := Watch{
		Path:   path,
		Active: true,
	}

	// Add to list
	watches = append(watches, newWatch)

	// Save back to file
	if err := wl.SaveWatchlist(watches); err != nil {
		return nil, fmt.Errorf("failed to save updated watchlist: %w", err)
	}

	preparedWatch, err := wl.prepareWatch(&newWatch)
	if err != nil {
		app.Logger.WithField("path", newWatch.Path).WithError(err).Warn("Dropping watch due to preparation failure")
		return nil, fmt.Errorf("failed to prepare new watch: %w", err)
	}

	wl.Watches = append(wl.Watches, preparedWatch)

	logger.Info("Successfully added watch to configuration")
	return &newWatch, nil
}

func (wl *WatchList) RemoveWatch(path string) (*Watch, error) {
	logger := app.Logger.WithField("path", path)
	logger.Info("Removing watch from configuration")

	// Load from file
	watches, err := wl.LoadWatchlist()
	if err != nil {
		return nil, fmt.Errorf("failed to load existing watchlist: %w", err)
	}

	found := false
	newWatches := make([]Watch, 0, len(watches))

	for _, watch := range watches {
		if watch.Path != path {
			newWatches = append(newWatches, watch)
		} else {
			found = true
		}
	}

	if !found {
		logger.Info("Watch not found in configuration file")
		return nil, fmt.Errorf("watch not found for path: %s", path)
	}

	// Save updated list to file
	if err := wl.SaveWatchlist(newWatches); err != nil {
		return nil, fmt.Errorf("failed to save updated watchlist: %w", err)
	}

	// **Update in-memory watches and find the removed watch**
	var foundWatch *Watch
	newInMemoryWatches := make([]*Watch, 0, len(wl.Watches))
	for _, watch := range wl.Watches {
		if watch.Path != path {
			newInMemoryWatches = append(newInMemoryWatches, watch)
		} else {
			foundWatch = watch // Store the pointer to the found watch
		}
	}

	if foundWatch == nil {
		logger.Error("Watch found in file but not in memory")
		return nil, fmt.Errorf("watch not found in memory for path: %s", path)
	}

	// Update the in-memory watches
	wl.Watches = newInMemoryWatches

	logger.WithField("watchDirs", len(foundWatch.WatchDirs)).Debug("Found watch with directories")
	logger.Info("Successfully removed watch from configuration")
	return foundWatch, nil
}

func (wl *WatchList) SaveWatchlist(watches []Watch) error {
	app.Logger.WithField("configPath", wl.ListPath).WithField("count", len(watches)).Debug("Saving watchlist to configuration")

	// Ensure config directory exists
	configDir := filepath.Dir(wl.ListPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		app.Logger.WithError(err).Error("Failed to create config directory")
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Ensure we have a non-nil slice to avoid JSON null output
	if watches == nil {
		watches = []Watch{}
	}

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(watches, "", "  ")
	if err != nil {
		app.Logger.WithError(err).Error("Failed to marshal watchlist to JSON")
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file with appropriate permissions
	if err := os.WriteFile(wl.ListPath, data, 0644); err != nil {
		app.Logger.WithError(err).Error("Failed to write watchlist configuration file")
		return fmt.Errorf("failed to write config file: %w", err)
	}

	app.Logger.Info("Successfully saved watchlist configuration")
	return nil
}

// validateWatchPath checks if a path is suitable for watching
func (wl *WatchList) validateWatchPath(path string) error {
	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	// Check if it's a directory
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	// Check if we can read the directory
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("cannot read directory %s: %w", path, err)
	}

	// Optional: Check if .rewind directory exists (depending on your requirements)
	rewindPath := filepath.Join(path, ".rewind")
	if _, err := os.Stat(rewindPath); os.IsNotExist(err) {
		app.Logger.WithField("path", path).Warn("No .rewind directory found - this may not be a rewind project")
		return fmt.Errorf("no .rewind directory found at %s: %w", path, err)
	}

	app.Logger.WithField("path", path).WithField("entries", len(entries)).Debug("Path validation successful")
	return nil
}

// Replace the discoverWatchDirectories method in your WatchList
func (wl *WatchList) discoverWatchDirectories(watch *Watch) ([]string, error) {
	app.Logger.WithField("rootDir", watch.Path).WithField("ignorePatterns", len(watch.IgnorePatterns)).Debug("Discovering watch directories")
	var watchDirs []string

	err := filepath.WalkDir(watch.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			app.Logger.WithField("path", path).WithField("error", err).Warn("Error walking directory")
			return err
		}

		if !d.IsDir() {
			return nil
		}

		// Use the Watch's ShouldIgnore method instead of WatchList's
		if watch.ShouldIgnore(path) {
			relPath, _ := filepath.Rel(watch.Path, path)
			app.Logger.WithField("path", relPath).WithField("name", d.Name()).Debug("Ignoring directory")
			return filepath.SkipDir
		}

		watchDirs = append(watchDirs, path)
		relPath, _ := filepath.Rel(watch.Path, path)
		app.Logger.WithField("path", path).WithField("relPath", relPath).Debug("Added watch directory")
		return nil
	})

	if err != nil {
		app.Logger.WithField("rootDir", watch.Path).WithField("error", err).Error("Error walking directory tree")
		return nil, fmt.Errorf("error walking directory tree: %w", err)
	}

	app.Logger.WithField("totalDirectories", len(watchDirs)).Info("Directory discovery completed")
	return watchDirs, nil
}

// Update the prepareWatch method to pass the watch instance
func (wl *WatchList) prepareWatch(watch *Watch) (*Watch, error) {
	logger := app.Logger.WithField("path", watch.Path)

	if !watch.Active {
		logger.Info("Watch not active, skipping preparation")
		return nil, fmt.Errorf("Watch not active")
	}

	// Check if .rewind directory exists
	if _, err := os.Stat(filepath.Join(watch.Path, ".rewind")); os.IsNotExist(err) {
		logger.Error("No .rewind folder found")
		return nil, fmt.Errorf("No .rewind directory found")
	}

	ignorePatterns, err := wl.loadIgnorePatterns(watch.Path)
	if err != nil {
		logger.WithError(err).Error("Failed to discover ignore patterns")
		return nil, fmt.Errorf("failed to discover ignore patterns")
	}

	watch.IgnorePatterns = ignorePatterns

	// Now pass the watch instance instead of separate parameters
	watchDirs, err := wl.discoverWatchDirectories(watch)
	if err != nil {
		logger.WithError(err).Error("Failed to discover directories")
		return nil, fmt.Errorf("failed to discover watch directories")
	}

	watch.WatchDirs = watchDirs

	logger.WithField("directoriesFound", len(watchDirs)).WithField("ignorePatterns", len(ignorePatterns)).Info("Watch preparation completed")
	return watch, nil
}

func (wl *WatchList) loadIgnorePatterns(rootDir string) ([]string, error) {
	app.Logger.WithField("rootDir", rootDir).Debug("Loading ignore patterns")

	patterns := []string{".rewind", ".rewind/*"}

	// homeDir, err := os.UserHomeDir()
	// if err != nil {
	// 	return nil, nil
	// }
	// defaultIgnorePath := filepath.Join(homeDir, ".config", "rewind", "ignore")
	// app.Logger.WithField("path", defaultIgnorePath).Debug("Checking for config rewind/ignore file")
	//
	// if rewindPatterns, err := wl.readIgnoreFile(defaultIgnorePath); err == nil {
	// 	patterns = append(patterns, rewindPatterns...)
	// 	app.Logger.WithField("count", len(rewindPatterns)).Debug("Loaded patterns from .rewind/ignore")
	// } else if !os.IsNotExist(err) {
	// 	app.Logger.WithField("path", defaultIgnorePath).WithField("error", err).Error("Error reading .rewind/ignore")
	// 	return nil, fmt.Errorf("error reading .rewind/ignore: %w", err)
	// } else {
	// 	app.Logger.WithField("path", defaultIgnorePath).Debug(".rewind/ignore file not found")
	// }

	// Check for .rewind/ignore file
	rewindIgnorePath := filepath.Join(rootDir, ".rewind", "ignore")
	app.Logger.WithField("path", rewindIgnorePath).Debug("Checking for .rewind/ignore file")

	if rewindPatterns, err := wl.readIgnoreFile(rewindIgnorePath); err == nil {
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

	if rwPatterns, err := wl.readIgnoreFile(rwIgnorePath); err == nil {
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

func (wl *WatchList) readIgnoreFile(filePath string) ([]string, error) {
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
