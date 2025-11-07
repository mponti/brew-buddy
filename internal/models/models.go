package models

// CoffeeItem holds the scraped data for a single product.
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
