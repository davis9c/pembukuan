package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func sharePage(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	book, bookID := resolveBook(r.URL.Path)
	if book == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	canManage := false
	if role, err := getUserBookRole(db, s.UserID, bookID); err == nil {
		canManage = role == "owner" || role == "admin"
	}
	if !canManage {
		http.Redirect(w, r, "/books/"+book.Slug, http.StatusSeeOther)
		return
	}
	links, _ := getShareLinks(db, bookID)
	pd := PageData{
		"Book":  book,
		"Links": links,
		"Now":   time.Now(),
	}.WithUser(s)

	if r.URL.Query().Get("fragment") == "1" {
		tmpl.ExecuteTemplate(w, "share_fragment.html", pd)
		return
	}
	tmpl.ExecuteTemplate(w, "share.html", pd)
}

func createShareLinkHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	book, bookID := resolveBook(r.URL.Path)
	if book == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	canManage := false
	if role, err := getUserBookRole(db, s.UserID, bookID); err == nil {
		canManage = role == "owner" || role == "admin"
	}
	if !canManage {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	permission := r.FormValue("permission")
	if permission != "editor" {
		permission = "viewer"
	}
	expiryChoice := r.FormValue("expiry")
	dur := 24 * time.Hour
	switch expiryChoice {
	case "1h":
		dur = time.Hour
	case "6h":
		dur = 6 * time.Hour
	case "12h":
		dur = 12 * time.Hour
	case "3d":
		dur = 3 * 24 * time.Hour
	case "7d":
		dur = 7 * 24 * time.Hour
	case "30d":
		dur = 30 * 24 * time.Hour
	}

	tok, err := generateToken()
	if err != nil {
		redir := "/books/" + book.Slug + "/share?error=Gagal+membuat+token"
		if r.URL.Query().Get("fragment") == "1" {
			redir += "&fragment=1"
		}
		http.Redirect(w, r, redir, http.StatusSeeOther)
		return
	}
	hash := hashToken(tok)
	expires := time.Now().Add(dur)
	createShareLink(db, bookID, hash, permission, expires, s.UserID)
	insertAuditLog(db, s.UserID, bookID, "share_link", 0, "create_share_link", "", permission, r.RemoteAddr)

	frag := r.URL.Query().Get("fragment") == "1"
	linkURL := fmt.Sprintf("%s/share/%s", appBaseURL, tok)
	if frag {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{
			"url":        linkURL,
			"expires":    expires.Format("02 Jan 2006 15:04"),
			"permission": permission,
		})
		return
	}
	tmpl.ExecuteTemplate(w, "share_created.html", PageData{
		"BookID":     bookID,
		"BookSlug":   book.Slug,
		"Token":      tok,
		"Link":       linkURL,
		"ExpiresAt":  expires,
		"Permission": permission,
	}.WithUser(s))
}

func revokeShareLinkHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	book, bookID := resolveBook(r.URL.Path)
	if book == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	canManage := false
	if role, err := getUserBookRole(db, s.UserID, bookID); err == nil {
		canManage = role == "owner" || role == "admin"
	}
	if !canManage {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	idStr := r.FormValue("link_id")
	id, _ := strconv.Atoi(idStr)
	revokeShareLink(db, id)
	insertAuditLog(db, s.UserID, bookID, "share_link", id, "revoke_share_link", "", "", r.RemoteAddr)
	redir := "/books/" + book.Slug + "/share"
	if r.URL.Query().Get("fragment") == "1" {
		redir += "?fragment=1"
	}
	http.Redirect(w, r, redir, http.StatusSeeOther)
}

