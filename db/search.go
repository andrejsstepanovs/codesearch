package db

import (
	"database/sql"
	"fmt"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

// SearchResult represents a search result with distance information
type SearchResult struct {
	ID       int
	File     string
	Distance float64
}

// SearchOptions provides flexible search configuration
type SearchOptions struct {
	MaxDistance float64 // Maximum distance threshold (e.g., 0.7)
	MinResults  int     // Minimum number of results to return
	MaxResults  int     // Maximum number of results to return
	UseAdaptive bool    // Use adaptive threshold based on result distribution
}

// DefaultSearchOptions returns sensible defaults
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		MaxDistance: 0.8, // Cosine similarity threshold
		MinResults:  2,
		MaxResults:  20,
		UseAdaptive: true,
	}
}

// Search with distance threshold instead of fixed limit
func SearchWithThreshold(db *sql.DB, embeddings []float32, opts SearchOptions) ([]SearchResult, error) {
	embeddingBytes, err := sqlite_vec.SerializeFloat32(embeddings)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize embedding: %w", err)
	}

	// Get a larger initial set to analyze distances
	initialLimit := opts.MaxResults * 2
	if initialLimit < 100 {
		initialLimit = 100
	}

	query := `
        SELECT uf.id, uf.file, distance
        FROM files uf
        JOIN context_vectors cv ON cv.rowid = uf.id
        WHERE cv.embedding MATCH vec_f32(?)
        AND k = ?
        ORDER BY distance ASC
    `

	rows, err := db.Query(query, embeddingBytes, initialLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search query: %w", err)
	}
	defer rows.Close()

	var allResults []SearchResult
	for rows.Next() {
		var result SearchResult
		err = rows.Scan(&result.ID, &result.File, &result.Distance)
		if err != nil {
			return nil, fmt.Errorf("failed to scan embedding search row: %w", err)
		}
		allResults = append(allResults, result)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	// Apply distance-based filtering
	return filterByDistance(allResults, opts), nil
}

// filterByDistance applies distance-based filtering logic
func filterByDistance(results []SearchResult, opts SearchOptions) []SearchResult {
	if len(results) == 0 {
		return results
	}

	var filtered []SearchResult

	if opts.UseAdaptive {
		// Adaptive threshold: find natural clustering break
		threshold := calculateAdaptiveThreshold(results, opts.MaxDistance)

		for _, result := range results {
			if result.Distance <= threshold && len(filtered) < opts.MaxResults {
				filtered = append(filtered, result)
			}
		}
	} else {
		// Simple threshold filtering
		for _, result := range results {
			if result.Distance <= opts.MaxDistance && len(filtered) < opts.MaxResults {
				filtered = append(filtered, result)
			}
		}
	}

	// Ensure minimum results if available
	if len(filtered) < opts.MinResults && len(results) > 0 {
		minCount := opts.MinResults
		if minCount > len(results) {
			minCount = len(results)
		}
		if minCount > opts.MaxResults {
			minCount = opts.MaxResults
		}
		return results[:minCount]
	}

	return filtered
}

// calculateAdaptiveThreshold finds a natural break in distance distribution
func calculateAdaptiveThreshold(results []SearchResult, maxThreshold float64) float64 {
	if len(results) <= 1 {
		return maxThreshold
	}

	// Look for the largest gap in distances (elbow method)
	largestGap := 0.0
	gapIndex := 0

	for i := 1; i < len(results) && i < 20; i++ { // Only check first 20 results
		gap := results[i].Distance - results[i-1].Distance
		if gap > largestGap {
			largestGap = gap
			gapIndex = i
		}
	}

	// Use the gap-based threshold if it's reasonable, otherwise use max threshold
	if largestGap > 0.05 && gapIndex > 0 { // Minimum meaningful gap
		adaptiveThreshold := results[gapIndex-1].Distance + (largestGap / 2)
		if adaptiveThreshold < maxThreshold {
			return adaptiveThreshold
		}
	}

	return maxThreshold
}

// Convenience function that returns just file names (backward compatibility)
func Search(db *sql.DB, embeddings []float32, limit int) ([]string, error) {
	opts := DefaultSearchOptions()
	opts.MaxResults = limit
	opts.UseAdaptive = false // Keep original behavior

	results, err := SearchWithThreshold(db, embeddings, opts)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, result := range results {
		files = append(files, result.File)
	}
	return files, nil
}

// Advanced search with similarity score (1 - distance)
func SearchWithSimilarity(db *sql.DB, embeddings []float32, minSimilarity float64, maxResults int) ([]SearchResult, error) {
	opts := SearchOptions{
		MaxDistance: 1.0 - minSimilarity, // Convert similarity to distance
		MinResults:  1,
		MaxResults:  maxResults,
		UseAdaptive: true,
	}

	results, err := SearchWithThreshold(db, embeddings, opts)
	if err != nil {
		return nil, err
	}

	// Convert distances to similarity scores
	for i := range results {
		results[i].Distance = 1.0 - results[i].Distance
	}

	return results, nil
}

// Example usage functions for different search strategies

// SearchCodeSimilar finds code files similar to the query with adaptive threshold
func SearchCodeSimilar(db *sql.DB, queryEmbedding []float32) ([]SearchResult, error) {
	opts := SearchOptions{
		MaxDistance: 0.6,  // Stricter for code similarity
		MinResults:  3,    // Always return at least 3 if available
		MaxResults:  15,   // Don't overwhelm with too many results
		UseAdaptive: true, // Use intelligent threshold
	}

	return SearchWithThreshold(db, queryEmbedding, opts)
}

// SearchCodeExact finds very similar code files with tight threshold
func SearchCodeExact(db *sql.DB, queryEmbedding []float32) ([]SearchResult, error) {
	opts := SearchOptions{
		MaxDistance: 0.3, // Very strict threshold
		MinResults:  1,
		MaxResults:  10,
		UseAdaptive: false,
	}

	return SearchWithThreshold(db, queryEmbedding, opts)
}
