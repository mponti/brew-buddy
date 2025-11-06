package main

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestParseHTML provides a static HTML string and a mock configuration to test parsing.
func TestParseHTML(t *testing.T) {
	// 1. Mock Configuration that matches our sample HTML below
	mockCfg := &SiteConfig{
		Selectors: Selectors{
			ProductRow:           "tbody.product-items tr.product-item",
			Link:                 "a.product-item-link",
			Price:                "span.price",
			Origin:               "td.origin",
			StockButton:          "button.action.tocart",
			Description:          "div.short-description",
			DescriptionIsNextRow: true,
		},
		// We will test that this keyword correctly filters out an item
		DisallowedKeywords: []string{"blend"},
	}

	// 2. Sample HTML simulating the target site structure
	const sampleHTML = `
<html>
<body>
  <table id="table-products-list">
    <tbody class="products list items product-items">
      <tr class="item product product-item">
        <td class="origin" data-th="Origin">Decaf</td>
        <td class="product-item-name" data-th="Name">
          <h2><a class="product-item-link" href="https://example.com/coffee1">Example Coffee One</a></h2>
        </td>
        <td class="product-price-info" data-th="Price">
          <span class="price">$25.50</span>
        </td>
        <td class="product-actions">
          <button type="button" class="action tocart primary">Add to Cart</button>
        </td>
      </tr>
      <tr class="catalog-quickview-content">
        <td colspan="4">
          <div class="short-description">This is the description for coffee one.</div>
        </td>
      </tr>

      <tr class="item product product-item">
        <td class="origin" data-th="Origin">Colombia</td>
        <td class="product-item-name" data-th="Name">
          <h2><a class="product-item-link" href="https://example.com/coffee2">Example Espresso Blend</a></h2>
        </td>
        <td class="product-price-info" data-th="Price">
          <span class="price">$19.99</span>
        </td>
        <td class="product-actions">
           <button type="button" class="action tocart primary">Add to Cart</button>
        </td>
      </tr>
      <tr class="catalog-quickview-content">
        <td colspan="4">
          <div class="short-description">This is a blend description.</div>
        </td>
      </tr>

      <tr class="item product product-item">
        <td class="origin" data-th="Origin">Kenya</td>
        <td class="product-item-name" data-th="Name">
          <h2><a class="product-item-link" href="https://example.com/coffee3">Kenya Kiambu</a></h2>
        </td>
        <td class="product-price-info" data-th="Price">
          <span class="price">$22.00</span>
        </td>
        <td class="product-actions">
           </td>
      </tr>
      <tr class="catalog-quickview-content">
        <td colspan="4">
          <div class="short-description">This is the description for coffee three.</div>
        </td>
      </tr>
    </tbody>
  </table>
</body>
</html>
	`

	// 3. Run Parser
	items, err := parseHTML(sampleHTML, mockCfg)
	if err != nil {
		t.Fatalf("parseHTML failed: %v", err)
	}

	// 4. Assertions
	// We expect 2 items. "Espresso Blend" should be filtered out.
	if len(items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(items))
	}

	// Check Item 1
	item1 := items[0]
	if item1.Name != "Example Coffee One" {
		t.Errorf("Item 1 Name wrong: expected 'Example Coffee One', got '%s'", item1.Name)
	}
	if item1.Price != 25.50 {
		t.Errorf("Item 1 Price wrong: expected 25.50, got %f", item1.Price)
	}
	if item1.Origin != "Decaf" {
		t.Errorf("Item 1 Origin wrong: expected 'Decaf', got '%s'", item1.Origin)
	}
	if item1.StockStatus != "In Stock" {
		t.Errorf("Item 1 StockStatus wrong: expected 'In Stock', got '%s'", item1.StockStatus)
	}
	if item1.Description != "This is the description for coffee one." {
		t.Errorf("Item 1 Description wrong: got '%s'", item1.Description)
	}

	// Check Item 2 (which is the third item in HTML)
	item2 := items[1]
	if item2.Name != "Kenya Kiambu" {
		t.Errorf("Item 2 Name wrong: expected 'Kenya Kiambu', got '%s'", item2.Name)
	}
	if item2.StockStatus != "Out of Stock" {
		t.Errorf("Item 2 StockStatus wrong: expected 'Out of Stock', got '%s'", item2.StockStatus)
	}
	if item2.Description != "This is the description for coffee three." {
		t.Errorf("Item 2 Description wrong: got '%s'", item2.Description)
	}
}

// TestDatabaseUPSERT tests the insert, update, and is_active logic.
func TestDatabaseUPSERT(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory db: %v", err)
	}
	defer db.Close()

	if err := createSchema(db); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// 1. Test INSERT
	item1 := CoffeeItem{
		URL:   "https://example.com/coffee1",
		Name:  "Test Coffee",
		Price: 10.00,
	}
	items := []CoffeeItem{item1}

	count, err := saveData(db, items)
	if err != nil {
		t.Fatalf("saveData (insert) failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 row affected for insert, got %d", count)
	}

	// Verify insert and default is_active status
	var name string
	var isActive int
	err = db.QueryRow("SELECT name, is_active FROM coffee WHERE url = ?", item1.URL).Scan(&name, &isActive)
	if err != nil {
		t.Fatalf("Failed to query inserted data: %v", err)
	}
	if name != "Test Coffee" {
		t.Errorf("Inserted name mismatch. Got '%s'", name)
	}
	if isActive != 1 {
		t.Errorf("New item should be active (1), got %d", isActive)
	}

	// 2. Test UPDATE (ON CONFLICT)
	// We also test that it stays active
	item2 := CoffeeItem{
		URL:   "https://example.com/coffee1", // Same URL
		Name:  "Test Coffee Updated",
		Price: 12.50,
	}
	items = []CoffeeItem{item2}

	_, err = saveData(db, items)
	if err != nil {
		t.Fatalf("saveData (update) failed: %v", err)
	}

	// Verify update
	err = db.QueryRow("SELECT name, price, is_active FROM coffee WHERE url = ?", item2.URL).Scan(&name, &item2.Price, &isActive)
	if err != nil {
		t.Fatalf("Failed to query updated data: %v", err)
	}
	if name != "Test Coffee Updated" {
		t.Errorf("Updated name mismatch. Got '%s'", name)
	}
	if isActive != 1 {
		t.Errorf("Updated item should remain active (1), got %d", isActive)
	}
}

func TestParsePrice(t *testing.T) {
	testCases := []struct {
		input    string
		expected float64
	}{
		{"$25.50", 25.50},
		{"$19.99", 19.99},
		{"Price $100", 100.0},
		{"$0.99", 0.99},
		{"Free", 0.0},
	}

	for _, tc := range testCases {
		if got := parsePrice(tc.input); got != tc.expected {
			t.Errorf("parsePrice(%q): expected %f, got %f", tc.input, tc.expected, got)
		}
	}
}
