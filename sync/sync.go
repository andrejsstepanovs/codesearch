package sync

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrejsstepanovs/codesearch/client"
	"github.com/andrejsstepanovs/codesearch/db"
	"github.com/andrejsstepanovs/codesearch/file"
	"github.com/andrejsstepanovs/codesearch/models"
)

type Config struct {
	ProjectAlias string
	ProjectPath  string
	ClientName   string
	ModelName    string
	Extensions   []string
}

func ParseConfig(args []string) (*Config, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("at least 2 arguments required (alias, path)")
	}

	config := &Config{
		ProjectAlias: args[0],
		ProjectPath:  args[1],
		ModelName:    "codesearch-embedding",
		ClientName:   "litellm",
		Extensions:   []string{"go", "js", "ts", "py", "java", "cpp", "c", "h", "hpp", "yaml", "yml"},
	}

	if config.ProjectPath == "." {
		var err error
		config.ProjectPath, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current working directory: %w", err)
		}
	}

	if len(args) >= 4 && args[2] != "" {
		config.ClientName = args[2]
	}

	if len(args) >= 4 && args[3] != "" {
		config.ModelName = args[3]
	}

	if len(args) >= 5 {
		config.Extensions = strings.Split(args[4], ",")
	}

	return config, nil
}

func processProjectFiles(ctx context.Context, dbConn *sql.DB, config *Config) error {
	log.Println("Syncing code files to the database")
	files, err := file.RecursiveFiles(config.ProjectPath, config.Extensions)
	if err != nil {
		return fmt.Errorf("error finding files: %w", err)
	}

	log.Printf("Found %d files", len(files))

	for i, filePath := range files {
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("Error reading file %s: %v", filePath, err)
			continue
		}
		relativePath := strings.TrimPrefix(filePath, config.ProjectPath)

		log.Printf("Processing file: %s", relativePath)
		embed := fmt.Sprintf("%s\n%s", relativePath, string(content))
		res, err := client.Embeddings(ctx, config.ClientName, config.ModelName, embed)
		if err != nil {
			log.Printf("Error generating embeddings for file %s: %v", filePath, err)
			continue
		}

		_, err = db.SaveFileEmbedding(dbConn, relativePath, res.GetEmbeddings())
		if err != nil {
			return fmt.Errorf("error saving embedding for file %s: %w", filePath, err)
		}

		percentage := float64(i+1) / float64(len(files)) * 100
		fmt.Printf("Progress: %.2f%%\n", percentage)
	}

	return nil
}

// Run builds
func Run(ctx context.Context, config *Config) error {
	res, err := client.Embeddings(ctx, config.ClientName, config.ModelName, "1")
	if err != nil {
		return fmt.Errorf("error generating embedding for dimensions: %w", err)
	}
	dimensions := len(res.GetEmbeddings().Float32())
	if dimensions == 0 {
		return fmt.Errorf("received empty embedding dimensions")
	}

	dbConn, err := db.SetupDatabase(config.ProjectAlias, dimensions)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer dbConn.Close()

	// Save project metadata
	project := models.Project{
		Alias:      config.ProjectAlias,
		Path:       config.ProjectPath,
		Client:     config.ClientName,
		Model:      config.ModelName,
		Extensions: config.Extensions,
	}

	err = db.UpsertProject(dbConn, project)
	if err != nil {
		return fmt.Errorf("error saving project metadata: %w", err)
	}

	err = db.DeleteVectorData(dbConn)
	if err != nil {
		return fmt.Errorf("error deleting existing vector data: %w", err)
	}

	err = processProjectFiles(ctx, dbConn, config)
	if err != nil {
		return fmt.Errorf("error processing project files: %w", err)
	}

	fmt.Printf("Project '%s' built successfully\n", config.ProjectAlias)
	return nil
}

// RunSync runs a sync operation using stored project configuration
func RunSync(ctx context.Context, projectAlias string) error {
	dbConn, err := db.SetupDatabase(projectAlias, 0)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer dbConn.Close()

	project, err := db.GetProjectByAlias(dbConn, projectAlias)
	if err != nil {
		return fmt.Errorf("failed to get project config for alias '%s': %w", projectAlias, err)
	}

	config := &Config{
		ProjectAlias: project.Alias,
		ProjectPath:  project.Path,
		ModelName:    project.Model,
		ClientName:   project.Client,
		Extensions:   project.Extensions,
	}

	// Fetch all local files
	localFiles, err := file.RecursiveFiles(config.ProjectPath, config.Extensions)
	if err != nil {
		return fmt.Errorf("error finding local files: %w", err)
	}

	// Create a map of local file paths for quick lookup
	localFilePaths := make(map[string]bool)
	for _, filePath := range localFiles {
		relativePath := strings.TrimPrefix(filePath, config.ProjectPath)
		localFilePaths[relativePath] = true
	}

	// Fetch all existing file paths from database
	existingFilePaths, err := db.GetProjectFilePaths(dbConn)
	if err != nil {
		return fmt.Errorf("error getting existing file paths: %w", err)
	}

	// Process new files
	log.Println("Processing new files")
	for _, filePath := range localFiles {
		relativePath := strings.TrimPrefix(filePath, config.ProjectPath)

		// Check if file is new (not in database)
		if !existingFilePaths[relativePath] {
			log.Printf("Adding new file: %s", relativePath)

			content, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("Error reading file %s: %v", filePath, err)
				continue
			}

			embed := fmt.Sprintf("%s\n%s", relativePath, string(content))
			res, err := client.Embeddings(ctx, config.ClientName, config.ModelName, embed)
			if err != nil {
				log.Printf("Error generating embeddings for file %s: %v", filePath, err)
				continue
			}

			_, err = db.SaveFileEmbedding(dbConn, relativePath, res.GetEmbeddings())
			if err != nil {
				return fmt.Errorf("error saving embedding for file %s: %w", filePath, err)
			}
		}
	}

	// Process existing files in order of creation date
	log.Println("Processing existing files")
	existingFiles, err := db.GetFilesToSync(dbConn)
	if err != nil {
		return fmt.Errorf("error getting files to sync: %w", err)
	}

	for _, fileRecord := range existingFiles {
		// Check if file still exists locally
		if localFilePaths[fileRecord.File] {
			// File exists, update it
			log.Printf("Updating file: %s", fileRecord.File)

			fullPath := filepath.Join(config.ProjectPath, fileRecord.File)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				log.Printf("Error reading file %s: %v", fullPath, err)
				continue
			}

			embed := fmt.Sprintf("%s\n%s", fileRecord.File, string(content))
			res, err := client.Embeddings(ctx, config.ClientName, config.ModelName, embed)
			if err != nil {
				log.Printf("Error generating embeddings for file %s: %v", fullPath, err)
				continue
			}

			err = db.UpdateFileEmbedding(dbConn, fileRecord.ID, fileRecord.File, res.GetEmbeddings())
			if err != nil {
				return fmt.Errorf("error updating embedding for file %s: %w", fullPath, err)
			}
		} else {
			// File was deleted, remove from database
			log.Printf("Removing deleted file: %s", fileRecord.File)

			err := db.DeleteFileAndVector(dbConn, fileRecord.ID)
			if err != nil {
				return fmt.Errorf("error deleting file and vector for file %s: %w", fileRecord.File, err)
			}
		}
	}

	fmt.Printf("Project '%s' synced successfully\n", config.ProjectAlias)
	return nil
}
