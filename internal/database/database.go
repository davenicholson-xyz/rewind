package database

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// FileVersion represents a file version in the database
type FileVersion struct {
	ID            int64
	FilePath      string
	VersionNumber int
	Timestamp     time.Time
	FileHash      string
	FileSize      int64
	StoragePath   string
	Deleted       bool
}

// Tag represents a version tag in the database
type Tag struct {
	ID        int64
	VersionID int64
	TagName   string
	CreatedAt time.Time
}

// DatabaseManager handles all database operations
type DatabaseManager struct {
	db      *sql.DB
	rootDir string
	dbPath  string
}

// NewDatabaseManager creates a new database manager instance
func NewDatabaseManager(rootDir string) (*DatabaseManager, error) {
	rewindDir := filepath.Join(rootDir, ".rewind")
	dbPath := filepath.Join(rewindDir, "versions.db")

	dm := &DatabaseManager{
		rootDir: rootDir,
		dbPath:  dbPath,
	}

	return dm, nil
}

// InitDatabase creates the .rewind directory and initializes the database schema
func (dm *DatabaseManager) InitDatabase() error {
	rewindDir := filepath.Dir(dm.dbPath)

	// Create .rewind directory if it doesn't exist
	if err := os.MkdirAll(rewindDir, 0755); err != nil {
		return fmt.Errorf("failed to create .rewind directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite", dm.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	dm.db = db

	// Create schema
	if err := dm.createSchema(); err != nil {
		return fmt.Errorf("failed to create database schema: %w", err)
	}

	return nil
}

// Connect opens a connection to an existing database
func (dm *DatabaseManager) Connect() error {
	// Check if database exists
	if _, err := os.Stat(dm.dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database does not exist at %s. Run 'rewind init' first", dm.dbPath)
	}

	// Open database connection
	db, err := sql.Open("sqlite", dm.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	dm.db = db

	return nil
}

// createSchema creates the database tables and indexes
func (dm *DatabaseManager) createSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS versions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL,
		version_number INTEGER NOT NULL,
		timestamp TEXT NOT NULL,
		file_hash TEXT NOT NULL,
		file_size INTEGER NOT NULL,
		storage_path TEXT NOT NULL,
		deleted BOOLEAN NOT NULL DEFAULT 0,
		UNIQUE(file_path, version_number)
	);

	CREATE TABLE IF NOT EXISTS tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		tag_name TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (version_id) REFERENCES versions(id),
		UNIQUE(version_id, tag_name)
	);

	CREATE INDEX IF NOT EXISTS idx_file_path ON versions(file_path);
	CREATE INDEX IF NOT EXISTS idx_timestamp ON versions(timestamp);
	CREATE INDEX IF NOT EXISTS idx_file_hash ON versions(file_hash);
	CREATE INDEX IF NOT EXISTS idx_tags_version_id ON tags(version_id);
	CREATE INDEX IF NOT EXISTS idx_tags_name ON tags(tag_name);
	`

	_, err := dm.db.Exec(query)
	return err
}

// Close closes the database connection
func (dm *DatabaseManager) Close() error {
	if dm.db != nil {
		return dm.db.Close()
	}
	return nil
}

// GetDatabasePath returns the path to the database file
func (dm *DatabaseManager) GetDatabasePath() string {
	return dm.dbPath
}

// DatabaseExists checks if the database file exists
func (dm *DatabaseManager) DatabaseExists() bool {
	_, err := os.Stat(dm.dbPath)
	return !os.IsNotExist(err)
}

// AddFileVersion adds a new file version to the database
func (dm *DatabaseManager) AddFileVersion(fv *FileVersion) error {
	query := `
	INSERT INTO versions (file_path, version_number, timestamp, file_hash, file_size, storage_path, deleted)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := dm.db.Exec(query, fv.FilePath, fv.VersionNumber, fv.Timestamp.UTC().Format("2006-01-02 15:04:05"),
		fv.FileHash, fv.FileSize, fv.StoragePath, fv.Deleted)

	if err != nil {
		return fmt.Errorf("failed to add file version: %w", err)
	}

	return nil
}

