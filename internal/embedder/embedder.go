package embedder

import (
	"context"
	"log"
	"time"

	"mspro-labs/brew-buddy/internal/ai"
	"mspro-labs/brew-buddy/internal/db"
	"database/sql"
)

// Run finds all coffees missing embeddings and processes them.
func Run(ctx context.Context, database *sql.DB, aiClient *ai.Client) error {
	// 1. Find work to do
	targets, err := db.GetUnembeddedCoffees(database)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		log.Println("✨ All active coffees are already embedded.")
		return nil
	}
	log.Printf("Found %d new coffees to embed...", len(targets))

	// 2. Process loop
	count := 0
	for url, textToEmbed := range targets {
		// Just show the start of the name for cleaner logs
		shortName := textToEmbed
		if len(shortName) > 30 {
			shortName = shortName[:30] + "..."
		}
		log.Printf("Embedding: %s", shortName)

		// Generate vector
		blob, _, err := aiClient.EmbedString(ctx, textToEmbed)
		if err != nil {
			log.Printf("⚠️ Error embedding item: %v", err)
			time.Sleep(1 * time.Second) // Backoff on error
			continue
		}

		// Save vector
		if err := db.UpdateEmbedding(database, url, blob); err != nil {
			log.Printf("⚠️ Error saving to DB: %v", err)
			continue
		}

		count++
		// Rate limit for free tier safety (approx 60 RPM max)
		time.Sleep(1 * time.Second)
	}

	log.Printf("🎉 Successfully embedded %d items.", count)
	return nil
}
