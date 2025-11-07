package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3" // Import for side-effects only

	"mspro-labs/brew-buddy/internal/models"
)

// Connect opens a connection to the SQLite database and ensures the schema exists.
// It automatically applies recommended settings for concurrency (WAL mode).
func Connect(dbPath string) (*sql.DB, error) {
	// Use robust connection settings to prevent "database locked" errors
	dsn := fmt.Sprintf("%s?_busy_timeout=5000&_journal_mode=WAL", dbPath)
	
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err = createSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ensure schema: %w", err)
	}

	return db, nil
}

// MarkAllAsInactive sets is_active=0 for all coffees.
// This is called at the start of a scrape run.
func MarkAllAsInactive(db *sql.DB) error {
	_, err := db.Exec(`UPDATE coffee SET is_active = 0 WHERE is_active = 1;`)
	if err != nil {
		return fmt.Errorf("failed to mark coffees as inactive: %w", err)
	}
	return nil
}

// createSchema is private as it's only called by Connect.
func createSchema(db *sql.DB) error {
	// Main Coffee Table
	coffeeTable := `
	CREATE TABLE IF NOT EXISTS coffee (
	  id INTEGER PRIMARY KEY AUTOINCREMENT,
	  url TEXT UNIQUE NOT NULL,
	  name TEXT,
	  price REAL,
	  score REAL,
	  origin TEXT,
	  region TEXT,
	  tasting_notes TEXT,
	  processing TEXT,
	  description TEXT,
	  stock_status TEXT,
	  first_scraped_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	  last_scraped_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	  last_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	  is_active INTEGER DEFAULT 1,
	  description_embedding BLOB -- Added for AI search
	);
	CREATE INDEX IF NOT EXISTS idx_url ON coffee(url);
	CREATE INDEX IF NOT EXISTS idx_is_active ON coffee(is_active);
	`
	if _, err := db.Exec(coffeeTable); err != nil {
		return err
	}

	// Search History Table (for local caching of AI queries)
	historyTable := `
	CREATE TABLE IF NOT EXISTS search_history (
		query_text TEXT PRIMARY KEY, 
		embedding BLOB, 
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(historyTable); err != nil {
		return err
	}

	// (Optional Future Use) My Notes Table
	notesTable := `
	CREATE TABLE IF NOT EXISTS my_notes (
	  id INTEGER PRIMARY KEY AUTOINCREMENT,
	  coffee_url TEXT NOT NULL,
	  rating INTEGER,
	  notes TEXT,
	  purchased_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	  FOREIGN KEY (coffee_url) REFERENCES coffee (url)
	);
	`
	if _, err := db.Exec(notesTable); err != nil {
		return err
	}

	return nil
}

// SaveData performs a batch UPSERT of coffee items into the database.
// It marks saved items as active and updates their 'last_seen_at' timestamp.
func SaveData(db *sql.DB, items []models.CoffeeItem) (int64, error) {
	upsertSQL := `
	INSERT INTO coffee (
	  url, name, price, score, origin, region, tasting_notes, processing, description, stock_status,
	  last_scraped_at, last_seen_at, is_active
	) VALUES (
	  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
	  CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 1
	) ON CONFLICT(url) DO UPDATE SET
	  name = excluded.name,
	  price = excluded.price,
	  score = excluded.score,
	  origin = excluded.origin,
	  region = excluded.region,
	  tasting_notes = excluded.tasting_notes,
	  processing = excluded.processing,
	  description = excluded.description,
	  stock_status = excluded.stock_status,
	  last_scraped_at = CURRENT_TIMESTAMP,
	  last_seen_at = CURRENT_TIMESTAMP,
	  is_active = 1;
	`

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}

	stmt, err := tx.PrepareContext(ctx, upsertSQL)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()

	var totalAffected int64 = 0
	for _, item := range items {
		res, err := stmt.ExecContext(ctx,
			item.URL,
			item.Name,
			item.Price,
			sql.NullFloat64{Float64: item.Score, Valid: item.Score > 0},
			sql.NullString{String: item.Origin, Valid: item.Origin != ""},
			sql.NullString{String: item.Region, Valid: item.Region != ""},
			sql.NullString{String: item.TastingNotes, Valid: item.TastingNotes != ""},
			sql.NullString{String: item.Processing, Valid: item.Processing != ""},
			sql.NullString{String: item.Description, Valid: item.Description != ""},
			sql.NullString{String: item.StockStatus, Valid: item.StockStatus != ""},
		)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("failed to upsert %s: %w", item.URL, err)
		}
		rows, _ := res.RowsAffected()
		totalAffected += rows
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return totalAffected, nil
}

// GetActiveCoffees returns all currently available coffees for the web UI.
func GetActiveCoffees(db *sql.DB) ([]models.CoffeeItem, error) {
	// We only need basic info for the main list
	rows, err := db.Query(`
		SELECT name, url, origin, price, stock_status
		FROM coffee
		WHERE is_active = 1
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.CoffeeItem
	for rows.Next() {
		var i models.CoffeeItem
		if err := rows.Scan(&i.Name, &i.URL, &i.Origin, &i.Price, &i.StockStatus); err == nil {
			items = append(items, i)
		}
	}
	return items, nil
}

