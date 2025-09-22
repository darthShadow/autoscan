package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteIntegration(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "sqlite_integration_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	// Create database
	dbPath := filepath.Join(tempDir, "test_integration.db")
	ctx := context.Background()

	t.Logf("Creating SQLite database at: %s", dbPath)
	db, err := NewDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	// Test basic functionality
	if got := db.Path(); got != dbPath {
		t.Errorf("Expected path %s, got %s", dbPath, got)
	}

	// Check connection stats to verify optimization
	rwStats := db.RW().Stats()
	roStats := db.RO().Stats()

	t.Logf("RW Connection - MaxOpenConns: %d, Idle: %d, InUse: %d",
		rwStats.MaxOpenConnections, rwStats.Idle, rwStats.InUse)
	t.Logf("RO Connection - MaxOpenConns: %d, Idle: %d, InUse: %d",
		roStats.MaxOpenConnections, roStats.Idle, roStats.InUse)

	// Verify connection optimization settings
	if rwStats.MaxOpenConnections != 1 {
		t.Errorf("Expected RW MaxOpenConnections to be 1, got %d", rwStats.MaxOpenConnections)
	}

	// RO connections should be optimized based on CPU count (2-8 range)
	if roStats.MaxOpenConnections < 2 || roStats.MaxOpenConnections > 8 {
		t.Errorf("Expected RO MaxOpenConnections to be between 2-8, got %d", roStats.MaxOpenConnections)
	}

	// Test ping
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	t.Log("✅ SQLite integration test passed!")
}
