package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AppConfig holds infrastructure config from standard env vars
type AppConfig struct {
	DBPath     string
	ConfigPath string // Path to the YAML config file
}

// SiteConfig holds all target-site specific settings (from YAML)
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

// GetAppConfig reads basic infrastructure settings from environment variables.
func GetAppConfig() (AppConfig, error) {
	dbPath := os.Getenv("DB_PATH")
	configPath := os.Getenv("CONFIG_PATH")

	// Set defaults if not provided
	if dbPath == "" {
		dbPath = "./local-data/coffee.db"
	}
	if configPath == "" {
		configPath = "config.yaml"
	}

	return AppConfig{
		DBPath:     dbPath,
		ConfigPath: configPath,
	}, nil
}

// LoadSiteConfig reads the YAML file to configure the scraper.
func LoadSiteConfig(path string) (*SiteConfig, error) {
	// We use os.ReadFile just like before
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file at '%s': %w", path, err)
	}
	var cfg SiteConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}
	return &cfg, nil
}
