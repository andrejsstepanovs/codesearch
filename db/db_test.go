package db

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/andrejsstepanovs/codesearch/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertProject(t *testing.T) {
	deleteDbFile(t, "test2.db")
	db, err := InitDB("test2", 1536)
	require.NoError(t, err)
	defer db.Close()

	testCases := []struct {
		name          string
		project       models.Project
		expectedPath  string
		expectedModel string
		expectedExts  []string
	}{
		{
			name:          "insert new project",
			project:       models.Project{Alias: "proj-a", Path: "/path/to/a", Model: "model-1", Extensions: []string{".go", ".mod"}},
			expectedPath:  "/path/to/a",
			expectedModel: "model-1",
			expectedExts:  []string{".go", ".mod"},
		},
		{
			name:          "update existing project",
			project:       models.Project{Alias: "proj-a", Path: "/new/path/a", Model: "model-2", Extensions: []string{".py", ".txt"}},
			expectedPath:  "/new/path/a",
			expectedModel: "model-2",
			expectedExts:  []string{".py", ".txt"},
		},
		{
			name:          "project with empty extensions",
			project:       models.Project{Alias: "proj-b", Path: "/path/to/b", Model: "model-3", Extensions: []string{}},
			expectedPath:  "/path/to/b",
			expectedModel: "model-3",
			expectedExts:  []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := UpsertProject(db, tc.project)
			assert.NoError(t, err)

			// Verification logic: Query the DB for the project by alias
			// and assert that its path and model match the expected values.
			var path, model, extensionsStr string
			row := db.QueryRow("SELECT path, model, extensions FROM projects WHERE alias = ?", tc.project.Alias)
			err = row.Scan(&path, &model, &extensionsStr)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedPath, path)
			assert.Equal(t, tc.expectedModel, model)

			if len(tc.expectedExts) > 0 {
				assert.Equal(t, strings.Join(tc.expectedExts, ","), extensionsStr)
			} else {
				assert.Equal(t, "", extensionsStr)
			}
		})
	}
}

func TestGetProjectByAlias(t *testing.T) {
	deleteDbFile(t, "test3.db")
	db, err := InitDB("test3", 1536)
	require.NoError(t, err)
	defer db.Close()

	// Seed data
	seedProject := models.Project{Alias: "test-proj", Path: "/path/to/test", Model: "test-model", Extensions: []string{".go", ".mod"}}
	err = UpsertProject(db, seedProject)
	require.NoError(t, err)

	// Seed data with empty extensions
	seedProjectEmpty := models.Project{Alias: "test-proj-empty", Path: "/path/to/test2", Model: "test-model2", Extensions: []string{}}
	err = UpsertProject(db, seedProjectEmpty)
	require.NoError(t, err)

	testCases := []struct {
		name            string
		alias           string
		expectErr       error
		expectedProject *models.Project
	}{
		{
			name:      "found project",
			alias:     "test-proj",
			expectErr: nil,
			expectedProject: &models.Project{
				Alias:      "test-proj",
				Path:       "/path/to/test",
				Model:      "test-model",
				Extensions: []string{".go", ".mod"},
			},
		},
		{
			name:      "found project with empty extensions",
			alias:     "test-proj-empty",
			expectErr: nil,
			expectedProject: &models.Project{
				Alias:      "test-proj-empty",
				Path:       "/path/to/test2",
				Model:      "test-model2",
				Extensions: []string{},
			},
		},
		{
			name:            "not found project",
			alias:           "not-found-proj",
			expectErr:       sql.ErrNoRows,
			expectedProject: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			project, err := GetProjectByAlias(db, tc.alias)

			if tc.expectErr != nil {
				assert.ErrorIs(t, err, tc.expectErr)
				assert.Nil(t, project)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, project)
				assert.Equal(t, tc.expectedProject.Alias, project.Alias)
				assert.Equal(t, tc.expectedProject.Path, project.Path)
				assert.Equal(t, tc.expectedProject.Model, project.Model)
				assert.Equal(t, tc.expectedProject.Extensions, project.Extensions)
			}
		})
	}
}