// public share accept
func shareAcceptHandler(w http.ResponseWriter, r *http.Request) {
	// path /share/{token}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	tok := parts[1]
	link, err := getShareLinkByHash(db, hashToken(tok))
	if err != nil {
		http.Error(w, "Link tidak valid: "+err.Error(), http.StatusNotFound)
		return
	}
	// debug removed
	now := time.Now()
	if link.RevokedAt != nil || now.After(link.ExpiresAt) {
		http.Error(w, fmt.Sprintf("Link kedaluwarsa atau dicabut (exp=%v now=%v revoked=%v)", link.ExpiresAt, now, link.RevokedAt), http.StatusGone)
		return
	}
	book, err := getBook(db, link.BookID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	s := getSession(r)
	// if logged in, allow accept
	if r.Method == "POST" && s != nil {
		// add membership if not exists (do not upgrade role automatically beyond link permission)
		role := "viewer"
		if link.Permission == "editor" {
			role = "editor"
		}
		// only add if not already member
		if _, err := getUserBookRole(db, s.UserID, book.ID); err != nil {
			addUserToBook(db, book.ID, s.UserID, role)
			insertAuditLog(db, s.UserID, book.ID, "membership", s.UserID, "accept_share", "", role, r.RemoteAddr)
		}
		http.Redirect(w, r, "/books/"+book.Slug, http.StatusSeeOther)
		return
	}

	// guest read-only view
	canEdit := false
	// if logged in member, use their role
	if s != nil {
		role, err := getUserBookRole(db, s.UserID, book.ID)
		if err == nil {
			canEdit = role == "owner" || role == "admin" || role == "editor"
		}
	}
	entries, _ := getEntries(db, book.ID)
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
	debit, credit, _ := getBalance(db, book.ID)
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
		"CanManage":   false,
		"Guest":       s == nil,
		"ShareToken":  tok,
	}.WithUser(s))
}

func voidEntryHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	book, bookID := resolveBook(r.URL.Path)
	if book == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	entryID := getEntryID(r.URL.Path)
	canEdit := false
	if role, err := getUserBookRole(db, s.UserID, bookID); err == nil {
		canEdit = role == "owner" || role == "admin" || role == "editor"
	}
	if !canEdit {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	old, _ := getEntryByID(db, entryID)
	oldJSON, _ := json.Marshal(old)
	voidEntry(db, entryID, s.UserID)
	newEntry, _ := getEntryByID(db, entryID)
	newJSON, _ := json.Marshal(newEntry)
	addRevision(db, entryID, s.UserID, string(oldJSON), string(newJSON))
	insertAuditLog(db, s.UserID, bookID, "entry", entryID, "void_entry", string(oldJSON), string(newJSON), r.RemoteAddr)
	http.Redirect(w, r, "/books/"+book.Slug+"?success=Entri+di-void", http.StatusSeeOther)
}

func leaveBookHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	if s == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	book, bookID := resolveBook(r.URL.Path)
	if book == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// cannot leave if owner
	if book.OwnerID == s.UserID {
		http.Redirect(w, r, "/books/"+book.Slug+"?error=Pemilik+tidak+bisa+keluar", http.StatusSeeOther)
		return
	}
	removeUserFromBook(db, bookID, s.UserID)
	insertAuditLog(db, s.UserID, bookID, "membership", s.UserID, "leave_book", "", "", r.RemoteAddr)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func auditPage(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	book, bookID := resolveBook(r.URL.Path)
	if book == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	canView := s.Role == "super_admin"
	if !canView {
		role, _ := getUserBookRole(db, s.UserID, bookID)
		canView = role == "owner" || role == "admin"
	}
	if !canView {
		http.Redirect(w, r, "/books/"+book.Slug, http.StatusSeeOther)
		return
	}
	logs, _ := getAuditLogs(db, bookID, 200)
	pd := PageData{
		"Book": book,
		"Logs": logs,
	}.WithUser(s)

	if r.URL.Query().Get("fragment") == "1" {
		tmpl.ExecuteTemplate(w, "audit_fragment.html", pd)
		return
	}
	tmpl.ExecuteTemplate(w, "audit.html", pd)
}
