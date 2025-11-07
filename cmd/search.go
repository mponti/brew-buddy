package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"mspro-labs/brew-buddy/internal/ai"
	"mspro-labs/brew-buddy/internal/config"
	"mspro-labs/brew-buddy/internal/db"
)

type searchResult struct {
	item  db.CoffeeVector
	score float32
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Semantic search for coffees by 'vibe'",
	Long: `Uses AI to find coffees that match the semantic meaning of your query.
Examples:
  brew-buddy search "funky and fruity with berry notes"
  brew-buddy search "classic comforting chocolate"

History commands:
  brew-buddy search history
  brew-buddy search clear "query string"
  brew-buddy search clear all`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		handleSearch(args)
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}

func handleSearch(args []string) {
	// 1. Setup
	appCfg, _ := config.GetAppConfig()
	database, err := db.Connect(appCfg.DBPath)
	if err != nil {
		log.Fatalf("Database error: %v", err)
	}
	defer database.Close()

	command := strings.ToLower(args[0])

	// 2. Commands
	if command == "history" {
		entries, err := db.ListSearchHistory(database)
		if err != nil {
			log.Fatalf("Failed to list history: %v", err)
		}
		fmt.Println("📜 Search History (Cached Queries)")
		fmt.Println("------------------------------------")
		if len(entries) == 0 {
			fmt.Println("No history found.")
			return
		}
		for _, e := range entries {
			fmt.Printf("[%s] %s\n", e.CreatedAt.Format("2006-01-02 15:04"), e.QueryText)
		}
		return
	}

	if command == "clear" {
		if len(args) < 2 {
			log.Fatal("Usage: brew-buddy search clear \"query text\" (or 'all')")
		}
		target := strings.ToLower(strings.TrimSpace(strings.Join(args[1:], " ")))
		var affected int64
		var err error

		if target == "all" {
			affected, err = db.ClearAllSearchHistory(database)
		} else {
			affected, err = db.ClearSearchHistory(database, target)
		}

		if err != nil {
			log.Fatalf("Failed to clear history: %v", err)
		}
		fmt.Printf("🗑️ Done. Removed %d entry(s) from cache.\n", affected)
		return
	}

	// 3. Perform regular search
	query := strings.Join(args, " ")
	if err := performSearch(database, query); err != nil {
		log.Fatalf("Search failed: %v", err)
	}
}

func performSearch(database *sql.DB, queryText string) error {
	ctx := context.Background()

	// A. Try Cache
	queryVectorFloats, _ := tryCache(database, queryText)

	// B. If miss, use AI
	if queryVectorFloats == nil {
		fmt.Println("🤖 Cache miss. Asking Gemini...")
		aiClient, err := ai.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to init AI: %w", err)
		}
		defer aiClient.Close()

		// Embed and get both blob (for DB) and floats (for math now)
		blob, floats, err := aiClient.EmbedString(ctx, queryText)
		if err != nil {
			return fmt.Errorf("embedding failed: %w", err)
		}

		// Save to cache for next time
		_ = db.SaveCachedQuery(database, queryText, blob)
		queryVectorFloats = floats
	} else {
		fmt.Println("⚡ Cache hit! Using saved vector.")
	}

	// C. Compare against all coffees
	coffees, err := db.GetCoffeeVectors(database)
	if err != nil {
		return fmt.Errorf("failed to load coffees: %w", err)
	}

	var results []searchResult
	for _, coffee := range coffees {
		coffeeFloats, err := ai.BytesToFloats(coffee.Vector)
		if err != nil {
			continue
		}
		score := ai.CosineSimilarity(queryVectorFloats, coffeeFloats)
		results = append(results, searchResult{item: coffee, score: score})
	}

	// D. Sort & Display
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	fmt.Printf("\n🔍 Top matches for: \"%s\"\n\n", queryText)
	for i, r := range results {
		if i >= 5 {
			break
		}
		fmt.Printf("#%d [%.1f%% match] %s (%s)\n", i+1, r.score*100, r.item.Name, r.item.Origin)
		fmt.Printf("   %s\n\n", truncate(r.item.Description, 150))
	}

	return nil
}

// Helper to try loading from cache and converting immediately
func tryCache(database *sql.DB, text string) ([]float32, error) {
	blob, err := db.GetCachedQuery(database, text)
	if err != nil {
		return nil, err
	}
	return ai.BytesToFloats(blob)
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
