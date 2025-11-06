package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v3"
)

// --- CONFIGURATION STRUCTS ---

// AppConfig holds infrastructure config from standard env vars
type AppConfig struct {
	DBPath     string
	ConfigPath string // Path to the JSON config file
}

type SiteConfig struct {
	CategoryURL        string    `yaml:"category_url"`
	Selectors          Selectors `yaml:"selectors"`
	DisallowedKeywords []string  `yaml:"disallowed_keywords"`
}

type Selectors struct {
	CookieButton         string `yaml:"cookie_button"`
	NewsletterPopup      string `yaml:"newsletter_popup"`
	ProductListWait      string `yaml:"product_list_wait"`
	ProductRow           string `yaml:"product_row"`
	Link                 string `yaml:"link"`
	Price                string `yaml:"price"`
	Origin               string `yaml:"origin"`
	StockButton          string `yaml:"stock_button"`
	StockComingSoon      string `yaml:"stock_coming_soon"`
	Description          string `yaml:"description"`
	DescriptionIsNextRow bool   `yaml:"description_is_next_row"`
}

// --- DATA STRUCTS ---

type CoffeeItem struct {
	URL          string
	Name         string
	Price        float64
	Score        float64
	Origin       string
	Region       string
	TastingNotes string
	Processing   string
	Description  string
	StockStatus  string
}

var logger = log.New(os.Stdout, "scraper: ", log.LstdFlags|log.Lshortfile)

// --- MAIN APPLICATION ---

func main() {
	logger.Println("Starting scraper...")
	if err := run(); err != nil {
		logger.Fatalf("Scraper failed: %v", err)
	}
	logger.Println("Scraper finished successfully.")
}

func run() error {
	// 1. Get App Configuration (Env Vars)
	appCfg, err := getAppConfig()
	if err != nil {
		return fmt.Errorf("app configuration error: %w", err)
	}

	// 2. Load Site Configuration (JSON File)
	siteCfg, err := loadSiteConfig(appCfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load site config: %w", err)
	}

	// 3. Initialize Database
	db, err := initDB(appCfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	// 4. Mark old items as inactive (start of sweep)
	if _, err := markAllAsInactive(db); err != nil {
		return fmt.Errorf("failed to mark inactive: %w", err)
	}

	// 5. Launch Browser
	logger.Println("Launching headless browser...")
	browser, err := launchBrowser()
	if err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}
	defer browser.MustClose()

	// 6. Fetch HTML (passing site config for URL and selectors)
	logger.Printf("Fetching URL: %s", siteCfg.CategoryURL)
	html, err := fetchHTML(browser, siteCfg)
	if err != nil {
		return fmt.Errorf("failed to fetch HTML: %w", err)
	}
	logger.Println("HTML fetched successfully.")

	// 7. Parse HTML (passing site config for selectors and keywords)
	logger.Println("Parsing HTML...")
	items, err := parseHTML(html, siteCfg)
	if err != nil {
		return fmt.Errorf("failed to parse HTML: %w", err)
	}
	logger.Printf("Parsed and filtered %d items.", len(items))

	if len(items) == 0 {
		logger.Println("No items found, skipping database update.")
		return nil
	}

	// 8. Save Data (UPSERT and mark active)
	logger.Println("Saving data to database...")
	count, err := saveData(db, items)
	if err != nil {
		return fmt.Errorf("failed to save data: %w", err)
	}
	logger.Printf("Successfully upserted %d records.", count)

	return nil
}

// --- CONFIGURATION HELPERS ---

func getAppConfig() (AppConfig, error) {
	dbPath := os.Getenv("DB_PATH")
	configPath := os.Getenv("CONFIG_PATH") // New env var for the JSON file location

	if dbPath == "" {
		return AppConfig{}, fmt.Errorf("DB_PATH environment variable not set")
	}
	if configPath == "" {
		// Default to looking in the current directory if not set
		configPath = "config.yaml"
	}

	return AppConfig{
		DBPath:     dbPath,
		ConfigPath: configPath,
	}, nil
}

func loadSiteConfig(path string) (*SiteConfig, error) {
	logger.Printf("Loading site config from: %s", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg SiteConfig
	// --- CHANGE HERE: json.Unmarshal -> yaml.Unmarshal ---
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// --- DATABASE FUNCTIONS ---

func initDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	if err = createSchema(db); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}
	return db, nil
}

func createSchema(db *sql.DB) error {
	schemaSQL := `
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
	  is_active INTEGER DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS idx_url ON coffee(url);
	CREATE INDEX IF NOT EXISTS idx_is_active ON coffee(is_active);
	`
	_, err := db.Exec(schemaSQL)
	return err
}

