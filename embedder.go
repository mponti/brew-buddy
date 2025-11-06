package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/generative-ai-go/genai"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/api/option"
)

// Reuse the same logger setup if you like
var embedLogger = log.New(os.Stdout, "AI-EMBEDDER: ", log.LstdFlags|log.Lshortfile)

func main() {
	embedLogger.Println("☕ Starting AI embedding process...")
	if err := runEmbedder(); err != nil {
		embedLogger.Fatalf("FAILED: %v", err)
	}
	embedLogger.Println("✨ Embedding process complete!")
}

func runEmbedder() error {
	ctx := context.Background()
	dbPath := os.Getenv("DB_PATH")
	apiKey := os.Getenv("GEMINI_API_KEY")

	if dbPath == "" {
		dbPath = "./local-data/coffee.db" // Default for local testing
	}
	if apiKey == "" {
		return fmt.Errorf("GEMINI_API_KEY environment variable is required")
	}

	// 1. Connect DB
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("failed to open DB: %w", err)
	}
	defer db.Close()

	// 2. Connect AI
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("failed to create AI client: %w", err)
	}
	defer client.Close()
	model := client.EmbeddingModel("text-embedding-004")

	// 3. Find unembedded coffees (active ones only to save API credits)
	rows, err := db.Query(`SELECT url, name, description FROM coffee WHERE is_active = 1 AND embedding IS NULL`)
	if err != nil {
		return fmt.Errorf("failed to query coffees: %w", err)
	}
	defer rows.Close()

	// 4. Process Loop
	count := 0
	for rows.Next() {
		var url, name, desc string
		if err := rows.Scan(&url, &name, &desc); err != nil {
			continue
		}

		embedLogger.Printf("Generating embedding for: %s", name)
		
		// Combine name and description for a richer vector
		contentToEmbed := fmt.Sprintf("Coffee Name: %s\nDescription: %s", name, desc)

		res, err := model.EmbedContent(ctx, genai.Text(contentToEmbed))
		if err != nil {
			embedLogger.Printf("ERROR embedding '%s': %v", name, err)
			time.Sleep(1 * time.Second) // Backoff on error
			continue
		}

		// Convert float vector to raw bytes for SQLite BLOB
		blob, err := floatsToBytes(res.Embedding.Values)
		if err != nil {
			return fmt.Errorf("failed to convert vector to bytes: %w", err)
		}

		// Save back to DB
		_, err = db.Exec(`UPDATE coffee SET embedding = ? WHERE url = ?`, blob, url)
		if err != nil {
			embedLogger.Printf("ERROR saving to DB for '%s': %v", name, err)
			continue
		}
		count++
		time.Sleep(1 * time.Second) // Rate limit politeness
	}

	embedLogger.Printf("Successfully embedded %d new coffees.", count)
	return nil
}

// --- Vector Helpers ---

// floatsToBytes converts []float32 to a byte slice for storage
func floatsToBytes(floats []float32) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, floats)
	return buf.Bytes(), err
}

// bytesToFloats converts the stored byte slice back to []float32
func bytesToFloats(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("invalid byte length for float32 slice")
	}
	floats := make([]float32, len(b)/4)
	reader := bytes.NewReader(b)
	err := binary.Read(reader, binary.LittleEndian, &floats)
	return floats, err
}
