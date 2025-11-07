package cmd

import (
	"context"
	"log"

	"github.com/spf13/cobra"
	"mspro-labs/brew-buddy/internal/ai"
	"mspro-labs/brew-buddy/internal/config"
	"mspro-labs/brew-buddy/internal/db"
	"mspro-labs/brew-buddy/internal/embedder"
)

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Generate AI embeddings for new coffees",
	Long:  `Finds coffees in the database that are missing semantic vectors and generates them using the Gemini API.`,
	Run: func(cmd *cobra.Command, args []string) {
		runEmbed()
	},
}

func init() {
	rootCmd.AddCommand(embedCmd)
}

func runEmbed() {
	ctx := context.Background()

	// 1. Config & DB
	appCfg, err := config.GetAppConfig()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}
	database, err := db.Connect(appCfg.DBPath)
	if err != nil {
		log.Fatalf("Database error: %v", err)
	}
	defer database.Close()

	// 2. Initialize AI
	aiClient, err := ai.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize AI client: %v", err)
	}
	defer aiClient.Close()

	// 3. Run Shared Embedder Logic
	if err := embedder.Run(ctx, database, aiClient); err != nil {
		log.Fatalf("Embedding process failed: %v", err)
	}
}
