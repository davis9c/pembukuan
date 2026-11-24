package main

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

func initDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	return runMigrations(db)
}

// ---------- user queries ----------

func createUser(db *sql.DB, username, hash, role string) error {
	_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)", username, hash, role)
	return err
}

func getUserByUsername(db *sql.DB, username string) (*User, error) {
	u := &User{}
	err := db.QueryRow("SELECT id, username, role FROM users WHERE username = ?", username).Scan(&u.ID, &u.Username, &u.Role)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func getUserPasswordHash(db *sql.DB, username string) (string, error) {
	var h string
	err := db.QueryRow("SELECT password_hash FROM users WHERE username = ?", username).Scan(&h)
	return h, err
}

func getAllUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query("SELECT id, username, role FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Username, &u.Role)
		users = append(users, u)
	}
	return users, nil
}

func upsertUserFromGate(db *sql.DB, ugID, username, role string) (*User, error) {
	// insert or update user from usergate (no password stored)
	u, err := getUserByUsername(db, username)
	if err != nil {
		_, err := db.Exec("INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)", username, "", role)
		if err != nil {
			return nil, err
		}
		return getUserByUsername(db, username)
	}
	// update role if changed
	if u.Role != role && role != "" {
		db.Exec("UPDATE users SET role = ? WHERE id = ?", role, u.ID)
		u.Role = role
	}
	return u, nil
}

func updateUserRole(db *sql.DB, userID int, role string) error {
	_, err := db.Exec("UPDATE users SET role = ? WHERE id = ?", role, userID)
	return err
}

func updateUserPassword(db *sql.DB, userID int, hash string) error {
	_, err := db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, userID)
	return err
}

func deleteUserByID(db *sql.DB, userID int) error {
	db.Exec("DELETE FROM book_users WHERE user_id = ?", userID)
	_, err := db.Exec("DELETE FROM users WHERE id = ?", userID)
	return err
}

// ---------- book queries ----------

