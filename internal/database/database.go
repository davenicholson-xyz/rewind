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

	CREATE INDEX IF NOT EXISTS idx_file_path ON versions(file_path);
	CREATE INDEX IF NOT EXISTS idx_timestamp ON versions(timestamp);
	CREATE INDEX IF NOT EXISTS idx_file_hash ON versions(file_hash);
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

	_, err := dm.db.Exec(query, fv.FilePath, fv.VersionNumber, fv.Timestamp.Format("2006-01-02 15:04:05"),
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

	// Parse timestamp
	fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}

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

		// Parse timestamp
		fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}

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

		fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}

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

	// Parse timestamp
	fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}

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

	_, err = dm.db.Exec(query, time.Now().Format("2006-01-02 15:04:05"), relPath, latestVersion.VersionNumber)
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

		fv.Timestamp, err = time.Parse("2006-01-02 15:04:05", timestampStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}

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

	_, err = dm.db.Exec(query, time.Now().Format("2006-01-02 15:04:05"), relPath, latestVersion.VersionNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to restore file in database: %w", err)
	}

	latestVersion.Deleted = false
	latestVersion.Timestamp = time.Now()

	return latestVersion, nil
}