// GetLatestFileVersion retrieves the latest version of a file from the database
func (dm *DatabaseManager) GetLatestFileVersion(filePath string) (*FileVersion, error) {
	// Convert to relative path for consistent storage
	relPath, err := filepath.Rel(dm.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	query := `
	SELECT id, file_path, version_number, timestamp, file_hash, file_size, storage_path, deleted
	FROM versions 
	WHERE file_path = ?
	ORDER BY version_number DESC
	LIMIT 1
	`

	row := dm.db.QueryRow(query, relPath)

	fv := &FileVersion{}
	var timestampStr string

	err = row.Scan(&fv.ID, &fv.FilePath, &fv.VersionNumber, &timestampStr,
		&fv.FileHash, &fv.FileSize, &fv.StoragePath, &fv.Deleted)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No version found
		}
		return nil, fmt.Errorf("failed to get latest file version: %w", err)
	}

	// Parse timestamp as UTC (since we stored it as UTC), then convert to local time
	fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	// Convert from UTC to local time for display
	fv.Timestamp = time.Date(fv.Timestamp.Year(), fv.Timestamp.Month(), fv.Timestamp.Day(),
		fv.Timestamp.Hour(), fv.Timestamp.Minute(), fv.Timestamp.Second(),
		fv.Timestamp.Nanosecond(), time.UTC).Local()

	return fv, nil
}

