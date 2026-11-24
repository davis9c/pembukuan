package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	adminUser = envOrDefault("ADMIN_USER", "admin")
	adminPass = envOrDefault("ADMIN_PASS", "Admin1234")
	dbPath := envOrDefault("DB_PATH", "pembukuan.db")
	port := envOrDefault("PORT", "8080")
	appBaseURL = strings.TrimRight(envOrDefault("BASE_URL", "http://localhost:"+port), "/")

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

	// UserGate integration — from DB settings only
	baseURL := getSetting(db, "usergate_base_url")
	apiKey := getSetting(db, "usergate_api_key")
	enabled := getSetting(db, "usergate_enabled")
	if enabled == "true" && baseURL != "" && apiKey != "" {
		ugClient = NewUserGateClient(baseURL, apiKey)
		log.Printf("usergate enabled: %s", baseURL)
	} else {
		log.Printf("usergate disabled (configure in /settings)")
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
			if hasSuffix(path, "/entries") && !hasSuffix(path, "/edit") && !hasSuffix(path, "/void") {
				addEntryHandler(w, r)
				return
			}
			if hasSuffix(path, "/edit") {
				editEntryHandler(w, r)
				return
			}
			if hasSuffix(path, "/void") {
				voidEntryHandler(w, r)
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
			if hasSuffix(path, "/share") {
				createShareLinkHandler(w, r)
				return
			}
			if hasSuffix(path, "/share/revoke") {
				revokeShareLinkHandler(w, r)
				return
			}
			if hasSuffix(path, "/leave") {
				leaveBookHandler(w, r)
				return
			}
		}
		if r.Method == "GET" {
			if hasSuffix(path, "/entries/new") {
				addEntryPage(w, r)
				return
			}
			if hasSuffix(path, "/entries/edit") {
				// not a standard suffix; handle via /edit below? fallback viewBook
			}
			if hasSuffix(path, "/edit") {
				editEntryPage(w, r)
				return
			}
			if hasSuffix(path, "/users") {
				manageUsersPage(w, r)
				return
			}
			if hasSuffix(path, "/share") {
				sharePage(w, r)
				return
			}
			if hasSuffix(path, "/audit") {
				auditPage(w, r)
				return
			}
			if hasSuffix(path, "/print") {
				printBookPage(w, r)
				return
			}
		}
		viewBook(w, r)
	}))

	mux.HandleFunc("/attachments/", authMiddleware(serveAttachmentHandler))
	mux.HandleFunc("/share/", shareAcceptHandler)

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
	mux.HandleFunc("/settings/users/toggle", requireSuperAdmin(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			toggleUserActiveHandler(w, r)
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
