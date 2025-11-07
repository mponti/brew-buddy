package cmd

import (
	"log"
	"context"

	"github.com/spf13/cobra"
	
	"mspro-labs/brew-buddy/internal/ai"
	"mspro-labs/brew-buddy/internal/config"
	"mspro-labs/brew-buddy/internal/db"
	"mspro-labs/brew-buddy/internal/embedder"
	"mspro-labs/brew-buddy/internal/scraper"
)

// scrapeCmd represents the scrape command
var scrapeCmd = &cobra.Command{
	Use:   "scrape",
	Short: "Run the scraper once auto-runs embed",
	Long:  `Connects to the target site, scrapes current inventory, updates the local database, and runs embed on new finds.`,
	Run: func(cmd *cobra.Command, args []string) {
		runScrape()
	},
}

func init() {
	rootCmd.AddCommand(scrapeCmd)
}

func runScrape() {
	// 1. Load Config
	appCfg, err := config.GetAppConfig()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}
	siteCfg, err := config.LoadSiteConfig(appCfg.ConfigPath)
	if err != nil {
		log.Fatalf("Failed to load site config: %v", err)
	}

	// 2. Connect to DB
	database, err := db.Connect(appCfg.DBPath)
	if err != nil {
		log.Fatalf("Database error: %v", err)
	}
	defer database.Close()

	// 3. Prep DB (Mark old items inactive)
	if err := db.MarkAllAsInactive(database); err != nil {
		log.Fatalf("Failed to mark inactive: %v", err)
	}

	// 4. Run Scraper
	items, err := scraper.Run(siteCfg)
	if err != nil {
		log.Fatalf("Scraping failed: %v", err)
	}
	log.Printf("Scraper found %d valid items.", len(items))

	if len(items) == 0 {
		log.Println("No items to save. Exiting.")
		return
	}

	// 5. Save to DB
	count, err := db.SaveData(database, items)
	if err != nil {
		log.Fatalf("Failed to save data: %v", err)
	}
	log.Printf("SUCCESS: Upserted %d records.", count)

	// 6. Auto-run Embedder
	log.Println("🤖 Starting automatic embedding...")
	ctx := context.Background()
	aiClient, err := ai.NewClient(ctx)
	if err != nil {
		log.Printf("⚠️ Warning: Could not initialize AI for auto-embedding (check GEMINI_API_KEY): %v", err)
		return // Don't fail the whole scrape if AI fails
	}
	defer aiClient.Close()

	if err := embedder.Run(ctx, database, aiClient); err != nil {
		log.Printf("⚠️ Warning: Auto-embedding failed: %v", err)
	}
}