// GetNextVersionNumber returns the next version number for a file
func (dm *DatabaseManager) GetNextVersionNumber(filePath string) (int, error) {
	relPath, err := filepath.Rel(dm.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	query := `
	SELECT COALESCE(MAX(version_number), 0) + 1
	FROM versions 
	WHERE file_path = ?
	`

	var nextVersion int
	err = dm.db.QueryRow(query, relPath).Scan(&nextVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to get next version number: %w", err)
	}

	return nextVersion, nil
}

// CalculateFileHash calculates SHA256 hash of a file
func CalculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to calculate hash: %w", err)
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// CreateStoragePath creates a storage path for a file version
func (dm *DatabaseManager) CreateStoragePath(filePath string, versionNumber int) string {
	relPath, err := filepath.Rel(dm.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	// Remove any leading "./" or "\" from the relative path
	relPath = filepath.Clean(relPath)
	if strings.HasPrefix(relPath, "./") {
		relPath = relPath[2:]
	}

	now := time.Now()
	timestamp := now.Format("20060102_150405")

	return filepath.Join(relPath, fmt.Sprintf("v%d_%s", versionNumber, timestamp))
}

func (dm *DatabaseManager) GetAllLatestFiles() ([]*FileVersion, error) {
	query := `
	SELECT v.id, v.file_path, v.version_number, v.timestamp, v.file_hash, v.file_size, v.storage_path, v.deleted
		FROM versions v
		INNER JOIN (
			SELECT file_path, MAX(version_number) as max_version
			FROM versions
			GROUP BY file_path
		) latest ON v.file_path = latest.file_path AND v.version_number = latest.max_version
		ORDER BY v.file_path
	`

	rows, err := dm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query versioned files: %w", err)
	}
	defer rows.Close()

	var fileVersions []*FileVersion
	for rows.Next() {
		fv := &FileVersion{}
		var timestampStr string

		err := rows.Scan(&fv.ID, &fv.FilePath, &fv.VersionNumber, &timestampStr, &fv.FileHash, &fv.FileSize, &fv.StoragePath, &fv.Deleted)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file version: %w", err)
		}

		// Parse timestamp as UTC (since we stored it as UTC), then convert to local time
		fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}
		// Convert from UTC to local time for display
		fv.Timestamp = time.Date(fv.Timestamp.Year(), fv.Timestamp.Month(), fv.Timestamp.Day(),
			fv.Timestamp.Hour(), fv.Timestamp.Minute(), fv.Timestamp.Second(),
			fv.Timestamp.Nanosecond(), time.UTC).Local()

		fileVersions = append(fileVersions, fv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return fileVersions, nil
}

func (dm *DatabaseManager) GetFileVersions(absPath string) ([]*FileVersion, error) {

	relPath, err := filepath.Rel(dm.rootDir, absPath)
	if err != nil {
		relPath = absPath
	}

	relPath = filepath.Clean(relPath)
	if strings.HasPrefix(relPath, "./") {
		relPath = relPath[2:]
	}

	query := `
	SELECT v.id, v.file_path, v.version_number, v.timestamp, v.file_hash, v.file_size, v.storage_path, v.deleted
	FROM versions v
	WHERE v.file_path = ?
	ORDER BY v.version_number DESC
	`

	rows, err := dm.db.Query(query, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query versioned files: %w", err)
	}
	defer rows.Close()

	var fileVersions []*FileVersion
	for rows.Next() {
		fv := &FileVersion{}
		var timestampStr string

		err := rows.Scan(&fv.ID, &fv.FilePath, &fv.VersionNumber, &timestampStr, &fv.FileHash, &fv.FileSize, &fv.StoragePath, &fv.Deleted)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file version: %w", err)
		}

		// Parse timestamp as UTC (since we stored it as UTC), then convert to local time
		fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}
		// Convert from UTC to local time for display
		fv.Timestamp = time.Date(fv.Timestamp.Year(), fv.Timestamp.Month(), fv.Timestamp.Day(),
			fv.Timestamp.Hour(), fv.Timestamp.Minute(), fv.Timestamp.Second(),
			fv.Timestamp.Nanosecond(), time.UTC).Local()

		fileVersions = append(fileVersions, fv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return fileVersions, nil
}

func (dm *DatabaseManager) GetFileVersion(absPath string, version int) (*FileVersion, error) {
	relPath, err := filepath.Rel(dm.rootDir, absPath)
	if err != nil {
		relPath = absPath
	}

	query := `
	SELECT id, file_path, version_number, timestamp, file_hash, file_size, storage_path, deleted
	FROM versions 
	WHERE file_path = ? AND version_number = ?
	`

	row := dm.db.QueryRow(query, relPath, version)

	fv := &FileVersion{}
	var timestampStr string

	err = row.Scan(&fv.ID, &fv.FilePath, &fv.VersionNumber, &timestampStr,
		&fv.FileHash, &fv.FileSize, &fv.StoragePath, &fv.Deleted)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Version not found
		}
		return nil, fmt.Errorf("failed to get file version: %w", err)
	}

	// Parse timestamp as UTC (since we stored it as UTC), then convert to local time
	fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	// Convert from UTC to local time for display
	fv.Timestamp = time.Date(fv.Timestamp.Year(), fv.Timestamp.Month(), fv.Timestamp.Day(),
		fv.Timestamp.Hour(), fv.Timestamp.Minute(), fv.Timestamp.Second(),
		fv.Timestamp.Nanosecond(), time.UTC).Local()

	return fv, nil

}

