package cmd

import (
	"context"
//	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"mspro-labs/brew-buddy/internal/ai"
	"mspro-labs/brew-buddy/internal/config"
	"mspro-labs/brew-buddy/internal/db"
	"mspro-labs/brew-buddy/internal/searcher"
	"mspro-labs/brew-buddy/internal/web"
)

// Helper for templates
var funcMap = template.FuncMap{
	"mul": func(a, b float32) float32 { return a * b },
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Web UI server",
	Run: func(cmd *cobra.Command, args []string) {
		runServer()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServer() {
	// 1. Setup
	appCfg, err := config.GetAppConfig()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}
	database, err := db.Connect(appCfg.DBPath)
	if err != nil {
		log.Fatalf("Database error: %v", err)
	}
	defer database.Close()

	// 2. Initialize AI
	// We need this alive as long as the server is running.
	ctx := context.Background()
	aiClient, err := ai.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize AI: %v", err)
	}
	defer aiClient.Close()

	// 3. Pre-build Templates (SEPARATELY to avoid block collisions)
	// A. Base Template (shared layout + funcs)
	base := template.New("base.html").Funcs(funcMap)
	base, err = base.ParseFS(web.GetTemplatesFS(), "templates/base.html")
	if err != nil {
		log.Fatalf("Failed to parse base template: %v", err)
	}

	// B. Home Template (= base + home.html)
	homeTmpl, _ := base.Clone()
	homeTmpl, err = homeTmpl.ParseFS(web.GetTemplatesFS(), "templates/home.html")
	if err != nil {
		log.Fatalf("Failed to parse home template: %v", err)
	}

	// C. Search Template (= base + search.html)
	searchTmpl, _ := base.Clone()
	searchTmpl, err = searchTmpl.ParseFS(web.GetTemplatesFS(), "templates/search.html")
	if err != nil {
		log.Fatalf("Failed to parse search template: %v", err)
	}
		
	// 4. Define Routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		// 1. Fetch data
		coffees, err := db.GetActiveCoffees(database)
		if err != nil {
			log.Printf("DB error: %v", err)
			http.Error(w, "Failed to load coffees", 500)
			return
		}

		// 2. Render 'base.html' (which includes home.html)
		// We pass 'coffees' as the data (.) for the template
		if err := homeTmpl.ExecuteTemplate(w, "base.html", coffees); err != nil {
			log.Printf("Template error: %v", err)
		}
	})

	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// Run Search
		results, err := searcher.Perform(r.Context(), database, aiClient, query)
		if err != nil {
			log.Printf("Search error: %v", err)
			http.Error(w, "Search failed", 500)
			return
		}

		// Filter low-scoring results before rendering
		var filtered []searcher.Result
		for _, res := range results {
			// 0.2 = 20% match threshold. Adjust as desired.
			if res.Score >= 0.2 {
				filtered = append(filtered, res)
			}
		}

		// Render results using a struct to pass both query and results to template
		data := struct {
			Query   string
			Results []searcher.Result
		}{
			Query:   query,
			Results: filtered,
		}

		if err := searchTmpl.ExecuteTemplate(w, "base.html", data); err != nil {
			log.Printf("Template error: %v", err)
		}
	})

	// 4. Start Server
	port := ":8080"
	log.Printf("🌍 Web UI started at http://localhost%s", port)
	server := &http.Server{
		Addr:         port,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
