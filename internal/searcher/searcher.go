package searcher

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"

	"mspro-labs/brew-buddy/internal/ai"
	"mspro-labs/brew-buddy/internal/db"
)

// Result holds a single search match.
type Result struct {
	Item  db.CoffeeVector
	Score float32
}

// Perform executes a semantic search.
func Perform(ctx context.Context, database *sql.DB, aiClient *ai.Client, queryText string) ([]Result, error) {
	// 1. Get Query Vector (Try cache first, then AI)
	queryVector, err := getQueryVector(ctx, database, aiClient, queryText)
	if err != nil {
		return nil, err
	}

	// 2. Load all coffee vectors
	coffees, err := db.GetCoffeeVectors(database)
	if err != nil {
		return nil, fmt.Errorf("failed to load coffees: %w", err)
	}

	// 3. Compare and score
	var results []Result
	for _, coffee := range coffees {
		coffeeFloats, err := ai.BytesToFloats(coffee.Vector)
		if err != nil {
			continue
		}
		score := ai.CosineSimilarity(queryVector, coffeeFloats)
		results = append(results, Result{Item: coffee, Score: score})
	}

	// 4. Sort by descending score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit to Top 5
	if len(results) > 5 {
		results = results[:5]
	}

	return results, nil
}

// getQueryVector handles the "cache-aside" logic for query embeddings.
func getQueryVector(ctx context.Context, database *sql.DB, aiClient *ai.Client, text string) ([]float32, error) {
	// A. Try Cache
	blob, err := db.GetCachedQuery(database, text)
	if err == nil {
		// Cache hit
		return ai.BytesToFloats(blob)
	}

	// B. Cache Miss - Use AI
	log.Printf("🤖 Cache miss for '%s'. Calling Gemini...", text)
	blob, floats, err := aiClient.EmbedString(ctx, text)
	if err != nil {
		return nil, err
	}

	// C. Save to Cache (don't fail the request if cache save fails)
	if err := db.SaveCachedQuery(database, text, blob); err != nil {
		log.Printf("Warning: failed to save query to cache: %v", err)
	}

	return floats, nil
}