// MarkFileDeleted marks the latest version of a file as deleted
func (dm *DatabaseManager) MarkFileDeleted(filePath string) error {
	// Convert to relative path for consistent storage
	relPath, err := filepath.Rel(dm.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	// Get the latest version to mark as deleted
	latestVersion, err := dm.GetLatestFileVersion(filePath)
	if err != nil {
		return fmt.Errorf("failed to get latest file version: %w", err)
	}

	if latestVersion == nil {
		return fmt.Errorf("no versions found for file: %s", relPath)
	}

	// Update the latest version to mark it as deleted
	query := `
	UPDATE versions 
	SET deleted = 1, timestamp = ?
	WHERE file_path = ? AND version_number = ?
	`

	_, err = dm.db.Exec(query, time.Now().UTC().Format("2006-01-02 15:04:05"), relPath, latestVersion.VersionNumber)
	if err != nil {
		return fmt.Errorf("failed to mark file as deleted: %w", err)
	}

	return nil
}

// GetAllDeletedFiles returns all files that are currently marked as deleted
func (dm *DatabaseManager) GetAllDeletedFiles() ([]*FileVersion, error) {
	query := `
	SELECT DISTINCT file_path, MAX(version_number) as version_number, timestamp, file_hash, file_size, storage_path 
	FROM versions 
	WHERE deleted = 1 
	GROUP BY file_path
	ORDER BY timestamp DESC
	`

	rows, err := dm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query deleted files: %w", err)
	}
	defer rows.Close()

	var deletedFiles []*FileVersion
	for rows.Next() {
		fv := &FileVersion{Deleted: true}
		var timestampStr string

		err := rows.Scan(&fv.FilePath, &fv.VersionNumber, &timestampStr, &fv.FileHash, &fv.FileSize, &fv.StoragePath)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deleted file row: %w", err)
		}

		// Parse timestamp as UTC (since we stored it as UTC), then convert to local time
		fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}
		// Convert from UTC to local time for display
		fv.Timestamp = time.Date(fv.Timestamp.Year(), fv.Timestamp.Month(), fv.Timestamp.Day(),
			fv.Timestamp.Hour(), fv.Timestamp.Minute(), fv.Timestamp.Second(),
			fv.Timestamp.Nanosecond(), time.UTC).Local()

		deletedFiles = append(deletedFiles, fv)
	}

	return deletedFiles, nil
}

// RestoreFile marks a deleted file as not deleted and returns the file version to restore
func (dm *DatabaseManager) RestoreFile(filePath string) (*FileVersion, error) {
	// Convert to relative path for consistent storage
	relPath, err := filepath.Rel(dm.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	// Get the latest version (should be deleted)
	latestVersion, err := dm.GetLatestFileVersion(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest file version: %w", err)
	}

	if latestVersion == nil {
		return nil, fmt.Errorf("no versions found for file: %s", relPath)
	}

	if !latestVersion.Deleted {
		return nil, fmt.Errorf("file is not deleted: %s", relPath)
	}

	// Update the latest version to mark it as not deleted
	query := `
	UPDATE versions 
	SET deleted = 0, timestamp = ?
	WHERE file_path = ? AND version_number = ?
	`

	_, err = dm.db.Exec(query, time.Now().UTC().Format("2006-01-02 15:04:05"), relPath, latestVersion.VersionNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to restore file in database: %w", err)
	}

	latestVersion.Deleted = false
	latestVersion.Timestamp = time.Now()

	return latestVersion, nil
}

// AddTag adds a tag to a specific version
func (dm *DatabaseManager) AddTag(filePath string, versionNumber int, tagName string) error {
	// Convert to relative path for consistent storage
	relPath, err := filepath.Rel(dm.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	// Get the version ID for the specified file and version number
	var versionID int64
	query := `SELECT id FROM versions WHERE file_path = ? AND version_number = ? AND deleted = 0`
	err = dm.db.QueryRow(query, relPath, versionNumber).Scan(&versionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("version %d not found for file %s", versionNumber, relPath)
		}
		return fmt.Errorf("failed to get version ID: %w", err)
	}

	// Insert the tag
	insertQuery := `INSERT INTO tags (version_id, tag_name, created_at) VALUES (?, ?, ?)`
	_, err = dm.db.Exec(insertQuery, versionID, tagName, time.Now().UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("tag '%s' already exists for version %d", tagName, versionNumber)
		}
		return fmt.Errorf("failed to add tag: %w", err)
	}

	return nil
}

