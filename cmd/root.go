package cmd

import (
	"fmt"
	"os"

	"github.com/andrejsstepanovs/codesearch/search"
	"github.com/andrejsstepanovs/codesearch/sync"
	"github.com/spf13/cobra"
)

type App struct {
	// This struct can be expanded later with shared dependencies
}

func newBuildCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build <project-alias> <project-path> [client-name] [model-name] [extensions]",
		Short: "Build embeddings for a project. First argument is project alias, second is project path, optional third is client name (litellm, ollama), model name, optional fourth is comma separated list of file extensions (default: go,js,ts,py,java,cpp,c,h,hpp,yaml,yml)",
		Run:   app.handleBuild,
	}
	return cmd
}

func newSyncCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync <project-alias>",
		Short: "Sync embeddings for a project using stored configuration",
		Args:  cobra.ExactArgs(1),
		Run:   app.handleSync,
	}
	return cmd
}

func newSearchCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "find <project-alias> <search-query>",
		Short: "Search for code files in a project. First argument is project alias, rest are search query",
		Run:   app.handleSearch,
	}
	return cmd
}

func newRootCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codesearch",
		Short: "CLI for managing code embeddings and search",
	}
	cmd.AddCommand(
		newBuildCmd(app),
		newSyncCmd(app),
		newSearchCmd(app),
	)
	return cmd
}

func (a *App) handleBuild(cmd *cobra.Command, args []string) {
	config, err := sync.ParseConfig(args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if err := sync.Run(cmd.Context(), config); err != nil {
		fmt.Printf("Error during build operation: %v\n", err)
		os.Exit(1)
	}
}

func (a *App) handleSync(cmd *cobra.Command, args []string) {
	projectAlias := args[0]

	if err := sync.RunSync(cmd.Context(), projectAlias); err != nil {
		fmt.Printf("Error during sync operation: %v\n", err)
		os.Exit(1)
	}
}

func (a *App) handleSearch(cmd *cobra.Command, args []string) {
	config, err := search.ParseConfig(args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Searching for: %s\n", config.Query)
	results, err := search.Run(cmd.Context(), config)
	if err != nil {
		fmt.Printf("Error during search operation: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d files\n", len(results))
	for _, filePath := range results {
		fmt.Printf("%s \t (%f %d)\n", filePath.File, filePath.Distance, filePath.ID)
	}
}

// Execute initializes and runs the root command. It is the single entry point
// for the command-line interface.
func Execute() {
	app := &App{}
	rootCmd := newRootCmd(app)
	if err := rootCmd.Execute(); err != nil {
		// Cobra prints the error, so we just need to exit.
		os.Exit(1)
	}
}
