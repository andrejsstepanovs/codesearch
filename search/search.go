package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/andrejsstepanovs/codesearch/client"
	"github.com/andrejsstepanovs/codesearch/db"
)

// Config holds the configuration for a search operation.
type Config struct {
	ProjectAlias string
	Query        string
}

// ParseConfig parses command line arguments into a Config struct.
func ParseConfig(args []string) (*Config, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("at least 2 arguments required (alias, query)")
	}

	config := &Config{
		ProjectAlias: args[0],
		Query:        strings.Join(args[1:], " "),
	}

	config.Query = strings.TrimSpace(config.Query)
	if config.Query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}

	return config, nil
}

// Run executes a search operation based on the provided config.
func Run(ctx context.Context, config *Config) ([]db.SearchResult, error) {
	dbConn, err := db.SetupDatabase(config.ProjectAlias, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	defer dbConn.Close()

	// Validate project exists
	proj, err := db.GetProjectByAlias(dbConn, config.ProjectAlias)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("project with alias '%s' not found", config.ProjectAlias)
		}
		return nil, fmt.Errorf("error retrieving project: %w", err)
	}

	embedding, err := client.Embeddings(ctx, proj.Client, proj.Model, config.Query)
	if err != nil {
		return nil, fmt.Errorf("error generating embeddings for query: %w", err)
	}

	minSimilarity := 0.03
	limit := 10
	results, err := db.SearchWithSimilarity(dbConn, embedding.GetEmbeddings().Float32(), minSimilarity, limit)
	if err != nil {
		return nil, fmt.Errorf("error searching for similar files: %w", err)
	}

	return results, nil
}
