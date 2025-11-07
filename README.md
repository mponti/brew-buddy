# Brew Buddy ☕

A vibe-coded project to try out AI coding capabilities and learn more about Go. It's designed to scrape, filter, and archive green coffee inventory data. Built with Go and designed to run autonomously as a Kubernetes CronJob. See roadmap below for additional features upcoming.

## Features

* **🕷️ Robust Scraper:** Headless Chromium browser (via `go-rod`) navigates e-commerce sites, bypassing bot detection and handling dynamic content in a safe, unobtrusive way.
* **🧠 AI-Powered Search:** Integrated Google Gemini embeddings allow you to search for "funky and bright" or "cozy chocolate" and get semantically ranked results.
* **🌐 Web Interface:** A clean, built-in web server to browse inventory and perform AI searches from any device on your network.
* **🗄️ Historical Tracking:** Maintains a long-term database of coffees, even after they are removed from vendor websites.
* **⚡ Automated Workflows:** Scraper automatically triggers AI embedding, keeping your search index up-to-date.

## Why It's Useful

Green coffee inventory changes frequently. Highly desirable single-origin lots often sell out quickly, and new arrivals drop throughout the year. This tool allows you to:

1.  **Never Miss a Coffee:** Automate checking for new arrivals that match your preferred flavor profile.
2.  **Track Inventory History:** See when specific origins typically arrive during the year.
3.  **Build a Personal Database:** Maintain a persistent history of coffees you've seen or purchased, even after they are removed from the retailer's website.

## Tech Stack

* **Language:** Go (Golang) 1.25+
* **CLI Framework:** Cobra
* **Browser Automation:** [go-rod](https://github.com/go-rod/rod) with [stealth](https://github.com/go-rod/stealth) plugin.
* **Parsing:** [goquery](https://github.com/PuerkitoBio/goquery)
* **Deployment:** Docker & Kubernetes (CronJob)
* **AI/ML:** Google Gemini (Embeddings)
* **Datastore:** SQLite3 (with WAL enabled for concurrency)
* **Web UI:** Native Go `html/template` with embedded assets

## Getting Started

### Prerequisites

* Docker (recommended for easy setup)
* OR Go 1.25+ and standard build tools (gcc for SQLite CGO)
* A [Google AI Studio](https://aistudio.google.com/app/apikey) API Key (free tier works great).

### Quick Setup

1.  **Clone and Setup:**
    Run the helper script to create necessary directories and a starter config file.
    ```bash
    ./setup.sh
    ```

2.  **Configure:**
    * Edit `config.yaml` with your target website's URL and CSS selectors.
    * Export your AI API key: `export GEMINI_API_KEY="your-key-here"`

### Setup

Run the included setup script to create necessary local directories and a starter configuration file:

```bash
./setup.sh
```
*IMPORTANT: After running setup, you must edit `config.yaml` and add the actual target URL and CSS selectors for the site you wish to scrape.*


### Running Locally

1.  **Build the Docker image:**
    ```bash
    docker build -t brew-buddy .
    ```

2.  **Run the scraper:**
    ```bash
    docker run --rm \
        -e "GEMINI_API_KEY=your-key" \
        -e "DB_PATH=/data/coffee.db" \
        -e "CONFIG_PATH=/app/config.yaml" \
        -v "$(pwd)/local-data:/data" \
        -v "$(pwd)/config.yaml:/app/config.yaml" \
        brew-buddy scrape
    ```

3.  **Run the web UI:**
    ```bash
    docker run -d \
        -p 8080:8080 \
        -e "GEMINI_API_KEY=your-key" \
        -e "DB_PATH=/data/coffee.db" \
        -e "CONFIG_PATH=/app/config.yaml" \
        -v "$(pwd)/local-data:/data" \
        -v "$(pwd)/config.yaml:/app/config.yaml" \
        brew-buddy serve
    ```

Access the UI at `http://localhost:8080`

### Configuration

The application is configured via a combination of environment variables (for infrastructure) and a YAML file (for scraper logic).

| Variable | Description | Example |
| :--- | :--- | :--- |
| `DB_PATH` | Path within the container to save the SQLite DB. | `/data/coffee.db` |
| `CONFIG_PATH` | Path within the container to the YAML config file. | `/app/config.yaml` |
| `GEMINI_API_KEY` | (Optional) Google Gemini API key for semantic search. | `AIzaSy...` |

`config.yaml`

This file controls how the scraper interacts with the target site. See `config.example.yaml` for a template.

## Roadmap

* [x] Core scraper with headless browser.
* [x] SQLite storage with history tracking (soft deletes).
* [x] Filtering for blends/roasted coffee via deny list.
* [x] External YAML configuration.
* [x] **AI-Powered Semantic Search:** Integrate LLM embeddings to allow for "vibe-based" searching of coffee profiles (e.g., "Find me something funky and bright").
* [x] UI for viewing and filtering coffees.
* [ ] Personal tasting notes table.
* [ ] Leverage semantic search to build a coffee blend tool.
* [ ] Notifications and tracking of origin/time of year stats for matching coffees.

## Help & Support

If you encounter issues or have questions, please open an issue in this repository.
That being said this is a project for learning and building something I find useful, your mileage may vary.

## Maintainers

Maintained by mponti. Contributions are welcome!
