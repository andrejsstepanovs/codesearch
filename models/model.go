package models

import "time"

// Project stores metadata about a registered project.
type Project struct {
	Alias      string
	Path       string
	Client     string
	Model      string
	Extensions []string
}

// File represents a file record in the database.
type File struct {
	ID        int64
	File      string
	CreatedAt time.Time
}
