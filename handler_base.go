package main

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

//go:embed templates/*
var templateFS embed.FS

var (
	db        *sql.DB
	tmpl      *template.Template
	adminUser string
	adminPass string
)

var funcMap = template.FuncMap{
	"fmtTime": func(t time.Time) string {
		return t.Format("02 Jan 2006 15:04")
	},
	"fmtDate": func(s string) string {
		t, err := time.Parse("2006-01-02T15:04", s)
		if err != nil {
			return s
		}
		return t.Format("02 Jan 2006 15:04")
	},
	"fmtAmount": func(a int64) string {
		s := fmt.Sprintf("%d", a)
		n := len(s)
		if n <= 3 {
			return s
		}
		var b strings.Builder
		for i, c := range s {
			if i > 0 && (n-i)%3 == 0 {
				b.WriteByte('.')
			}
			b.WriteRune(c)
		}
		return b.String()
	},
	"fmtSize": func(size int64) string {
		if size < 1024 {
			return fmt.Sprintf("%d B", size)
		}
		if size < 1024*1024 {
			return fmt.Sprintf("%.1f KB", float64(size)/1024)
		}
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	},
	"isImage": func(mime string) bool {
		return strings.HasPrefix(mime, "image/")
	},
}

// ---------- middleware ----------

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := getSession(r)
		if s == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// ---------- helpers ----------

type PageData map[string]interface{}

func (p PageData) WithUser(s *Session) PageData {
	if s != nil {
		p["User"] = map[string]interface{}{
			"Username":     s.Username,
			"Role":         s.Role,
			"IsSuperAdmin": s.Role == "super_admin",
		}
	}
	return p
}

func getBookID(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return 0
	}
	id, _ := strconv.Atoi(parts[1])
	return id
}

func redirectError(w http.ResponseWriter, r *http.Request, url, msg string) {
	http.Redirect(w, r, url+"?error="+msg, http.StatusSeeOther)
}

// ---------- login ----------

func loginPage(w http.ResponseWriter, r *http.Request) {
	errMsg := r.URL.Query().Get("error")
	tmpl.ExecuteTemplate(w, "login.html", PageData{"Error": errMsg})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == adminUser && password == adminPass {
		u, err := getUserByUsername(db, username)
		if err != nil {
			hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			createUser(db, username, string(hash), "super_admin")
			u, _ = getUserByUsername(db, username)
		}
		setSession(w, Session{UserID: u.ID, Username: u.Username, Role: "super_admin", Expires: time.Now().Add(24 * time.Hour)})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	hash, err := getUserPasswordHash(db, username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		http.Redirect(w, r, "/login?error=Username+atau+password+salah", http.StatusSeeOther)
		return
	}

	u, _ := getUserByUsername(db, username)
	setSession(w, Session{UserID: u.ID, Username: u.Username, Role: u.Role, Expires: time.Now().Add(24 * time.Hour)})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	clearSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}