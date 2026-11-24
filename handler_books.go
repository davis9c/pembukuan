package main

import (
	"encoding/json"
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
	var owned, shared []Book
	for _, b := range books {
		if s.Role == "super_admin" || b.OwnerID == s.UserID {
			owned = append(owned, b)
		} else {
			shared = append(shared, b)
		}
	}
	success := r.URL.Query().Get("success")
	tmpl.ExecuteTemplate(w, "index.html", PageData{
		"OwnedBooks":  owned,
		"SharedBooks": shared,
		"Success":     success,
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
	openingStr := r.FormValue("opening_balance")
	var opening int64
	if openingStr != "" {
		v, err := strconv.ParseInt(openingStr, 10, 64)
		if err == nil {
			opening = v
		}
	}
	id, err := createBook(db, name, desc, s.UserID, opening)
	if err != nil {
		redirectError(w, r, "/books/new", "Gagal+membuat+buku")
		return
	}
	insertAuditLog(db, s.UserID, int(id), "book", int(id), "create_book", "", name, r.RemoteAddr)
	http.Redirect(w, r, "/books/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func viewBook(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	bookID := getBookID(r.URL.Path)
	if bookID == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	canView := s.Role == "super_admin"
	canEdit := s.Role == "super_admin"
	canManage := s.Role == "super_admin"
	if !canView {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canView = true
			canEdit = role == "owner" || role == "admin" || role == "editor"
			canManage = role == "owner" || role == "admin"
		}
	}
	if !canView {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// super admin access audit
	if s.Role == "super_admin" {
		insertAuditLog(db, s.UserID, bookID, "book", bookID, "admin_view_book", "", "", r.RemoteAddr)
	}

	book, err := getBook(db, bookID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	entries, _ := getEntries(db, bookID)

	// running balance
	bal := book.OpeningBalance
	for i := range entries {
		if entries[i].Voided {
			entries[i].RunningBalance = bal
			continue
		}
		if entries[i].Type == "debit" {
			bal += entries[i].Amount
		} else {
			bal -= entries[i].Amount
		}
		entries[i].RunningBalance = bal
	}

	debit, credit, _ := getBalance(db, bookID)
	success := r.URL.Query().Get("success")

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
		"Balance":     book.OpeningBalance + debit - credit,
		"CanEdit":     canEdit,
		"CanManage":   canManage,
		"Success":     success,
	}.WithUser(s))
}

// ---------- entries ----------

func addEntryPage(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	bookID := getBookID(r.URL.Path)

	canEdit := s.Role == "super_admin"
	if !canEdit {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canEdit = role == "owner" || role == "admin" || role == "editor"
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

	canEdit := s.Role == "super_admin"
	if !canEdit {
		role, err := getUserBookRole(db, s.UserID, bookID)
		if err == nil {
			canEdit = role == "owner" || role == "admin" || role == "editor"
		}
	}
	if !canEdit {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
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
	if etype != "debit" && etype != "credit" {
		etype = "debit"
	}
	desc := strings.TrimSpace(r.FormValue("description"))
	voucherNo := strings.TrimSpace(r.FormValue("voucher_no"))
	category := strings.TrimSpace(r.FormValue("category"))
	party := strings.TrimSpace(r.FormValue("party"))
	note := strings.TrimSpace(r.FormValue("note"))
	amountStr := r.FormValue("amount")
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil || amount <= 0 {
		redirectError(w, r, "/books/"+strconv.Itoa(bookID)+"/entries/new", "Jumlah+harus+angka+positif")
		return
	}

	entryID, err := addEntry(db, bookID, s.UserID, date, voucherNo, desc, etype, category, party, note, amount)
	if err != nil {
		redirectError(w, r, "/books/"+strconv.Itoa(bookID)+"/entries/new", "Gagal+menambah+entri")
		return
	}

	// audit
	after, _ := json.Marshal(map[string]interface{}{
		"date": date, "type": etype, "amount": amount, "description": desc,
	})
	insertAuditLog(db, s.UserID, bookID, "entry", int(entryID), "create_entry", "", string(after), r.RemoteAddr)

	// attachments
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
			if err := saveAttachment(db, uploadDir, int(entryID), fh, s.UserID); err != nil {
				continue
			}
			saved++
		}
	}

	http.Redirect(w, r, "/books/"+strconv.Itoa(bookID)+"?success=Entri+berhasil+ditambah", http.StatusSeeOther)
}

// edit entry
func editEntryPage(w http.ResponseWriter, r *http.Request) {
	// route handled via query ?edit=id on book page? We'll provide /books/{id}/entries/{eid}/edit
	s := getSession(r)
	bookID := getBookID(r.URL.Path)
	entryID := getEntryID(r.URL.Path)
	if entryID == 0 {
		http.Redirect(w, r, "/books/"+strconv.Itoa(bookID), http.StatusSeeOther)
		return
	}
	canEdit := s.Role == "super_admin"
	if !canEdit {
		role, _ := getUserBookRole(db, s.UserID, bookID)
		canEdit = role == "owner" || role == "admin" || role == "editor"
	}
	if !canEdit {
		http.Redirect(w, r, "/books/"+strconv.Itoa(bookID), http.StatusSeeOther)
		return
	}
	entry, err := getEntryByID(db, entryID)
	if err != nil {
		http.Redirect(w, r, "/books/"+strconv.Itoa(bookID), http.StatusSeeOther)
		return
	}
	book, _ := getBook(db, bookID)
	tmpl.ExecuteTemplate(w, "edit-entry.html", PageData{
		"Book":  book,
		"Entry": entry,
	}.WithUser(s))
}

func editEntryHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	bookID := getBookID(r.URL.Path)
	entryID := getEntryID(r.URL.Path)
	if entryID == 0 {
		http.Redirect(w, r, "/books/"+strconv.Itoa(bookID), http.StatusSeeOther)
		return
	}
	canEdit := s.Role == "super_admin"
	if !canEdit {
		role, _ := getUserBookRole(db, s.UserID, bookID)
		canEdit = role == "owner" || role == "admin" || role == "editor"
	}
	if !canEdit {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		redirectError(w, r, "/books/"+strconv.Itoa(bookID), "File+terlalu+besar")
		return
	}

	old, _ := getEntryByID(db, entryID)
	oldJSON, _ := json.Marshal(old)

	date := r.FormValue("date")
	if date == "" {
		date = old.Date
	} else {
		t, err := time.Parse("2006-01-02T15:04", date)
		if err == nil {
			date = t.Format("2006-01-02T15:04")
		}
	}
	etype := r.FormValue("type")
	if etype != "debit" && etype != "credit" {
		etype = old.Type
	}
	desc := strings.TrimSpace(r.FormValue("description"))
	voucherNo := strings.TrimSpace(r.FormValue("voucher_no"))
	category := strings.TrimSpace(r.FormValue("category"))
	party := strings.TrimSpace(r.FormValue("party"))
	note := strings.TrimSpace(r.FormValue("note"))
	amountStr := r.FormValue("amount")
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil || amount <= 0 {
		amount = old.Amount
	}

	updateEntry(db, entryID, s.UserID, date, voucherNo, desc, etype, category, party, note, amount)

	newEntry, _ := getEntryByID(db, entryID)
	newJSON, _ := json.Marshal(newEntry)
	addRevision(db, entryID, s.UserID, string(oldJSON), string(newJSON))
	insertAuditLog(db, s.UserID, bookID, "entry", entryID, "edit_entry", string(oldJSON), string(newJSON), r.RemoteAddr)

	// attachments
	if r.MultipartForm != nil && r.MultipartForm.File["files"] != nil {
		allowedMimes := map[string]bool{
			"image/jpeg": true, "image/png": true, "image/gif": true,
			"image/webp": true, "application/pdf": true,
		}
		files := r.MultipartForm.File["files"]
		count, _ := countAttachments(db, entryID)
		saved := 0
		for _, fh := range files {
			if count+saved >= 5 {
				break
			}
			mimeType := fh.Header.Get("Content-Type")
			if !allowedMimes[mimeType] {
				continue
			}
			if err := saveAttachment(db, uploadDir, entryID, fh, s.UserID); err != nil {
				continue
			}
			saved++
		}
	}

	http.Redirect(w, r, "/books/"+strconv.Itoa(bookID)+"?success=Entri+berhasil+diubah", http.StatusSeeOther)
}
