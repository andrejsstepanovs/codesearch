package file

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

func RecursiveFiles(path string, extensions []string) ([]string, error) {
	var files []string

	// Normalize extensions to include the dot and be lowercase
	normalizedExts := make([]string, len(extensions))
	for i, ext := range extensions {
		ext = strings.TrimSpace(ext)
		if ext != "" && !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		normalizedExts[i] = strings.ToLower(ext)
	}

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			// Log the error but continue walking
			log.Printf("Error accessing path %s: %v", filePath, err)
			return nil
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip hidden files and directories (optional - remove if you want hidden files)
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// If extensions are specified, filter by them
		if len(normalizedExts) > 0 {
			ext := strings.ToLower(filepath.Ext(filePath))
			for _, e := range normalizedExts {
				if ext == e {
					files = append(files, filePath)
					break
				}
			}
		} else {
			files = append(files, filePath)
		}

		return nil
	})

	return files, err
}
