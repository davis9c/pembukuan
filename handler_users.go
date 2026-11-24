package main

import (
	"net/http"
	"strconv"
)

// ---------- user management per book ----------

func manageUsersPage(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	bookID := getBookID(r.URL.Path)

	canManage := s.Role == "super_admin"
	if !canManage {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canManage = role == "admin"
		}
	}
	if !canManage {
		http.Redirect(w, r, "/books/"+strconv.Itoa(bookID), http.StatusSeeOther)
		return
	}

	book, err := getBook(db, bookID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	allUsers, _ := getAllUsers(db)
	bookUsers, _ := getBookUsers(db, bookID)

	success := r.URL.Query().Get("success")
	errMsg := r.URL.Query().Get("error")

	tmpl.ExecuteTemplate(w, "users.html", PageData{
		"Book":      book,
		"AllUsers":  allUsers,
		"BookUsers": bookUsers,
		"Success":   success,
		"Error":     errMsg,
	}.WithUser(s))
}

func addUserToBookHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	bookID := getBookID(r.URL.Path)

	canManage := s.Role == "super_admin"
	if !canManage {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canManage = role == "admin"
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
	bookID := getBookID(r.URL.Path)

	canManage := s.Role == "super_admin"
	if !canManage {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canManage = role == "admin"
		}
	}
	if !canManage {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	userID, _ := strconv.Atoi(r.FormValue("user_id"))
	if userID == 0 {
		http.Redirect(w, r, "/books/"+strconv.Itoa(bookID)+"/users?error=User+tidak+valid", http.StatusSeeOther)
		return
	}

	removeUserFromBook(db, bookID, userID)
	http.Redirect(w, r, "/books/"+strconv.Itoa(bookID)+"/users?success=User+berhasil+dihapus", http.StatusSeeOther)
}