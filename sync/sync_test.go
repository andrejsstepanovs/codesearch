package sync

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/andrejsstepanovs/codesearch/db"
	"github.com/andrejsstepanovs/codesearch/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_FullRebuild(t *testing.T) {
	// Create a temporary directory for our test project
	tempDir := t.TempDir()

	// Create mock project files
	file1Path := filepath.Join(tempDir, "file1.go")
	err := os.WriteFile(file1Path, []byte("package main\n\nfunc main() {}"), 0644)
	require.NoError(t, err)

	file2Path := filepath.Join(tempDir, "file2.go")
	err = os.WriteFile(file2Path, []byte("package main\n\nfunc hello() {}"), 0644)
	require.NoError(t, err)

	// Set up config
	config := &Config{
		ProjectAlias: "test_project",
		ProjectPath:  tempDir,
		ModelName:    "codesearch-embedding",
		ClientName:   "litellm",
		Extensions:   []string{"go"},
	}

	deleteDbFile(t, config.ProjectAlias+".db")

	// Initialize database connection
	dbConn, err := db.SetupDatabase(config.ProjectAlias, 1536)
	require.NoError(t, err)
	defer dbConn.Close()

	// Pre-populate database with dummy files to simulate existing data
	// This simulates the scenario where we have old files in the database
	dummyEmbedding := make(models.Embedding, 1536)
	for i := range dummyEmbedding {
		dummyEmbedding[i] = 0.1
	}

	// Add dummy files that should be deleted during full rebuild
	_, err = db.SaveFileEmbedding(dbConn, "/dummy_old_file1.go", &dummyEmbedding)
	require.NoError(t, err)
	_, err = db.SaveFileEmbedding(dbConn, "/dummy_old_file2.go", &dummyEmbedding)
	require.NoError(t, err)

	// Verify dummy files were added
	var initialCount int
	err = dbConn.QueryRow("SELECT COUNT(*) FROM files").Scan(&initialCount)
	require.NoError(t, err)
	assert.Equal(t, 2, initialCount)

	// Run the sync process (should perform full rebuild)
	ctx := context.Background()
	err = Run(ctx, config)
	require.NoError(t, err)

	// Check that we have the correct number of files after rebuild
	// (2 new files from current run, old dummy files should be deleted)
	var finalCount int
	err = dbConn.QueryRow("SELECT COUNT(*) FROM files").Scan(&finalCount)
	require.NoError(t, err)
	assert.Equal(t, 2, finalCount)

	// Check that specific project files exist
	var file1Exists, file2Exists bool

	err = dbConn.QueryRow("SELECT file FROM files WHERE file = '/file1.go'").Scan(&file1Path)
	if err == nil {
		file1Exists = true
	}

	err = dbConn.QueryRow("SELECT file FROM files WHERE file = '/file2.go'").Scan(&file2Path)
	if err == nil {
		file2Exists = true
	}

	assert.True(t, file1Exists)
	assert.True(t, file2Exists)

	// Verify that dummy files were deleted
	var dummyFile1Exists, dummyFile2Exists bool

	err = dbConn.QueryRow("SELECT file FROM files WHERE file = '/dummy_old_file1.go'").Scan(&file1Path)
	if err == nil || err != sql.ErrNoRows {
		dummyFile1Exists = true
	}

	err = dbConn.QueryRow("SELECT file FROM files WHERE file = '/dummy_old_file2.go'").Scan(&file2Path)
	if err == nil || err != sql.ErrNoRows {
		dummyFile2Exists = true
	}

	assert.False(t, dummyFile1Exists)
	assert.False(t, dummyFile2Exists)

	// Add a new file and run again to test full rebuild still works
	file3Path := filepath.Join(tempDir, "file3.go")
	err = os.WriteFile(file3Path, []byte("package main\n\nfunc world() {}"), 0644)
	require.NoError(t, err)

	// Run sync again
	err = Run(ctx, config)
	require.NoError(t, err)

	// Check that we have the correct number of files (3 project files)
	err = dbConn.QueryRow("SELECT COUNT(*) FROM files").Scan(&finalCount)
	require.NoError(t, err)
	assert.Equal(t, 3, finalCount)

	// Verify old project files and dummy files are gone
	var fileCount int
	err = dbConn.QueryRow("SELECT COUNT(*) FROM files WHERE file IN ('/file1.go', '/file2.go')").Scan(&fileCount)
	require.NoError(t, err)
	assert.Equal(t, 2, fileCount) // file1.go and file2.go should still exist

	err = dbConn.QueryRow("SELECT COUNT(*) FROM files WHERE file = '/file3.go'").Scan(&fileCount)
	require.NoError(t, err)
	assert.Equal(t, 1, fileCount) // file3.go should exist

	err = dbConn.QueryRow("SELECT COUNT(*) FROM files WHERE file LIKE '/dummy_old_%'").Scan(&fileCount)
	require.NoError(t, err)
	assert.Equal(t, 0, fileCount) // dummy files should be deleted
}

func deleteDbFile(t *testing.T, file string) {
	if _, err := os.Stat(file); err == nil {
		err = os.Remove(file)
		require.NoError(t, err, "Failed to remove existing test.db file")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Unexpected error checking for test.db: %v", err)
	}
}
