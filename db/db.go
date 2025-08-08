package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/andrejsstepanovs/codesearch/models"
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func InitDB(name string, dimensions int) (*sql.DB, error) {
	sqlite_vec.Auto()

	db, err := sql.Open("sqlite3", fmt.Sprintf("%s.db", name))
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}

	_, err = db.Exec(`
				CREATE TABLE IF NOT EXISTS files (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					file TEXT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
			`)
	if err != nil {
		return nil, fmt.Errorf("error creating files table: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			alias TEXT PRIMARY KEY NOT NULL,
			path TEXT NOT NULL,
			client TEXT NOT NULL,
			model TEXT NOT NULL,
			extensions TEXT NOT NULL DEFAULT ''
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("error creating projects table: %w", err)
	}

	_, err = db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS context_vectors USING vec0(
			embedding float[` + fmt.Sprintf("%d", dimensions) + `],
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("error creating context_vectors table: %w", err)
	}

	return db, nil
}

func SaveFileEmbedding(db *sql.DB, file string, embedding *models.Embedding) (int64, error) {
	result, err := db.Exec("INSERT INTO files (file) VALUES (?)", file)
	if err != nil {
		return 0, fmt.Errorf("failed to insert err: %w", err)
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id err: %w", err)
	}

	embeddingBytes, errSerialize := sqlite_vec.SerializeFloat32(embedding.Float32())
	if errSerialize != nil {
		return 0, fmt.Errorf("failed to serialize embedding: %w", errSerialize)
	}

	_, err = db.Exec("INSERT INTO context_vectors (rowid, embedding) VALUES (?, vec_f32(?))",
		lastID,
		embeddingBytes,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert into context_vectors: %w", err)
	}

	return lastID, nil
}

func UpdateFileEmbedding(db *sql.DB, fileID int64, filePath string, newEmbedding *models.Embedding) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Delete the old vector first
	_, err = tx.Exec("DELETE FROM context_vectors WHERE rowid = ?", fileID)
	if err != nil {
		return fmt.Errorf("failed to delete vector for fileID %d: %w", fileID, err)
	}

	// Delete the old file record
	_, err = tx.Exec("DELETE FROM files WHERE id = ?", fileID)
	if err != nil {
		return fmt.Errorf("failed to delete file record for fileID %d: %w", fileID, err)
	}

	// Insert the new file record
	result, err := tx.Exec("INSERT INTO files (file) VALUES (?)", filePath)
	if err != nil {
		return fmt.Errorf("failed to insert new file record: %w", err)
	}

	newID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	// Serialize the new embedding
	embeddingBytes, err := sqlite_vec.SerializeFloat32(newEmbedding.Float32())
	if err != nil {
		return fmt.Errorf("failed to serialize embedding: %w", err)
	}

	// Insert the new vector
	_, err = tx.Exec("INSERT INTO context_vectors (rowid, embedding) VALUES (?, vec_f32(?))",
		newID,
		embeddingBytes,
	)
	if err != nil {
		return fmt.Errorf("failed to insert new vector: %w", err)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func DeleteFileAndVector(db *sql.DB, fileID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Delete the vector first
	_, err = tx.Exec("DELETE FROM context_vectors WHERE rowid = ?", fileID)
	if err != nil {
		return fmt.Errorf("failed to delete vector for fileID %d: %w", fileID, err)
	}

	// Delete the file record
	_, err = tx.Exec("DELETE FROM files WHERE id = ?", fileID)
	if err != nil {
		return fmt.Errorf("failed to delete file record for fileID %d: %w", fileID, err)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func DeleteVectorData(db *sql.DB) error {
	_, err := db.Exec("DELETE FROM files")
	if err != nil {
		return fmt.Errorf("failed to delete from files: %w", err)
	}

	_, err = db.Exec("DELETE FROM context_vectors")
	if err != nil {
		return fmt.Errorf("failed to delete from context_vectors: %w", err)
	}

	return nil
}

func DeleteAll(db *sql.DB) error {
	return DeleteVectorData(db)
}

func UpsertProject(db *sql.DB, project models.Project) error {
	query := `
		INSERT INTO projects (alias, path, client, model, extensions) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET path = excluded.path, client = excluded.client, model = excluded.model, extensions = excluded.extensions;
	`
	extensionsStr := strings.Join(project.Extensions, ",")
	_, err := db.Exec(query, project.Alias, project.Path, project.Client, project.Model, extensionsStr)
	if err != nil {
		return fmt.Errorf("failed to upsert project with alias '%s': %w", project.Alias, err)
	}
	return nil
}

func GetProjectByAlias(db *sql.DB, alias string) (*models.Project, error) {
	query := `
		SELECT alias, path, client, model, extensions FROM projects WHERE alias = ?
	`
	row := db.QueryRow(query, alias)

	var project models.Project
	var extensionsStr string
	err := row.Scan(&project.Alias, &project.Path, &project.Client, &project.Model, &extensionsStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get project with alias '%s': %w", alias, err)
	}

	if extensionsStr != "" {
		project.Extensions = strings.Split(extensionsStr, ",")
	} else {
		project.Extensions = []string{}
	}

	return &project, nil
}

func SetupDatabase(projectAlias string, dimensions int) (*sql.DB, error) {
	dbConn, err := InitDB(projectAlias, dimensions)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	return dbConn, nil
}

func GetFilesToSync(db *sql.DB) ([]models.File, error) {
	query := `SELECT id, file, created_at FROM files ORDER BY created_at ASC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query files for sync: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log the error or handle it as appropriate for your application
		}
	}()

	var files []models.File
	for rows.Next() {
		var file models.File
		// The driver will handle DATETIME -> time.Time conversion
		if err := rows.Scan(&file.ID, &file.File, &file.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during file row iteration: %w", err)
	}

	return files, nil
}

func GetProjectFilePaths(db *sql.DB) (map[string]bool, error) {
	filePaths := make(map[string]bool)

	rows, err := db.Query("SELECT file FROM files")
	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log the error or handle it as appropriate for your application
		}
	}()

	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("failed to scan file path: %w", err)
		}
		filePaths[path] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during file path iteration: %w", err)
	}

	return filePaths, nil
}