func markAllAsInactive(db *sql.DB) (int64, error) {
	logger.Println("Marking all active coffees as inactive...")
	res, err := db.Exec(`UPDATE coffee SET is_active = 0 WHERE is_active = 1;`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func saveData(db *sql.DB, items []CoffeeItem) (int64, error) {
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
			item.URL, item.Name, item.Price, sql.NullFloat64{Float64: item.Score, Valid: item.Score > 0},
			sql.NullString{String: item.Origin, Valid: item.Origin != ""}, sql.NullString{String: item.Region, Valid: item.Region != ""},
			sql.NullString{String: item.TastingNotes, Valid: item.TastingNotes != ""}, sql.NullString{String: item.Processing, Valid: item.Processing != ""},
			sql.NullString{String: item.Description, Valid: item.Description != ""}, sql.NullString{String: item.StockStatus, Valid: item.StockStatus != ""},
		)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("failed to exec upsert for %s: %w", item.URL, err)
		}
		affected, _ := res.RowsAffected()
		totalAffected += affected
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return totalAffected, nil
}

// --- BROWSER FUNCTIONS ---

func launchBrowser() (*rod.Browser, error) {
	l := launcher.New().Headless(true).NoSandbox(true)
	u, err := l.Launch()
	if err != nil {
		return nil, err
	}
	return rod.New().ControlURL(u).MustConnect(), nil
}

func fetchHTML(browser *rod.Browser, cfg *SiteConfig) (string, error) {
	page, err := stealth.Page(browser)
	if err != nil {
		return "", err
	}
	defer func() {
		if r := recover(); r != nil {
			// Basic panic recovery for production
			logger.Printf("Panic recovered in fetchHTML: %v", r)
			page.MustClose()
			panic(r) // Re-panic after ensuring page is closed
		}
	}()

	page = page.Timeout(90 * time.Second)
	logger.Println("Navigating to page...")
	page.MustNavigate(cfg.CategoryURL)
	page.MustWaitStable()

	// 1. Handle Cookie Banner
	if cfg.Selectors.CookieButton != "" {
		logger.Printf("Searching for cookie button: %s", cfg.Selectors.CookieButton)
		err = rod.Try(func() {
			page.Timeout(10 * time.Second).MustElement(cfg.Selectors.CookieButton).MustClick()
			page.MustWaitStable()
		})
		if err != nil {
			logger.Printf("Cookie button not found or timed out (ignoring): %v", err)
		}
	}

	// 2. Handle Newsletter Popup
	if cfg.Selectors.NewsletterPopup != "" {
		logger.Printf("Searching for newsletter popup: %s", cfg.Selectors.NewsletterPopup)
		err = rod.Try(func() {
			page.Timeout(10 * time.Second).MustElement(cfg.Selectors.NewsletterPopup).MustClick()
			page.MustWaitStable()
		})
		if err != nil {
			logger.Printf("Newsletter popup not found or timed out (ignoring): %v", err)
		}
	}

	// 3. Wait for product list
	logger.Printf("Waiting for product list: %s", cfg.Selectors.ProductListWait)
	page.MustWaitElementsMoreThan(cfg.Selectors.ProductListWait, 1)

	return page.HTML()
}

// --- PARSING FUNCTIONS ---

func parseHTML(html string, cfg *SiteConfig) ([]CoffeeItem, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var items []CoffeeItem
	sel := cfg.Selectors // shorthand

	doc.Find(sel.ProductRow).Each(func(i int, s *goquery.Selection) {
		var item CoffeeItem

		link := s.Find(sel.Link).First()
		item.URL, _ = link.Attr("href")
		item.Name = strings.TrimSpace(link.Text())

		// Keyword Filtering
		nameLower := strings.ToLower(item.Name)
		for _, keyword := range cfg.DisallowedKeywords {
			if strings.Contains(nameLower, strings.ToLower(keyword)) {
				logger.Printf("Skipping item (disallowed keyword '%s'): %s", keyword, item.Name)
				return
			}
		}

		item.Price = parsePrice(s.Find(sel.Price).First().Text())
		if sel.Origin != "" {
			item.Origin = strings.TrimSpace(s.Find(sel.Origin).First().Text())
		}

		// Stock Status Generic Logic
		if sel.StockButton != "" && s.Find(sel.StockButton).Length() > 0 {
			item.StockStatus = "In Stock"
		} else if sel.StockComingSoon != "" && s.Find(sel.StockComingSoon).Length() > 0 {
			// simplified 'contains' check might be needed here depending on generic needs
			item.StockStatus = "Coming Soon"
		} else {
			item.StockStatus = "Out of Stock"
		}

		// Description Logic (generic vs next-row quirk)
		if sel.Description != "" {
			if sel.DescriptionIsNextRow {
				item.Description = strings.TrimSpace(s.Next().Find(sel.Description).Text())
			} else {
				item.Description = strings.TrimSpace(s.Find(sel.Description).Text())
			}
		}

		// Add if valid
		if item.URL != "" && item.Name != "" {
			items = append(items, item)
		}
	})

	return items, nil
}

var rePrice = regexp.MustCompile(`[^\d\.]+`)

func parsePrice(priceStr string) float64 {
	cleaned := rePrice.ReplaceAllString(priceStr, "")
	price, _ := strconv.ParseFloat(cleaned, 64)
	return price
}
