package main

import (
    "database/sql"
    "fmt"
    "html/template"
    "log"
    "net/http"
    "strconv"
    "strings"
    "time"

    _ "github.com/lib/pq"
    "gopkg.in/yaml.v3"
    "os"
)

// Config struct
type Config struct {
    Server struct {
        Port int `yaml:"port"`
		Subpath string `yaml:"subpath"`
    } `yaml:"server"`

    Auth struct {
        Password string `yaml:"password"`
    } `yaml:"auth"`

    Database struct {
        Host     string `yaml:"host"`
        Port     int    `yaml:"port"`
        User     string `yaml:"user"`
        Password string `yaml:"password"`
        DBName   string `yaml:"dbname"`
        SSLMode  string `yaml:"sslmode"`
    } `yaml:"database"`
}

// Domain struct
type Domain struct {
    ID             int
    DomainName     string
    ExpirationDate time.Time
    DaysRemaining  int
}

var (
    db     *sql.DB
    tmpl   *template.Template
    config Config
)

func main() {
    // Load config
    if err := loadConfig("config.yml"); err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Initialize database
    initDB()
    defer db.Close()

    // Parse templates
    tmpl = template.Must(template.ParseGlob("templates/*.html"))

	// ! MUX - new
	mux := http.NewServeMux()

	mux.HandleFunc("/", authMiddleware(listDomains))
	mux.HandleFunc("/add", authMiddleware(addDomain))
	mux.HandleFunc("/edit", authMiddleware(editDomain))
	mux.HandleFunc("/delete", authMiddleware(deleteDomain))
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/logout", logoutHandler)

	var handler http.Handler = mux
	if config.Server.Subpath != "" {
		handler = http.StripPrefix(config.Server.Subpath, mux)
		log.Printf("📍 Running behind proxy with subpath: %s", config.Server.Subpath)
	} else {
		log.Printf("📍 Running directly (no proxy)")
	}

	// ! HTTP - old
	// http.HandleFunc("/", authMiddleware(listDomains))
    // http.HandleFunc("/add", authMiddleware(addDomain))
    // http.HandleFunc("/edit", authMiddleware(editDomain))
    // http.HandleFunc("/delete", authMiddleware(deleteDomain))
    // http.HandleFunc("/login", loginHandler)
    // http.HandleFunc("/logout", logoutHandler)
	
	// !! DO NOT USE
	//handler := http.StripPrefix(config.Server.Subpath, http.DefaultServeMux)

    // Start server
    addr := fmt.Sprintf(":%d", config.Server.Port)
    log.Printf("🚀 Server starting on http://localhost%s", addr)
	// MUX :
	log.Fatal(http.ListenAndServe(addr, handler))
	// HTTP : log.Fatal(http.ListenAndServe(addr, nil))
}

// Load config from YAML
func loadConfig(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("error reading config file: %w", err)
    }

    if err := yaml.Unmarshal(data, &config); err != nil {
        return fmt.Errorf("error parsing config file: %w", err)
    }

    // Set defaults if not specified
    if config.Server.Port == 0 {
        config.Server.Port = 8080
    }

    return nil
}

// Initialize PostgreSQL database
func initDB() {
    connStr := fmt.Sprintf(
        "host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        config.Database.Host,
        config.Database.Port,
        config.Database.User,
        config.Database.Password,
        config.Database.DBName,
        config.Database.SSLMode,
    )

    var err error
    db, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatalf("Failed to open database: %v", err)
    }

    if err := db.Ping(); err != nil {
        log.Fatalf("Database connection failed: %v", err)
    }

    // Create table if not exists
    createTableSQL := `
    CREATE TABLE IF NOT EXISTS domains (
        id SERIAL PRIMARY KEY,
        domain_name TEXT NOT NULL UNIQUE,
        expiration_date TIMESTAMP NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );`

    if _, err := db.Exec(createTableSQL); err != nil {
        log.Fatalf("Failed to create table: %v", err)
    }

    log.Println("✅ Connected to PostgreSQL")
}

// Authentication middleware
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        cookie, err := r.Cookie("auth")
        if err != nil || cookie.Value != "authenticated" {
            http.Redirect(w, r, "/login", http.StatusSeeOther)
            return
        }
        next(w, r)
    }
}

// Login handler
func loginHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        password := r.FormValue("password")

        if password == config.Auth.Password {
            http.SetCookie(w, &http.Cookie{
                Name:    "auth",
                Value:   "authenticated",
                Expires: time.Now().Add(24 * time.Hour),
                Path:    "/",
            })
            http.Redirect(w, r, "/", http.StatusSeeOther)
            return
        }

        // Wrong password - show error
        tmpl.ExecuteTemplate(w, "login.html", map[string]interface{}{
            "Error": "❌ Invalid password. Please try again.",
        })
        return
    }

    // Show login page
    tmpl.ExecuteTemplate(w, "login.html", nil)
}

