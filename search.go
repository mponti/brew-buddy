package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/api/option"
)

type SearchResult struct {
	Name        string
	Origin      string
	Description string
	Score       float32
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./local-data/coffee.db"
	}

	// Initialize DB connection centrally
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Command routing
	command := strings.ToLower(os.Args[1])
	switch command {
	case "history":
		if err := listHistory(db); err != nil {
			log.Fatalf("Failed to list history: %v", err)
		}
	case "clear":
		if len(os.Args) < 3 {
			log.Fatal("Usage: go run search.go clear \"query text\" (or 'all')")
		}
		target := strings.ToLower(strings.TrimSpace(strings.Join(os.Args[2:], " ")))
		if err := clearHistory(db, target); err != nil {
			log.Fatalf("Failed to clear history: %v", err)
		}
	default:
		// Default behavior: treat arguments as a search query
		queryText := strings.ToLower(strings.TrimSpace(strings.Join(os.Args[1:], " ")))
		if err := runSearch(db, queryText); err != nil {
			log.Fatalf("Search failed: %v", err)
		}
	}
}

func printUsage() {
	fmt.Println("Brew Buddy Search CLI")
	fmt.Println("Usage:")
	fmt.Println("  go run search.go \"your vibe query\"   # Perform a search")
	fmt.Println("  go run search.go history             # List cached searches")
	fmt.Println("  go run search.go clear \"your query\"  # Remove specific cache entry")
	fmt.Println("  go run search.go clear all           # Clear entire cache")
}

// --- History Management ---

func listHistory(db *sql.DB) error {
	rows, err := db.Query("SELECT query_text, created_at FROM search_history ORDER BY created_at DESC")
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Println("📜 Search History (Cached Queries)")
	fmt.Println("------------------------------------")
	count := 0
	for rows.Next() {
		var text string
		var createdAt time.Time
		if err := rows.Scan(&text, &createdAt); err != nil {
			continue
		}
		fmt.Printf("[%s] %s\n", createdAt.Format("2006-01-02 15:04"), text)
		count++
	}
	if count == 0 {
		fmt.Println("No history found.")
	}
	fmt.Println("")
	return nil
}

func clearHistory(db *sql.DB, target string) error {
	var res sql.Result
	var err error

	if target == "all" {
		fmt.Println("🗑️  Clearing ALL search history cache...")
		res, err = db.Exec("DELETE FROM search_history")
	} else {
		fmt.Printf("🗑️  Clearing cache for: '%s'...\n", target)
		res, err = db.Exec("DELETE FROM search_history WHERE query_text = ?", target)
	}

	if err != nil {
		return err
	}

	affected, _ := res.RowsAffected()
	fmt.Printf("Done. Removed %d entry(s).\n", affected)
	return nil
}

// --- Search Logic ---

func runSearch(db *sql.DB, queryText string) error {
	ctx := context.Background()
	apiKey := os.Getenv("GEMINI_API_KEY")

	// 1. Get Query Vector (Cached or from AI)
	queryVector, err := getQueryVector(ctx, db, apiKey, queryText)
	if err != nil {
		return fmt.Errorf("failed to get query vector: %w", err)
	}

	// 2. Load all coffee embeddings and compare
	rows, err := db.Query(`SELECT name, origin, description, embedding FROM coffee WHERE is_active = 1 AND embedding IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("failed to query coffees: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var blob []byte
		if err := rows.Scan(&r.Name, &r.Origin, &r.Description, &blob); err != nil {
			continue
		}

		coffeeVector, err := bytesToFloats(blob)
		if err != nil {
			continue
		}

		r.Score = cosineSimilarity(queryVector, coffeeVector)
		results = append(results, r)
	}

	// Sort by descending score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Print results
	fmt.Printf("\n🔍 Top matches for: \"%s\"\n\n", queryText)
	for i, r := range results {
		if i >= 5 {
			break
		}
		fmt.Printf("#%d [%.1f%% match] %s (%s)\n", i+1, r.Score*100, r.Name, r.Origin)
		fmt.Printf("   %s\n\n", truncate(r.Description, 150))
	}

	return nil
}

func getQueryVector(ctx context.Context, db *sql.DB, apiKey, queryText string) ([]float32, error) {
	// A. Check Cache
	var blob []byte
	err := db.QueryRow("SELECT embedding FROM search_history WHERE query_text = ?", queryText).Scan(&blob)
	if err == nil {
		fmt.Println("⚡ Cache hit! Using saved vector for this query.")
		return bytesToFloats(blob)
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("cache lookup failed: %w", err)
	}

	// B. Cache Miss - Call AI API
	fmt.Println("🤖 Cache miss. Calling Gemini API...")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is required for new queries")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("AI client error: %w", err)
	}
	defer client.Close()

	apiCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.EmbeddingModel("text-embedding-004").EmbedContent(apiCtx, genai.Text(queryText))
	if err != nil {
		return nil, fmt.Errorf("AI API call failed: %w", err)
	}
	if resp.Embedding == nil {
		return nil, fmt.Errorf("AI API returned empty embedding")
	}
	vector := resp.Embedding.Values

	// C. Save to Cache
	newBlob, _ := floatsToBytes(vector)
	_, err = db.Exec("INSERT OR IGNORE INTO search_history (query_text, embedding) VALUES (?, ?)", queryText, newBlob)
	if err != nil {
		log.Printf("Warning: failed to save query to cache: %v", err)
	}

	return vector, nil
}

// --- Math & Byte Helpers ---

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) { return 0 }
	var dotProduct, magA, magB float32
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 { return 0 }
	return dotProduct / (float32(math.Sqrt(float64(magA))) * float32(math.Sqrt(float64(magB))))
}

func floatsToBytes(floats []float32) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, floats)
	return buf.Bytes(), err
}

func bytesToFloats(b []byte) ([]float32, error) {
	if len(b)%4 != 0 { return nil, fmt.Errorf("invalid data") }
	floats := make([]float32, len(b)/4)
	err := binary.Read(bytes.NewReader(b), binary.LittleEndian, &floats)
	return floats, err
}

func truncate(s string, max int) string {
	if len(s) > max { return s[:max] + "..." }
	return s
}