// GetTagsForVersion returns all tags for a specific version
func (dm *DatabaseManager) GetTagsForVersion(filePath string, versionNumber int) ([]*Tag, error) {
	// Convert to relative path for consistent storage
	relPath, err := filepath.Rel(dm.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	query := `
	SELECT t.id, t.version_id, t.tag_name, t.created_at
	FROM tags t
	JOIN versions v ON t.version_id = v.id
	WHERE v.file_path = ? AND v.version_number = ? AND v.deleted = 0
	ORDER BY t.created_at ASC
	`

	rows, err := dm.db.Query(query, relPath, versionNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to query tags: %w", err)
	}
	defer rows.Close()

	var tags []*Tag
	for rows.Next() {
		tag := &Tag{}
		var createdAtStr string

		err := rows.Scan(&tag.ID, &tag.VersionID, &tag.TagName, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tag row: %w", err)
		}

		// Parse timestamp
		tag.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse tag timestamp: %w", err)
		}
		tag.CreatedAt = tag.CreatedAt.Local()

		tags = append(tags, tag)
	}

	return tags, nil
}

// GetVersionByTag returns a file version by tag name
func (dm *DatabaseManager) GetVersionByTag(filePath string, tagName string) (*FileVersion, error) {
	// Convert to relative path for consistent storage
	relPath, err := filepath.Rel(dm.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	query := `
	SELECT v.id, v.file_path, v.version_number, v.timestamp, v.file_hash, v.file_size, v.storage_path, v.deleted
	FROM versions v
	JOIN tags t ON v.id = t.version_id
	WHERE v.file_path = ? AND t.tag_name = ? AND v.deleted = 0
	`

	row := dm.db.QueryRow(query, relPath, tagName)

	fv := &FileVersion{}
	var timestampStr string
	err = row.Scan(&fv.ID, &fv.FilePath, &fv.VersionNumber, &timestampStr, &fv.FileHash, &fv.FileSize, &fv.StoragePath, &fv.Deleted)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no version found with tag '%s' for file %s", tagName, relPath)
		}
		return nil, fmt.Errorf("failed to get version by tag: %w", err)
	}

	// Parse timestamp
	fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	fv.Timestamp = fv.Timestamp.Local()

	return fv, nil
}

// GetAllTagsForFile returns all tags for all versions of a file
func (dm *DatabaseManager) GetAllTagsForFile(filePath string) (map[int][]*Tag, error) {
	// Convert to relative path for consistent storage
	relPath, err := filepath.Rel(dm.rootDir, filePath)
	if err != nil {
		relPath = filePath
	}

	query := `
	SELECT v.version_number, t.id, t.version_id, t.tag_name, t.created_at
	FROM tags t
	JOIN versions v ON t.version_id = v.id
	WHERE v.file_path = ? AND v.deleted = 0
	ORDER BY v.version_number, t.created_at ASC
	`

	rows, err := dm.db.Query(query, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query file tags: %w", err)
	}
	defer rows.Close()

	tagsByVersion := make(map[int][]*Tag)
	for rows.Next() {
		var versionNumber int
		tag := &Tag{}
		var createdAtStr string

		err := rows.Scan(&versionNumber, &tag.ID, &tag.VersionID, &tag.TagName, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tag row: %w", err)
		}

		// Parse timestamp
		tag.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse tag timestamp: %w", err)
		}
		tag.CreatedAt = tag.CreatedAt.Local()

		tagsByVersion[versionNumber] = append(tagsByVersion[versionNumber], tag)
	}

	return tagsByVersion, nil
}

