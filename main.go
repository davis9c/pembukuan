package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	adminUser = envOrDefault("ADMIN_USER", "admin")
	adminPass = envOrDefault("ADMIN_PASS", "Admin1234")
	dbPath := envOrDefault("DB_PATH", "pembukuan.db")

	var err error
	db, err = initDB(dbPath)
	if err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	uploadsDir := envOrDefault("UPLOADS_DIR", filepath.Join(filepath.Dir(dbPath), "uploads"))
	if err := ensureUploadDir(uploadsDir); err != nil {
		log.Fatalf("failed to create uploads dir: %v", err)
	}

	// UserGate integration — env overrides, then DB settings
	if os.Getenv("USERGATE_ENABLED") == "true" || getSetting(db, "usergate_enabled") == "true" {
		baseURL := os.Getenv("USERGATE_BASE_URL")
		if baseURL == "" {
			baseURL = getSetting(db, "usergate_base_url")
		}
		apiKey := os.Getenv("USERGATE_API_KEY")
		if apiKey == "" {
			apiKey = getSetting(db, "usergate_api_key")
		}
		if baseURL == "" || apiKey == "" {
			log.Printf("UserGate enabled but missing URL or API key — check settings page")
		} else {
			ugClient = NewUserGateClient(baseURL, apiKey)
			log.Printf("usergate enabled: %s", baseURL)
		}
	}

	localAdminEnabled = os.Getenv("LOCAL_ADMIN_ENABLED") != "false"
	if localAdminEnabled {
		log.Printf("local admin login enabled (admin: %s)", adminUser)
	} else {
		log.Printf("local admin login disabled")
	}

	tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*"))

	mux := http.NewServeMux()

	// public
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			loginPage(w, r)
		} else if r.Method == "POST" {
			loginHandler(w, r)
		}
	})

	// auth required
	mux.HandleFunc("/", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		dashboard(w, r)
	}))

	mux.HandleFunc("/logout", authMiddleware(logoutHandler))

	mux.HandleFunc("/books", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/books/new", http.StatusSeeOther)
	}))

	mux.HandleFunc("/books/new", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			createBookPage(w, r)
		} else if r.Method == "POST" {
			createBookHandler(w, r)
		}
	}))

	mux.HandleFunc("/books/", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// route: /books/{id}, /books/{id}/delete, /books/{id}/entries, /books/{id}/entries/new, /books/{id}/users, /books/{id}/users/add, /books/{id}/users/remove
		if len(path) <= 7 {
			http.NotFound(w, r)
			return
		}
		// determine action by suffix
		if r.Method == "POST" {
			if hasSuffix(path, "/delete") {
				deleteBookHandler(w, r)
				return
			}
			if hasSuffix(path, "/entries") {
				addEntryHandler(w, r)
				return
			}
			if hasSuffix(path, "/users/add") {
				addUserToBookHandler(w, r)
				return
			}
			if hasSuffix(path, "/users/remove") {
				removeUserFromBookHandler(w, r)
				return
			}
			if hasSuffix(path, "/attachments") {
				uploadAttachmentHandler(w, r)
				return
			}
			if hasSuffix(path, "/attachments/delete") {
				deleteAttachmentHandler(w, r)
				return
			}
		}
		if r.Method == "GET" {
			if hasSuffix(path, "/entries/new") {
				addEntryPage(w, r)
				return
			}
			if hasSuffix(path, "/users") {
				manageUsersPage(w, r)
				return
			}
		}
		viewBook(w, r)
	}))

	mux.HandleFunc("/attachments/", authMiddleware(serveAttachmentHandler))

	// settings (super_admin only)
	mux.HandleFunc("/settings", requireSuperAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			settingsPage(w, r)
			return
		}
	}))
	mux.HandleFunc("/settings/users", requireSuperAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			createLocalUserHandler(w, r)
			return
		}
	}))
	mux.HandleFunc("/settings/users/edit", requireSuperAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			editUserRoleHandler(w, r)
			return
		}
	}))
	mux.HandleFunc("/settings/users/delete", requireSuperAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			deleteLocalUserHandler(w, r)
			return
		}
	}))
	mux.HandleFunc("/settings/usergate/sync", requireSuperAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			syncGateUsersHandler(w, r)
			return
		}
	}))
	mux.HandleFunc("/settings/usergate/import", requireSuperAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			importGateUserHandler(w, r)
			return
		}
	}))
	mux.HandleFunc("/settings/usergate/set-superadmin", requireSuperAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			setGateSuperAdminHandler(w, r)
			return
		}
	}))
	mux.HandleFunc("/settings/usergate/config", requireSuperAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			saveGateConfigHandler(w, r)
			return
		}
	}))

	port := envOrDefault("PORT", "8080")
	log.Printf("server starting on :%s (admin: %s)", port, adminUser)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func envOrDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}