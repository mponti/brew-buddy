# Brew Buddy ☕

A vibe-coded project to try out AI coding capabilities and learn more about Go. It's designed to scrape, filter, and archive green coffee inventory data. Built with Go and designed to run autonomously as a Kubernetes CronJob. See roadmap below for additional features upcoming.

## What This Project Does

This application automates the process of tracking green coffee availability from online vendors. It performs the following actions:

* **Headless Browsing:** Uses a real Chromium browser (via `go-rod`) to navigate target sites, bypass JavaScript-based bot detection, and handle cookie/newsletter modals.
* **Intelligent Scraping:** Waits for dynamic content to load before parsing the DOM.
* **Automatic Filtering:** Pre-filters results to exclude blends, roasted coffee, and samplers, ensuring only single-origin green coffees are stored.
* **Historical Tracking:** Uses an "UPSERT" strategy with "soft deletes." Items currently on the site are marked active; items removed are marked inactive but retained in the database for historical analysis.
* **Data Enrichment:** Captures detailed metadata including origin, price, score, tasting notes, and full descriptions.

## Why It's Useful

Green coffee inventory changes frequently. Highly desirable single-origin lots often sell out quickly, and new arrivals drop throughout the year. This tool allows you to:

1.  **Never Miss a Coffee:** Automate checking for new arrivals that match your preferred flavor profile.
2.  **Track Inventory History:** See when specific origins typically arrive during the year.
3.  **Build a Personal Database:** Maintain a persistent history of coffees you've seen or purchased, even after they are removed from the retailer's website.

## Tech Stack

* **Language:** Go (Golang) 1.25+
* **Browser Automation:** [go-rod](https://github.com/go-rod/rod) with [stealth](https://github.com/go-rod/stealth) plugin.
* **Parsing:** [goquery](https://github.com/PuerkitoBio/goquery)
* **Database:** SQLite3
* **Deployment:** Docker & Kubernetes (CronJob)

## Getting Started

### Prerequisites

* Docker
* (Optional) `sqlite3` command-line tool for inspecting the data.

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
      -e "DB_PATH=/data/coffee.db" \
      -e "CONFIG_PATH=/app/config.yaml" \
      -e "CATEGORY_URL=https://YOUR_TARGET_[SITE.com/green-coffee.html?product_list_limit=all](https://SITE.com/green-coffee.html?product_list_limit=all)" \
      -v "$(pwd)/local-data:/data" \
      -v "$(pwd)/config.yaml:/app/config.yaml" \
      brew-buddy:latest
    ```

Once complete, your data will be available in `./local-data/coffee.db`.

### Configuration

The application is configured via a combination of environment variables (for infrastructure) and a YAML file (for scraper logic).

| Variable | Description | Example |
| :--- | :--- | :--- |
| `DB_PATH` | Path within the container to save the SQLite DB. | `/data/coffee.db` |
| `CONFIG_PATH` | Path within the container to the YAML config file. | `/app/config.yaml` |

`config.yaml`

This file controls how the scraper interacts with the target site. See `config.example.yaml` for a template.

## Deployment (Kubernetes)

A sample `k8s-spec.yaml` is included for deploying this as a CronJob.

1.  Push your Docker image to a registry your cluster can access.
2.  Update the `image:` field in `k8s-spec.yaml`.
3.  Update the `CATEGORY_URL` environment variable in `k8s-spec.yaml` to your target.
4.  Apply the configuration:
    ```bash
    kubectl apply -f k8s-spec.yaml
    ```
    *Note: This will create a 1Gi Persistent Volume Claim (PVC) to store the database.*

## Using the Data

Here are some useful SQL queries to get started with your data.

**Open the database:**
```bash
sqlite3 ./local-data/coffee.db
```

**See currently available coffees**
```sql
SELECT name, origin, price FROM coffee WHERE is_active = 1;
```

**Find coffees matching a flavor profile (e.g., "fruity"):**
```sql
SELECT name, origin, description
FROM coffee
WHERE is_active = 1 AND description LIKE '%fruity%';
```

**See which origins appear most frequently:**
```sql
SELECT origin, COUNT(*) as count
FROM coffee
GROUP BY origin
ORDER BY count DESC;
```

## Roadmap

* [x] Core scraper with headless browser.
* [x] SQLite storage with history tracking (soft deletes).
* [x] Filtering for blends/roasted coffee via deny list.
* [x] External YAML configuration.
* [ ] **AI-Powered Semantic Search:** Integrate LLM embeddings to allow for "vibe-based" searching of coffee profiles (e.g., "Find me something funky and bright").
* [ ] Personal tasting notes table.
* [ ] UI for viewing and filtering coffees.

## Help & Support

If you encounter issues or have questions, please open an issue in this repository.
That being said this is a project for learning and building something I find useful, your mileage may vary.

## Maintainers

Maintained by mponti. Contributions are welcome!