// GetVersionsForPurge returns version IDs to be purged based on keep-last strategy
// Excludes tagged versions and ensures at least one version remains per file
func (dm *DatabaseManager) GetVersionsForPurge(keepLast int) ([]int64, error) {
	if keepLast < 1 {
		return nil, fmt.Errorf("keepLast must be at least 1")
	}

	query := `
	SELECT v.id, v.file_path, v.version_number, v.storage_path
	FROM versions v
	LEFT JOIN tags t ON v.id = t.version_id
	WHERE v.deleted = 0
	  AND t.version_id IS NULL
	ORDER BY v.file_path, v.version_number DESC
	`

	rows, err := dm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query versions: %w", err)
	}
	defer rows.Close()

	fileVersions := make(map[string][]int64)
	versionPaths := make(map[int64]string)

	for rows.Next() {
		var id int64
		var filePath, storagePath string
		var versionNumber int

		if err := rows.Scan(&id, &filePath, &versionNumber, &storagePath); err != nil {
			return nil, fmt.Errorf("failed to scan version row: %w", err)
		}

		fileVersions[filePath] = append(fileVersions[filePath], id)
		versionPaths[id] = storagePath
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	var versionsToPurge []int64

	for _, versions := range fileVersions {
		// Skip if file has fewer than or equal to keepLast versions
		if len(versions) <= keepLast {
			continue
		}

		// Keep first keepLast versions (they're already sorted DESC by version_number)
		// So we purge everything after index keepLast
		for i := keepLast; i < len(versions); i++ {
			versionsToPurge = append(versionsToPurge, versions[i])
		}
	}

	return versionsToPurge, nil
}

// GetVersionsForPurgeByAge returns version IDs to be purged based on age
// Excludes tagged versions and ensures at least one version remains per file
func (dm *DatabaseManager) GetVersionsForPurgeByAge(olderThan time.Time) ([]int64, error) {
	query := `
	SELECT v.id, v.file_path, v.timestamp
	FROM versions v
	LEFT JOIN tags t ON v.id = t.version_id
	WHERE v.deleted = 0
	  AND t.version_id IS NULL
	  AND v.timestamp < ?
	ORDER BY v.file_path, v.timestamp DESC
	`

	rows, err := dm.db.Query(query, olderThan.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("failed to query versions by age: %w", err)
	}
	defer rows.Close()

	fileVersions := make(map[string][]int64)
	
	for rows.Next() {
		var id int64
		var filePath, timestampStr string

		if err := rows.Scan(&id, &filePath, &timestampStr); err != nil {
			return nil, fmt.Errorf("failed to scan version row: %w", err)
		}

		fileVersions[filePath] = append(fileVersions[filePath], id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// Get total count per file to ensure we don't remove all versions
	fileCounts := make(map[string]int)
	countQuery := `
	SELECT file_path, COUNT(*) as total_count
	FROM versions
	WHERE deleted = 0
	GROUP BY file_path
	`
	
	countRows, err := dm.db.Query(countQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query version counts: %w", err)
	}
	defer countRows.Close()

	for countRows.Next() {
		var filePath string
		var count int
		if err := countRows.Scan(&filePath, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count row: %w", err)
		}
		fileCounts[filePath] = count
	}

	if err := countRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating count rows: %w", err)
	}

	var versionsToPurge []int64

	for filePath, versions := range fileVersions {
		totalCount := fileCounts[filePath]
		
		// Never remove all versions of a file - always keep at least 1
		if len(versions) >= totalCount {
			// Keep the newest version (first in DESC order)
			versionsToPurge = append(versionsToPurge, versions[1:]...)
		} else {
			// Safe to remove all old versions
			versionsToPurge = append(versionsToPurge, versions...)
		}
	}

	return versionsToPurge, nil
}

// GetVersionsForPurgeBySize returns version IDs to be purged to keep total size under maxSize
// Excludes tagged versions and ensures at least one version remains per file
func (dm *DatabaseManager) GetVersionsForPurgeBySize(maxSize int64) ([]int64, error) {
	// First get current total size
	totalSizeQuery := `
	SELECT COALESCE(SUM(file_size), 0) as total_size
	FROM versions
	WHERE deleted = 0
	`
	
	var currentSize int64
	err := dm.db.QueryRow(totalSizeQuery).Scan(&currentSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get current total size: %w", err)
	}
	
	// If we're already under the limit, nothing to purge
	if currentSize <= maxSize {
		return []int64{}, nil
	}
	
	// Get all versions ordered by timestamp (oldest first), excluding tagged versions
	query := `
	SELECT v.id, v.file_path, v.file_size, v.timestamp
	FROM versions v
	LEFT JOIN tags t ON v.id = t.version_id
	WHERE v.deleted = 0
	  AND t.version_id IS NULL
	ORDER BY v.timestamp ASC
	`
	
	rows, err := dm.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query versions by size: %w", err)
	}
	defer rows.Close()
	
	type versionInfo struct {
		id       int64
		filePath string
		size     int64
		timestamp string
	}
	
	var versions []versionInfo
	for rows.Next() {
		var v versionInfo
		if err := rows.Scan(&v.id, &v.filePath, &v.size, &v.timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan version row: %w", err)
		}
		versions = append(versions, v)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	// Get file counts to ensure we don't remove all versions of a file
	fileCounts := make(map[string]int)
	countQuery := `
	SELECT file_path, COUNT(*) as total_count
	FROM versions
	WHERE deleted = 0
	GROUP BY file_path
	`
	
	countRows, err := dm.db.Query(countQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query version counts: %w", err)
	}
	defer countRows.Close()
	
	for countRows.Next() {
		var filePath string
		var count int
		if err := countRows.Scan(&filePath, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count row: %w", err)
		}
		fileCounts[filePath] = count
	}
	
	if err := countRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating count rows: %w", err)
	}
	
	// Select versions to purge, starting with oldest
	var versionsToPurge []int64
	sizeToRemove := currentSize - maxSize
	removedSize := int64(0)
	filesToRemove := make(map[string]int) // Track how many versions we're removing per file
	
	for _, v := range versions {
		// Check if we can remove this version without leaving the file with 0 versions
		if filesToRemove[v.filePath]+1 >= fileCounts[v.filePath] {
			// Skip this version as it would leave the file with no versions
			continue
		}
		
		versionsToPurge = append(versionsToPurge, v.id)
		removedSize += v.size
		filesToRemove[v.filePath]++
		
		// Stop if we've removed enough to get under the limit
		if removedSize >= sizeToRemove {
			break
		}
	}
	
	return versionsToPurge, nil
}