func TestDeleteVectorData(t *testing.T) {
	deleteDbFile(t, "test.db")
	// Setup: Create a temporary in-memory DB and initialize the schema
	db, err := InitDB("test", 1536)
	require.NoError(t, err)
	defer db.Close()

	// Seed project data
	project := models.Project{Alias: "test-proj", Path: "/path/to/test", Model: "test-model", Extensions: []string{".go", ".mod"}}
	err = UpsertProject(db, project)
	require.NoError(t, err)

	// Seed file and vector data
	embedding := make(models.Embedding, 1536)
	for i := range embedding {
		embedding[i] = float64(i)
	}

	// Convert embedding to float32 slice for serialization
	embedding32 := make([]float32, len(embedding))
	for i, v := range embedding {
		embedding32[i] = float32(v)
	}

	_, err = SaveFileEmbedding(db, "/path/to/file1.go", &embedding)
	require.NoError(t, err)

	_, err = SaveFileEmbedding(db, "/path/to/file2.go", &embedding)
	require.NoError(t, err)

	// Verify initial state
	var projectCount, fileCount, vectorCount int
	row := db.QueryRow("SELECT COUNT(*) as projects FROM projects")
	err = row.Scan(&projectCount)
	require.NoError(t, err)
	assert.Equal(t, 1, projectCount)

	err = db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)
	require.NoError(t, err)
	assert.Equal(t, 2, fileCount)

	err = db.QueryRow("SELECT COUNT(*) FROM context_vectors").Scan(&vectorCount)
	require.NoError(t, err)
	assert.Equal(t, 2, vectorCount)

	// Execute DeleteVectorData
	err = DeleteVectorData(db)
	assert.NoError(t, err)

	// Verify final state
	err = db.QueryRow("SELECT COUNT(*) FROM projects").Scan(&projectCount)
	require.NoError(t, err)
	assert.Equal(t, 1, projectCount) // Projects should be preserved

	err = db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)
	require.NoError(t, err)
	assert.Equal(t, 0, fileCount) // Files should be deleted

	err = db.QueryRow("SELECT COUNT(*) FROM context_vectors").Scan(&vectorCount)
	require.NoError(t, err)
	assert.Equal(t, 0, vectorCount) // Vectors should be deleted
}

func TestUpdateFileEmbedding(t *testing.T) {
	deleteDbFile(t, "test_update_embedding.db")
	db, err := InitDB("test_update_embedding", 1536)
	require.NoError(t, err)
	defer db.Close()

	// Create test embedding
	embedding := make(models.Embedding, 1536)
	for i := range embedding {
		embedding[i] = float64(i)
	}

	// Create another embedding for update
	newEmbedding := make(models.Embedding, 1536)
	for i := range newEmbedding {
		newEmbedding[i] = float64(i) + 0.5
	}

	// Test case 1: Successful update
	t.Run("successful update", func(t *testing.T) {
		// Insert initial file and embedding
		fileID, err := SaveFileEmbedding(db, "/path/to/old_file.go", &embedding)
		require.NoError(t, err)
		require.Equal(t, int64(1), fileID)

		// Verify initial state
		var fileCount, vectorCount int
		err = db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)
		require.NoError(t, err)
		assert.Equal(t, 1, fileCount)

		err = db.QueryRow("SELECT COUNT(*) FROM context_vectors").Scan(&vectorCount)
		require.NoError(t, err)
		assert.Equal(t, 1, vectorCount)

		// Perform update
		err = UpdateFileEmbedding(db, fileID, "/path/to/new_file.go", &newEmbedding)
		assert.NoError(t, err)

		// Verify final state
		err = db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)
		require.NoError(t, err)
		assert.Equal(t, 1, fileCount)

		err = db.QueryRow("SELECT COUNT(*) FROM context_vectors").Scan(&vectorCount)
		require.NoError(t, err)
		assert.Equal(t, 1, vectorCount)

		// Check that the file was updated
		var filePath string
		var newFileID int64
		row := db.QueryRow("SELECT id, file FROM files WHERE id = ?", fileID)
		err = row.Scan(&newFileID, &filePath)
		assert.Error(t, err) // The old fileID should no longer exist

		// Check that the new file exists
		row = db.QueryRow("SELECT id, file FROM files WHERE file = ?", "/path/to/new_file.go")
		err = row.Scan(&newFileID, &filePath)
		assert.NoError(t, err)
		assert.Equal(t, "/path/to/new_file.go", filePath)
		assert.NotEqual(t, fileID, newFileID) // Should have a new ID
	})
}

func deleteDbFile(t *testing.T, file string) {
	if _, err := os.Stat(file); err == nil {
		err = os.Remove(file)
		require.NoError(t, err, "Failed to remove existing test.db file")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Unexpected error checking for test.db: %v", err)
	}
}

