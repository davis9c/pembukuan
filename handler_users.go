package main

import (
	"net/http"
	"strconv"
)

// ---------- user management per book ----------

func manageUsersPage(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	book, bookID := resolveBook(r.URL.Path)
	if book == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	canManage := s.Role == "super_admin"
	if !canManage {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canManage = role == "owner" || role == "admin"
		}
	}
	if !canManage {
		http.Redirect(w, r, "/books/"+book.Slug, http.StatusSeeOther)
		return
	}

	bookUsers, _ := getBookUsers(db, bookID)
	success := r.URL.Query().Get("success")
	errMsg := r.URL.Query().Get("error")

	pd := PageData{
		"Book":      book,
		"BookUsers": bookUsers,
		"Success":   success,
		"Error":     errMsg,
	}.WithUser(s)

	if r.URL.Query().Get("fragment") == "1" {
		tmpl.ExecuteTemplate(w, "users_fragment.html", pd)
		return
	}
	tmpl.ExecuteTemplate(w, "users.html", pd)
}

func addUserToBookHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	bookID := resolveBookID(r.URL.Path)

	canManage := s.Role == "super_admin"
	if !canManage {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canManage = role == "owner" || role == "admin"
		}
	}
	if !canManage {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	userID, _ := strconv.Atoi(r.FormValue("user_id"))
	role := r.FormValue("role")
	if userID == 0 || role == "" {
		http.Redirect(w, r, "/books/"+strconv.Itoa(bookID)+"/users?error=Pilih+user+dan+role", http.StatusSeeOther)
		return
	}

	if err := addUserToBook(db, bookID, userID, role); err != nil {
		http.Redirect(w, r, "/books/"+strconv.Itoa(bookID)+"/users?error=Gagal+menambah+user", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/books/"+strconv.Itoa(bookID)+"/users?success=User+berhasil+ditambah", http.StatusSeeOther)
}

func removeUserFromBookHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	book, bookID := resolveBook(r.URL.Path)
	if book == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	canManage := s.Role == "super_admin"
	if !canManage {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canManage = role == "owner" || role == "admin"
		}
	}
	if !canManage {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	userID, _ := strconv.Atoi(r.FormValue("user_id"))
	if userID == 0 {
		http.Redirect(w, r, "/books/"+book.Slug+"/users?error=User+tidak+valid", http.StatusSeeOther)
		return
	}

	removeUserFromBook(db, bookID, userID)
	redir := "/books/" + book.Slug + "/users?success=User+berhasil+dihapus"
	if r.URL.Query().Get("fragment") == "1" {
		redir += "&fragment=1"
	}
	http.Redirect(w, r, redir, http.StatusSeeOther)
}
