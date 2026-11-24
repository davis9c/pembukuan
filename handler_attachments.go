package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var uploadDir string

func ensureUploadDir(dir string) error {
	uploadDir = dir
	return os.MkdirAll(dir, 0755)
}

func uploadAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	entryID := getEntryID(r.URL.Path)
	if entryID == 0 {
		http.Error(w, "invalid entry", http.StatusBadRequest)
		return
	}

	// check access to the entry's book
	bookID := getBookID(r.URL.Path)
	if !canAccessBook(s, bookID, true) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// limit to 5 files per entry
	count, _ := countAttachments(db, entryID)
	if count >= 5 {
		http.Error(w, "maksimal 5 lampiran per entri", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "file terlalu besar (max 10MB)", http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		http.Error(w, "tidak ada file", http.StatusBadRequest)
		return
	}

	allowedMimes := map[string]bool{
		"image/jpeg":      true,
		"image/png":       true,
		"image/gif":       true,
		"image/webp":      true,
		"application/pdf": true,
	}

	var saved int
	var errors []string
	for _, fh := range files {
		if count+saved >= 5 {
			errors = append(errors, "batas 5 file tercapai")
			break
		}
		if !allowedMimes[fh.Header.Get("Content-Type")] {
			errors = append(errors, fh.Filename+": tipe file tidak didukung")
			continue
		}
		if err := saveAttachment(db, uploadDir, entryID, fh, s.UserID); err != nil {
			errors = append(errors, fh.Filename+": gagal menyimpan")
			continue
		}
		saved++
	}

	msg := fmt.Sprintf("%d file berhasil diupload", saved)
	if len(errors) > 0 {
		msg += ". Gagal: " + strings.Join(errors, "; ")
	}
	http.Redirect(w, r, fmt.Sprintf("/books/%d?success=%s", bookID, msg), http.StatusSeeOther)
}

func saveAttachment(db *sql.DB, dir string, entryID int, fh *multipart.FileHeader, uploadedBy int) error {
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	// generate random name to avoid conflicts
	b := make([]byte, 16)
	rand.Read(b)
	ext := filepath.Ext(fh.Filename)
	storedName := hex.EncodeToString(b) + ext

	dst, err := os.Create(filepath.Join(dir, storedName))
	if err != nil {
		return err
	}
	defer dst.Close()

	written, err := io.Copy(dst, f)
	if err != nil {
		os.Remove(filepath.Join(dir, storedName))
		return err
	}

	mimeType := fh.Header.Get("Content-Type")
	return addAttachment(db, entryID, fh.Filename, storedName, mimeType, written, uploadedBy)
}

func serveAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	if s == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)
	if id == 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	att, err := getAttachmentByID(db, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	entry, err := getEntryByID(db, att.EntryID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !canAccessBook(s, entry.BookID, false) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	path := filepath.Join(uploadDir, att.StoredName)
	w.Header().Set("Content-Type", att.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", att.Filename))
	http.ServeFile(w, r, path)
}

func deleteAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	attID := getAttachmentID(r.URL.Path)
	if attID == 0 {
		http.Error(w, "invalid", http.StatusBadRequest)
		return
	}

	att, err := getAttachmentByID(db, attID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	bookID := getBookID(r.URL.Path)
	if !canAccessBook(s, bookID, true) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// delete file from disk
	os.Remove(filepath.Join(uploadDir, att.StoredName))
	deleteAttachment(db, attID)

	http.Redirect(w, r, fmt.Sprintf("/books/%d?success=Lampiran+dihapus", bookID), http.StatusSeeOther)
}

func canAccessBook(s *Session, bookID int, needEdit bool) bool {
	if s == nil {
		return false
	}
	if s.Role == "super_admin" {
		return true
	}
	role, err := getUserBookRole(db, s.UserID, bookID)
	if err != nil {
		return false
	}
	if !needEdit {
		return true
	}
	return role == "admin" || role == "editor"
}

// path helpers for nested routes like /books/{bookID}/entries/{entryID}/attachments/...
func getEntryID(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// 0=books, 1=bookID, 2=entries, 3=entryID, 4=...
	if len(parts) >= 4 && parts[2] == "entries" {
		id, _ := strconv.Atoi(parts[3])
		return id
	}
	return 0
}

func getAttachmentID(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// 0=books, 1=bookID, 2=entries, 3=entryID, 4=attachments, 5=attachID, 6=delete
	if len(parts) >= 6 && parts[4] == "attachments" {
		id, _ := strconv.Atoi(parts[5])
		return id
	}
	return 0
}