func createBook(db *sql.DB, name, desc string) (int64, error) {
	r, err := db.Exec("INSERT INTO books (name, description) VALUES (?, ?)", name, desc)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func getBooksForUser(db *sql.DB, userID int, isSuperAdmin bool) ([]Book, error) {
	if isSuperAdmin {
		rows, err := db.Query("SELECT id, name, description, created_at FROM books ORDER BY name")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanBooks(rows)
	}
	rows, err := db.Query(`
		SELECT b.id, b.name, b.description, b.created_at
		FROM books b
		JOIN book_users bu ON b.id = bu.book_id
		WHERE bu.user_id = ?
		ORDER BY b.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBooks(rows)
}

func scanBooks(rows *sql.Rows) ([]Book, error) {
	var books []Book
	for rows.Next() {
		var b Book
		rows.Scan(&b.ID, &b.Name, &b.Description, &b.CreatedAt)
		books = append(books, b)
	}
	return books, nil
}

func getBook(db *sql.DB, bookID int) (*Book, error) {
	b := &Book{}
	err := db.QueryRow("SELECT id, name, description, created_at FROM books WHERE id = ?", bookID).
		Scan(&b.ID, &b.Name, &b.Description, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func deleteBook(db *sql.DB, bookID int) error {
	db.Exec("DELETE FROM entries WHERE book_id = ?", bookID)
	db.Exec("DELETE FROM book_users WHERE book_id = ?", bookID)
	_, err := db.Exec("DELETE FROM books WHERE id = ?", bookID)
	return err
}

// ---------- book_user queries ----------

func addUserToBook(db *sql.DB, bookID, userID int, role string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO book_users (book_id, user_id, role) VALUES (?, ?, ?)", bookID, userID, role)
	return err
}

func removeUserFromBook(db *sql.DB, bookID, userID int) error {
	_, err := db.Exec("DELETE FROM book_users WHERE book_id = ? AND user_id = ?", bookID, userID)
	return err
}

func getUserBookRole(db *sql.DB, userID, bookID int) (string, error) {
	var role string
	err := db.QueryRow("SELECT role FROM book_users WHERE book_id = ? AND user_id = ?", bookID, userID).Scan(&role)
	return role, err
}

func getBookUsers(db *sql.DB, bookID int) ([]BookUser, error) {
	rows, err := db.Query(`
		SELECT u.id, u.username, bu.role
		FROM book_users bu
		JOIN users u ON u.id = bu.user_id
		WHERE bu.book_id = ?
		ORDER BY u.username`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bus []BookUser
	for rows.Next() {
		var bu BookUser
		bu.BookID = bookID
		rows.Scan(&bu.UserID, &bu.Username, &bu.Role)
		bus = append(bus, bu)
	}
	return bus, nil
}

// ---------- entry queries ----------

func addEntry(db *sql.DB, bookID, userID int, date, desc, etype string, amount int64) (int64, error) {
	r, err := db.Exec("INSERT INTO entries (book_id, user_id, date, description, type, amount) VALUES (?, ?, ?, ?, ?, ?)",
		bookID, userID, date, desc, etype, amount)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func getEntries(db *sql.DB, bookID int) ([]Entry, error) {
	rows, err := db.Query("SELECT id, book_id, date, description, type, amount, created_at FROM entries WHERE book_id = ? ORDER BY date DESC, id DESC", bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var e Entry
		rows.Scan(&e.ID, &e.BookID, &e.Date, &e.Description, &e.Type, &e.Amount, &e.CreatedAt)
		entries = append(entries, e)
	}
	return entries, nil
}

func getBalance(db *sql.DB, bookID int) (debit int64, credit int64, err error) {
	err = db.QueryRow("SELECT COALESCE(SUM(CASE WHEN type='debit' THEN amount ELSE 0 END),0), COALESCE(SUM(CASE WHEN type='credit' THEN amount ELSE 0 END),0) FROM entries WHERE book_id = ?", bookID).
		Scan(&debit, &credit)
	return
}

func deleteEntry(db *sql.DB, entryID int) error {
	_, err := db.Exec("DELETE FROM entries WHERE id = ?", entryID)
	return err
}

// ---------- attachment queries ----------

func addAttachment(db *sql.DB, entryID int, filename, storedName, mimeType string, size int64) error {
	_, err := db.Exec("INSERT INTO attachments (entry_id, filename, stored_name, mime_type, size) VALUES (?, ?, ?, ?, ?)",
		entryID, filename, storedName, mimeType, size)
	return err
}

func getAttachments(db *sql.DB, entryID int) ([]Attachment, error) {
	rows, err := db.Query("SELECT id, entry_id, filename, stored_name, mime_type, size, created_at FROM attachments WHERE entry_id = ? ORDER BY id", entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var atts []Attachment
	for rows.Next() {
		var a Attachment
		rows.Scan(&a.ID, &a.EntryID, &a.Filename, &a.StoredName, &a.MimeType, &a.Size, &a.CreatedAt)
		atts = append(atts, a)
	}
	return atts, nil
}

func getAttachmentByID(db *sql.DB, id int) (*Attachment, error) {
	a := &Attachment{}
	err := db.QueryRow("SELECT id, entry_id, filename, stored_name, mime_type, size, created_at FROM attachments WHERE id = ?", id).
		Scan(&a.ID, &a.EntryID, &a.Filename, &a.StoredName, &a.MimeType, &a.Size, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func deleteAttachment(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM attachments WHERE id = ?", id)
	return err
}

func countAttachments(db *sql.DB, entryID int) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM attachments WHERE entry_id = ?", entryID).Scan(&n)
	return n, err
}

// ---------- settings queries ----------

func getSetting(db *sql.DB, key string) string {
	var v string
	db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	return v
}

func setSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value)
	return err
}

func nowStr() string {
	return time.Now().Format("2006-01-02T15:04")
}
