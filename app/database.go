package app

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