// --- Embedding & Search Helpers ---

// GetUnembeddedCoffees returns a map of URL -> Description for active items missing embeddings.
func GetUnembeddedCoffees(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT url, name, description FROM coffee WHERE is_active = 1 AND description_embedding IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make(map[string]string)
	for rows.Next() {
		var url, name, desc string
		if err := rows.Scan(&url, &name, &desc); err == nil {
			// Combine name and description for a richer embedding
			results[url] = fmt.Sprintf("Coffee Name: %s\nDescription: %s", name, desc)
		}
	}
	return results, nil
}

// UpdateEmbedding saves the generated vector blob for a specific coffee URL.
func UpdateEmbedding(db *sql.DB, url string, embedding []byte) error {
	_, err := db.Exec("UPDATE coffee SET description_embedding = ? WHERE url = ?", embedding, url)
	return err
}

// GetCoffeeVectors returns all active coffees that have embeddings.
// Returns a slice of struct for easy iteration during search.
type CoffeeVector struct {
	URL			string
	Name        string
	Origin      string
	Description string
	Vector      []byte
}

func GetCoffeeVectors(db *sql.DB) ([]CoffeeVector, error) {
	rows, err := db.Query(`SELECT url, name, origin, description, description_embedding FROM coffee WHERE is_active = 1 AND description_embedding IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CoffeeVector
	for rows.Next() {
		var cv CoffeeVector
		if err := rows.Scan(&cv.URL, &cv.Name, &cv.Origin, &cv.Description, &cv.Vector); err == nil {
			results = append(results, cv)
		}
	}
	return results, nil
}

// GetCachedQuery tries to find a previously searched query vector.
func GetCachedQuery(db *sql.DB, text string) ([]byte, error) {
	var blob []byte
	err := db.QueryRow("SELECT embedding FROM search_history WHERE query_text = ?", text).Scan(&blob)
	return blob, err
}

// SaveCachedQuery saves a new query and its vector to the history table.
func SaveCachedQuery(db *sql.DB, text string, blob []byte) error {
	_, err := db.Exec("INSERT OR IGNORE INTO search_history (query_text, embedding) VALUES (?, ?)", text, blob)
	return err
}

// --- History Management for search ---

type HistoryEntry struct {
	QueryText string
	CreatedAt time.Time
}

// ListSearchHistory returns all cached queries, newest first.
func ListSearchHistory(db *sql.DB) ([]HistoryEntry, error) {
	rows, err := db.Query("SELECT query_text, created_at FROM search_history ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.QueryText, &e.CreatedAt); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// ClearSearchHistory removes a specific query from the cache.
func ClearSearchHistory(db *sql.DB, queryText string) (int64, error) {
	res, err := db.Exec("DELETE FROM search_history WHERE query_text = ?", queryText)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ClearAllSearchHistory wipes the entire cache.
func ClearAllSearchHistory(db *sql.DB) (int64, error) {
	res, err := db.Exec("DELETE FROM search_history")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
