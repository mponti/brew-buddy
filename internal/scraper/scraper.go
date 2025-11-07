package scraper

import (
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

	"mspro-labs/brew-buddy/internal/config"
	"mspro-labs/brew-buddy/internal/models"
)

var logger = log.New(os.Stdout, "SCRAPER: ", log.LstdFlags|log.Lshortfile)

// Run orchestrates the entire scraping process: launch, fetch, and parse.
func Run(cfg *config.SiteConfig) ([]models.CoffeeItem, error) {
	logger.Println("Launching headless browser...")
	browser, err := launchBrowser()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}
	defer browser.MustClose()

	logger.Printf("Navigating to: %s", cfg.CategoryURL)
	html, err := fetchHTML(browser, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch HTML: %w", err)
	}

	logger.Println("Parsing HTML content...")
	items, err := parseHTML(html, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return items, nil
}

func launchBrowser() (*rod.Browser, error) {
	l := launcher.New().Headless(true).NoSandbox(true)
	u, err := l.Launch()
	if err != nil {
		return nil, err
	}
	return rod.New().ControlURL(u).MustConnect(), nil
}

func fetchHTML(browser *rod.Browser, cfg *config.SiteConfig) (string, error) {
	page, err := stealth.Page(browser)
	if err != nil {
		return "", err
	}

	// Generic panic recovery to ensure browser cleanup
	defer func() {
		if r := recover(); r != nil {
			logger.Printf("Panic in fetchHTML: %v", r)
			page.MustClose()
		}
	}()

	page = page.Timeout(90 * time.Second)

	logger.Println("Navigating...")
	page.MustNavigate(cfg.CategoryURL)
	page.MustWaitStable()

	// Handle Cookie Consent
	if sel := cfg.Selectors.CookieButton; sel != "" {
		logger.Printf("Looking for cookie button: %s", sel)
		// Try to find and click, but don't fail the scrape if it's missing
		_ = rod.Try(func() {
			page.Timeout(5 * time.Second).MustElement(sel).MustClick()
			page.MustWaitStable()
		})
	}

	// Handle Newsletter Popup
	if sel := cfg.Selectors.NewsletterPopup; sel != "" {
		logger.Printf("Looking for newsletter popup: %s", sel)
		_ = rod.Try(func() {
			page.Timeout(5 * time.Second).MustElement(sel).MustClick()
			page.MustWaitStable()
		})
	}

	// Wait for main content
	logger.Printf("Waiting for product list: %s", cfg.Selectors.ProductListWait)
	page.MustWaitElementsMoreThan(cfg.Selectors.ProductListWait, 0)

	return page.HTML()
}

func parseHTML(html string, cfg *config.SiteConfig) ([]models.CoffeeItem, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var items []models.CoffeeItem
	sel := cfg.Selectors

	doc.Find(sel.ProductRow).Each(func(_ int, s *goquery.Selection) {
		var item models.CoffeeItem

		// Basic Details
		link := s.Find(sel.Link).First()
		item.Name = strings.TrimSpace(link.Text())
		item.URL, _ = link.Attr("href")

		// Keyword Filter
		nameLower := strings.ToLower(item.Name)
		for _, kw := range cfg.DisallowedKeywords {
			if strings.Contains(nameLower, strings.ToLower(kw)) {
				logger.Printf("Skipping (keyword '%s'): %s", kw, item.Name)
				return
			}
		}

		// Price & Origin
		item.Price = parsePrice(s.Find(sel.Price).First().Text())
		if sel.Origin != "" {
			item.Origin = strings.TrimSpace(s.Find(sel.Origin).First().Text())
		}

		// Description (handle quirk where it might be in the next row)
		if sel.Description != "" {
			if sel.DescriptionIsNextRow {
				item.Description = strings.TrimSpace(s.Next().Find(sel.Description).Text())
			} else {
				item.Description = strings.TrimSpace(s.Find(sel.Description).Text())
			}
		}

		// Simple Stock Check
		if sel.StockButton != "" && s.Find(sel.StockButton).Length() > 0 {
			item.StockStatus = "In Stock"
		} else if sel.StockComingSoon != "" && s.Find(sel.StockComingSoon).Length() > 0 {
			item.StockStatus = "Coming Soon"
		} else {
			item.StockStatus = "Out of Stock"
		}

		if item.Name != "" && item.URL != "" {
			items = append(items, item)
		}
	})

	return items, nil
}

var rePrice = regexp.MustCompile(`[^\d\.]+`)

func parsePrice(priceStr string) float64 {
	val := rePrice.ReplaceAllString(priceStr, "")
	price, _ := strconv.ParseFloat(val, 64)
	return price
}