func TestGetFilesToSync(t *testing.T) {
	deleteDbFile(t, "test_files_sync.db")
	db, err := InitDB("test_files_sync", 1536)
	require.NoError(t, err)
	defer db.Close()

	// Test case 1: Empty table
	files, err := GetFilesToSync(db)
	assert.NoError(t, err)
	assert.Empty(t, files)

	// Seed data with specific timestamps to test ordering
	_, err = db.Exec("INSERT INTO files (file, created_at) VALUES (?, ?)", "/path/to/file1.go", "2023-01-01 10:00:00")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO files (file, created_at) VALUES (?, ?)", "/path/to/file2.go", "2023-01-01 09:00:00")
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO files (file, created_at) VALUES (?, ?)", "/path/to/file3.go", "2023-01-01 11:00:00")
	require.NoError(t, err)

	// Test case 2: Multiple files ordered by created_at
	files, err = GetFilesToSync(db)
	assert.NoError(t, err)
	require.Len(t, files, 3)

	// Check that files are ordered by created_at ASC
	assert.Equal(t, "/path/to/file2.go", files[0].File) // Earliest timestamp
	assert.Equal(t, "/path/to/file1.go", files[1].File) // Middle timestamp
	assert.Equal(t, "/path/to/file3.go", files[2].File) // Latest timestamp

	// Check that IDs are correctly assigned
	assert.Equal(t, int64(2), files[0].ID)
	assert.Equal(t, int64(1), files[1].ID)
	assert.Equal(t, int64(3), files[2].ID)

	// Check that CreatedAt is properly parsed
	expectedTime1, _ := time.Parse("2006-01-02 15:04:05", "2023-01-01 09:00:00")
	expectedTime2, _ := time.Parse("2006-01-02 15:04:05", "2023-01-01 10:00:00")
	expectedTime3, _ := time.Parse("2006-01-02 15:04:05", "2023-01-01 11:00:00")

	assert.Equal(t, expectedTime1.Unix(), files[0].CreatedAt.Unix())
	assert.Equal(t, expectedTime2.Unix(), files[1].CreatedAt.Unix())
	assert.Equal(t, expectedTime3.Unix(), files[2].CreatedAt.Unix())
}

func TestGetProjectFilePaths(t *testing.T) {
	deleteDbFile(t, "test_file_paths.db")
	db, err := InitDB("test_file_paths", 1536)
	require.NoError(t, err)
	defer db.Close()

	// Test case 1: Empty database
	filePaths, err := GetProjectFilePaths(db)
	assert.NoError(t, err)
	assert.NotNil(t, filePaths)
	assert.Empty(t, filePaths)

	// Seed data
	embedding := make(models.Embedding, 1536)
	for i := range embedding {
		embedding[i] = float64(i)
	}

	_, err = SaveFileEmbedding(db, "/path/to/file1.go", &embedding)
	require.NoError(t, err)

	_, err = SaveFileEmbedding(db, "/path/to/file2.go", &embedding)
	require.NoError(t, err)

	_, err = SaveFileEmbedding(db, "/path/to/file3.go", &embedding)
	require.NoError(t, err)

	// Test case 2: Populated database
	filePaths, err = GetProjectFilePaths(db)
	assert.NoError(t, err)
	assert.Len(t, filePaths, 3)
	assert.True(t, filePaths["/path/to/file1.go"])
	assert.True(t, filePaths["/path/to/file2.go"])
	assert.True(t, filePaths["/path/to/file3.go"])
}

func TestDeleteFileAndVector(t *testing.T) {
	deleteDbFile(t, "test_delete_file_vector.db")
	db, err := InitDB("test_delete_file_vector", 1536)
	require.NoError(t, err)
	defer db.Close()

	// Create test embedding
	embedding := make(models.Embedding, 1536)
	for i := range embedding {
		embedding[i] = float64(i)
	}

	// Insert a file and its vector
	fileID, err := SaveFileEmbedding(db, "/path/to/test_file.go", &embedding)
	require.NoError(t, err)
	require.Equal(t, int64(1), fileID)

	// Verify initial state
	var fileCount, vectorCount int
	err = db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)
	require.NoError(t, err)
	assert.Equal(t, 1, fileCount)

	err = db.QueryRow("SELECT COUNT(*) FROM context_vectors").Scan(&vectorCount)
	require.NoError(t, err)
	assert.Equal(t, 1, vectorCount)

	// Delete the file and vector
	err = DeleteFileAndVector(db, fileID)
	assert.NoError(t, err)

	// Verify final state
	err = db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)
	require.NoError(t, err)
	assert.Equal(t, 0, fileCount)

	err = db.QueryRow("SELECT COUNT(*) FROM context_vectors").Scan(&vectorCount)
	require.NoError(t, err)
	assert.Equal(t, 0, vectorCount)
}
