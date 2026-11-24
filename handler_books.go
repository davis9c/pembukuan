package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------- dashboard ----------

func dashboard(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	books, err := getBooksForUser(db, s.UserID, s.Role == "super_admin")
	if err != nil {
		tmpl.ExecuteTemplate(w, "index.html", PageData{"Error": "Gagal memuat data"}.WithUser(s))
		return
	}
	success := r.URL.Query().Get("success")
	tmpl.ExecuteTemplate(w, "index.html", PageData{
		"Books":   books,
		"Success": success,
	}.WithUser(s))
}

// ---------- books ----------

func createBookPage(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	errMsg := r.URL.Query().Get("error")
	tmpl.ExecuteTemplate(w, "create-book.html", PageData{"Error": errMsg}.WithUser(s))
}

func createBookHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		redirectError(w, r, "/books/new", "Nama+buku+wajib+diisi")
		return
	}
	desc := strings.TrimSpace(r.FormValue("description"))
	id, err := createBook(db, name, desc)
	if err != nil {
		redirectError(w, r, "/books/new", "Gagal+membuat+buku")
		return
	}
	// auto-add creator as book admin
	addUserToBook(db, int(id), s.UserID, "admin")
	http.Redirect(w, r, "/books/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func viewBook(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	bookID := getBookID(r.URL.Path)
	if bookID == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// check access
	canView := s.Role == "super_admin"
	canEdit := s.Role == "super_admin"
	canManage := s.Role == "super_admin"
	if !canView {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canView = true
			canEdit = role == "admin" || role == "editor"
			canManage = s.Role == "super_admin" || role == "admin"
		}
	}
	if !canView {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	book, err := getBook(db, bookID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	entries, _ := getEntries(db, bookID)
	debit, credit, _ := getBalance(db, bookID)
	success := r.URL.Query().Get("success")

	// load attachments for all entries
	attMap := make(map[int][]Attachment)
	for _, e := range entries {
		atts, _ := getAttachments(db, e.ID)
		if len(atts) > 0 {
			attMap[e.ID] = atts
		}
	}

	tmpl.ExecuteTemplate(w, "book.html", PageData{
		"Book":        book,
		"Entries":     entries,
		"Attachments": attMap,
		"TotalDebit":  debit,
		"TotalCredit": credit,
		"Balance":     debit - credit,
		"CanEdit":     canEdit,
		"CanManage":   canManage,
		"Success":     success,
	}.WithUser(s))
}

func deleteBookHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	if s.Role != "super_admin" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	bookID := getBookID(r.URL.Path)
	deleteBook(db, bookID)
	http.Redirect(w, r, "/?success=Buku+berhasil+dihapus", http.StatusSeeOther)
}

// ---------- entries ----------

func addEntryPage(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	bookID := getBookID(r.URL.Path)

	// check edit access
	canEdit := s.Role == "super_admin"
	if !canEdit {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canEdit = role == "admin" || role == "editor"
		}
	}
	if !canEdit {
		http.Redirect(w, r, "/books/"+strconv.Itoa(bookID), http.StatusSeeOther)
		return
	}

	book, err := getBook(db, bookID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	errMsg := r.URL.Query().Get("error")
	tmpl.ExecuteTemplate(w, "add-entry.html", PageData{
		"Book":  book,
		"Error": errMsg,
	}.WithUser(s))
}

func addEntryHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	bookID := getBookID(r.URL.Path)

	// check edit access
	canEdit := s.Role == "super_admin"
	if !canEdit {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canEdit = role == "admin" || role == "editor"
		}
	}
	if !canEdit {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 50<<20) // 50MB total
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		redirectError(w, r, "/books/"+strconv.Itoa(bookID)+"/entries/new", "File+terlalu+besar")
		return
	}

	dateMode := r.FormValue("date_mode")
	date := r.FormValue("date")
	if dateMode == "auto" || date == "" {
		date = nowStr()
	} else {
		t, err := time.Parse("2006-01-02T15:04", date)
		if err != nil {
			date = nowStr()
		} else {
			date = t.Format("2006-01-02T15:04")
		}
	}

	etype := r.FormValue("type")
	desc := strings.TrimSpace(r.FormValue("description"))
	amountStr := r.FormValue("amount")
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil || amount <= 0 {
		redirectError(w, r, "/books/"+strconv.Itoa(bookID)+"/entries/new", "Jumlah+harus+angka+positif")
		return
	}

	entryID, err := addEntry(db, bookID, s.UserID, date, desc, etype, amount)
	if err != nil {
		redirectError(w, r, "/books/"+strconv.Itoa(bookID)+"/entries/new", "Gagal+menambah+entri")
		return
	}

	// handle file uploads
	if r.MultipartForm != nil && r.MultipartForm.File["files"] != nil {
		allowedMimes := map[string]bool{
			"image/jpeg": true, "image/png": true, "image/gif": true,
			"image/webp": true, "application/pdf": true,
		}
		files := r.MultipartForm.File["files"]
		saved := 0
		for _, fh := range files {
			if saved >= 5 {
				break
			}
			mimeType := fh.Header.Get("Content-Type")
			if !allowedMimes[mimeType] {
				continue
			}
			if err := saveAttachment(db, uploadDir, int(entryID), fh); err != nil {
				continue
			}
			saved++
		}
	}

	http.Redirect(w, r, "/books/"+strconv.Itoa(bookID)+"?success=Entri+berhasil+ditambah", http.StatusSeeOther)
}