// Logout handler
func logoutHandler(w http.ResponseWriter, r *http.Request) {
    http.SetCookie(w, &http.Cookie{
        Name:   "auth",
        Value:  "",
        MaxAge: -1,
        Path:   "/",
    })
    http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// List all domains
func listDomains(w http.ResponseWriter, r *http.Request) {
    rows, err := db.Query(`
        SELECT id, domain_name, expiration_date,
               EXTRACT(DAY FROM (expiration_date - NOW()))::INTEGER as days_remaining
        FROM domains 
        ORDER BY expiration_date ASC
    `)
    if err != nil {
        http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var domains []Domain
    for rows.Next() {
        var d Domain
        var daysRemaining sql.NullInt64
        err := rows.Scan(&d.ID, &d.DomainName, &d.ExpirationDate, &daysRemaining)
        if err != nil {
            http.Error(w, "Error reading data: "+err.Error(), http.StatusInternalServerError)
            return
        }

        if daysRemaining.Valid {
            d.DaysRemaining = int(daysRemaining.Int64)
        } else {
            d.DaysRemaining = int(d.ExpirationDate.Sub(time.Now()).Hours() / 24)
        }

        domains = append(domains, d)
    }

    data := struct {
        Domains []Domain
    }{
        Domains: domains,
    }

    tmpl.ExecuteTemplate(w, "index.html", data)
}

// Add domain
func addDomain(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        domainName := strings.TrimSpace(r.FormValue("domain_name"))
        expirationDate := r.FormValue("expiration_date")

        if domainName == "" || expirationDate == "" {
            tmpl.ExecuteTemplate(w, "form.html", map[string]interface{}{
                "Title":   "Add New Domain",
                "Action":  "/add",
                "Domain":  Domain{},
                "Error":   "❌ All fields are required",
            })
            return
        }

        expDate, err := time.Parse("2006-01-02", expirationDate)
        if err != nil {
            tmpl.ExecuteTemplate(w, "form.html", map[string]interface{}{
                "Title":   "Add New Domain",
                "Action":  "/add",
                "Domain":  Domain{},
                "Error":   "❌ Invalid date format. Use YYYY-MM-DD",
            })
            return
        }

        _, err = db.Exec(
            "INSERT INTO domains (domain_name, expiration_date) VALUES ($1, $2)",
            domainName, expDate,
        )
        if err != nil {
            tmpl.ExecuteTemplate(w, "form.html", map[string]interface{}{
                "Title":   "Add New Domain",
                "Action":  "/add",
                "Domain":  Domain{},
                "Error":   "❌ " + err.Error(),
            })
            return
        }

        http.Redirect(w, r, "/", http.StatusSeeOther)
        return
    }

    // GET request - show form
    tmpl.ExecuteTemplate(w, "form.html", map[string]interface{}{
        "Title":   "Add New Domain",
        "Action":  "/add",
        "Domain":  Domain{},
        "Error":   "",
    })
}

// Edit domain
func editDomain(w http.ResponseWriter, r *http.Request) {
    idStr := r.URL.Query().Get("id")
    if idStr == "" {
        http.Error(w, "Missing ID", http.StatusBadRequest)
        return
    }

    id, err := strconv.Atoi(idStr)
    if err != nil {
        http.Error(w, "Invalid ID", http.StatusBadRequest)
        return
    }

    if r.Method == http.MethodPost {
        domainName := strings.TrimSpace(r.FormValue("domain_name"))
        expirationDate := r.FormValue("expiration_date")

        if domainName == "" || expirationDate == "" {
            // Get existing domain data for the form
            var d Domain
            db.QueryRow(
                "SELECT id, domain_name, expiration_date FROM domains WHERE id = $1", id,
            ).Scan(&d.ID, &d.DomainName, &d.ExpirationDate)

            tmpl.ExecuteTemplate(w, "form.html", map[string]interface{}{
                "Title":   "Edit Domain",
                "Action":  "/edit?id=" + idStr,
                "Domain":  d,
                "Error":   "❌ All fields are required",
            })
            return
        }

        expDate, err := time.Parse("2006-01-02", expirationDate)
        if err != nil {
            var d Domain
            db.QueryRow(
                "SELECT id, domain_name, expiration_date FROM domains WHERE id = $1", id,
            ).Scan(&d.ID, &d.DomainName, &d.ExpirationDate)

            tmpl.ExecuteTemplate(w, "form.html", map[string]interface{}{
                "Title":   "Edit Domain",
                "Action":  "/edit?id=" + idStr,
                "Domain":  d,
                "Error":   "❌ Invalid date format. Use YYYY-MM-DD",
            })
            return
        }

        _, err = db.Exec(
            "UPDATE domains SET domain_name = $1, expiration_date = $2 WHERE id = $3",
            domainName, expDate, id,
        )
        if err != nil {
            var d Domain
            db.QueryRow(
                "SELECT id, domain_name, expiration_date FROM domains WHERE id = $1", id,
            ).Scan(&d.ID, &d.DomainName, &d.ExpirationDate)

            tmpl.ExecuteTemplate(w, "form.html", map[string]interface{}{
                "Title":   "Edit Domain",
                "Action":  "/edit?id=" + idStr,
                "Domain":  d,
                "Error":   "❌ " + err.Error(),
            })
            return
        }

        http.Redirect(w, r, "/", http.StatusSeeOther)
        return
    }

    // GET request - show edit form
    var d Domain
    err = db.QueryRow(
        "SELECT id, domain_name, expiration_date FROM domains WHERE id = $1", id,
    ).Scan(&d.ID, &d.DomainName, &d.ExpirationDate)
    if err != nil {
        http.Error(w, "Domain not found", http.StatusNotFound)
        return
    }

    tmpl.ExecuteTemplate(w, "form.html", map[string]interface{}{
        "Title":   "Edit Domain",
        "Action":  "/edit?id=" + idStr,
        "Domain":  d,
        "Error":   "",
    })
}

// Delete domain
func deleteDomain(w http.ResponseWriter, r *http.Request) {
    idStr := r.URL.Query().Get("id")
    if idStr == "" {
        http.Error(w, "Missing ID", http.StatusBadRequest)
        return
    }

    id, err := strconv.Atoi(idStr)
    if err != nil {
        http.Error(w, "Invalid ID", http.StatusBadRequest)
        return
    }

    _, err = db.Exec("DELETE FROM domains WHERE id = $1", id)
    if err != nil {
        http.Error(w, "Delete error: "+err.Error(), http.StatusInternalServerError)
        return
    }

    http.Redirect(w, r, "/", http.StatusSeeOther)
}