// RemoveVersions removes specified versions from both database and filesystem
func (dm *DatabaseManager) RemoveVersions(versionIDs []int64) error {
	if len(versionIDs) == 0 {
		return nil
	}

	// First, get storage paths for file deletion
	storagePaths := make(map[int64]string)
	
	// Build placeholders for IN clause
	placeholders := strings.Repeat("?,", len(versionIDs)-1) + "?"
	query := fmt.Sprintf(`
	SELECT id, storage_path
	FROM versions
	WHERE id IN (%s)
	`, placeholders)

	args := make([]interface{}, len(versionIDs))
	for i, id := range versionIDs {
		args[i] = id
	}

	rows, err := dm.db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("failed to query storage paths: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var storagePath string
		if err := rows.Scan(&id, &storagePath); err != nil {
			return fmt.Errorf("failed to scan storage path: %w", err)
		}
		storagePaths[id] = storagePath
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating storage paths: %w", err)
	}

	// Delete physical files
	for _, storagePath := range storagePaths {
		fullPath := filepath.Join(dm.rootDir, ".rewind", "versions", storagePath)
		if err := os.Remove(fullPath); err != nil {
			// Log error but continue - the file might already be deleted
			fmt.Printf("Warning: failed to delete file %s: %v\n", fullPath, err)
		}
	}

	// Delete from database
	deleteQuery := fmt.Sprintf(`
	DELETE FROM versions
	WHERE id IN (%s)
	`, placeholders)

	_, err = dm.db.Exec(deleteQuery, args...)
	if err != nil {
		return fmt.Errorf("failed to delete versions from database: %w", err)
	}

	return nil
}
