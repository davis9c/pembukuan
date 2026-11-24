package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
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
	err := db.QueryRow("SELECT id, username, role, active FROM users WHERE username = ?", username).Scan(&u.ID, &u.Username, &u.Role, &u.Active)
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
	rows, err := db.Query("SELECT id, username, role, active FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		var active int
		rows.Scan(&u.ID, &u.Username, &u.Role, &active)
		u.Active = active == 1
		users = append(users, u)
	}
	return users, nil
}

func upsertUserFromGate(db *sql.DB, ugID, username, role string) (*User, error) {
	u, err := getUserByUsername(db, username)
	if err != nil {
		_, err := db.Exec("INSERT INTO users (username, password_hash, role, active) VALUES (?, ?, ?, 1)", username, "", role)
		if err != nil {
			return nil, err
		}
		return getUserByUsername(db, username)
	}
	if u.Role != role && role != "" {
		db.Exec("UPDATE users SET role = ? WHERE id = ?", role, u.ID)
		u.Role = role
	}
	return u, nil
}

func toggleUserActive(db *sql.DB, userID int) error {
	_, err := db.Exec("UPDATE users SET active = CASE WHEN active = 1 THEN 0 ELSE 1 END WHERE id = ?", userID)
	return err
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

func createBook(db *sql.DB, name, desc string, ownerID int, openingBalance int64) (int64, error) {
	slug := generateSlug()
	// ensure uniktif there is a collision (very rare)
	for {
		var exists int
		db.QueryRow("SELECT COUNT(*) FROM books WHERE slug = ?", slug).Scan(&exists)
		if exists == 0 {
			break
		}
		slug = generateSlug()
	}
	r, err := db.Exec("INSERT INTO books (name, description, owner_id, opening_balance, slug) VALUES (?, ?, ?, ?, ?)", name, desc, ownerID, openingBalance, slug)
	if err != nil {
		return 0, err
	}
	id, _ := r.LastInsertId()
	addUserToBook(db, int(id), ownerID, "owner")
	return id, nil
}

func generateSlug() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b) // 12 chars
}

func getBooksForUser(db *sql.DB, userID int, isSuperAdmin bool) ([]Book, error) {
	var rows *sql.Rows
	var err error
	if isSuperAdmin {
		rows, err = db.Query("SELECT id, slug, name, description, owner_id, status, opening_balance, currency, created_at FROM books ORDER BY name")
	} else {
		rows, err = db.Query(`
			SELECT b.id, b.slug, b.name, b.description, b.owner_id, b.status, b.opening_balance, b.currency, b.created_at
			FROM books b
			JOIN book_users bu ON b.id = bu.book_id
			WHERE bu.user_id = ? AND bu.status = 'active' AND b.status = 'active'
			ORDER BY b.name`, userID)
	}
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
		var slug sql.NullString
		var owner sql.NullInt64
		var status sql.NullString
		var currency sql.NullString
		rows.Scan(&b.ID, &slug, &b.Name, &b.Description, &owner, &status, &b.OpeningBalance, &currency, &b.CreatedAt)
		if slug.Valid {
			b.Slug = slug.String
		}
		if owner.Valid {
			b.OwnerID = int(owner.Int64)
		}
		if status.Valid && status.String != "" {
			b.Status = status.String
		} else {
			b.Status = "active"
		}
		if currency.Valid && currency.String != "" {
			b.Currency = currency.String
		} else {
			b.Currency = "IDR"
		}
		books = append(books, b)
	}
	return books, nil
}

func getBook(db *sql.DB, bookID int) (*Book, error) {
	b := &Book{}
	var slug sql.NullString
	var owner sql.NullInt64
	var status sql.NullString
	var currency sql.NullString
	err := db.QueryRow("SELECT id, slug, name, description, owner_id, status, opening_balance, currency, created_at FROM books WHERE id = ?", bookID).
		Scan(&b.ID, &slug, &b.Name, &b.Description, &owner, &status, &b.OpeningBalance, &currency, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	if slug.Valid {
		b.Slug = slug.String
	}
	if owner.Valid {
		b.OwnerID = int(owner.Int64)
	}
	if status.Valid && status.String != "" {
		b.Status = status.String
	} else {
		b.Status = "active"
	}
	if currency.Valid && currency.String != "" {
		b.Currency = currency.String
	} else {
		b.Currency = "IDR"
	}
	return b, nil
}

func getBookBySlug(db *sql.DB, slug string) (*Book, error) {
	var id int
	err := db.QueryRow("SELECT id FROM books WHERE slug = ?", slug).Scan(&id)
	if err != nil {
		return nil, err
	}
	return getBook(db, id)
}

// archive instead of delete
func deleteBook(db *sql.DB, bookID int) error {
	_, err := db.Exec("UPDATE books SET status = 'archived' WHERE id = ?", bookID)
	return err
}

// ---------- book_user queries ----------

func addUserToBook(db *sql.DB, bookID, userID int, role string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO book_users (book_id, user_id, role, status) VALUES (?, ?, ?, 'active')", bookID, userID, role)
	return err
}

func removeUserFromBook(db *sql.DB, bookID, userID int) error {
	_, err := db.Exec("DELETE FROM book_users WHERE book_id = ? AND user_id = ?", bookID, userID)
	return err
}

func getUserBookRole(db *sql.DB, userID, bookID int) (string, error) {
	var role string
	var status string
	err := db.QueryRow("SELECT role, status FROM book_users WHERE book_id = ? AND user_id = ?", bookID, userID).Scan(&role, &status)
	if err != nil {
		return "", err
	}
	if status != "active" {
		return "", sql.ErrNoRows
	}
	return role, nil
}

func getBookUsers(db *sql.DB, bookID int) ([]BookUser, error) {
	rows, err := db.Query(`
		SELECT u.id, u.username, bu.role, bu.status, bu.invited_at, bu.accepted_at
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
		var invited, accepted sql.NullString
		var status sql.NullString
		rows.Scan(&bu.UserID, &bu.Username, &bu.Role, &status, &invited, &accepted)
		if status.Valid {
			bu.Status = status.String
		} else {
			bu.Status = "active"
		}
		if invited.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05Z", invited.String)
			// try sqlite default format
			if t.IsZero() {
				t, _ = time.Parse("2006-01-02 15:04:05", invited.String)
			}
			bu.InvitedAt = t
		}
		if accepted.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05Z", accepted.String)
			if t.IsZero() {
				t, _ = time.Parse("2006-01-02 15:04:05", accepted.String)
			}
			bu.AcceptedAt = t
		}
		bus = append(bus, bu)
	}
	return bus, nil
}

// ---------- entry queries ----------

func addEntry(db *sql.DB, bookID, userID int, date, voucherNo, desc, etype, category, party, note string, amount int64) (int64, error) {
	r, err := db.Exec(`INSERT INTO entries (book_id, user_id, date, voucher_no, description, type, amount, category, party, note, created_by, updated_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		bookID, userID, date, voucherNo, desc, etype, amount, category, party, note, userID, userID, nowStr())
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func updateEntry(db *sql.DB, entryID int, actorID int, date, voucherNo, desc, etype, category, party, note string, amount int64) error {
	_, err := db.Exec(`UPDATE entries SET date=?, voucher_no=?, description=?, type=?, amount=?, category=?, party=?, note=?, updated_by=?, updated_at=? WHERE id=?`,
		date, voucherNo, desc, etype, amount, category, party, note, actorID, nowStr(), entryID)
	return err
}

func getEntries(db *sql.DB, bookID int) ([]Entry, error) {
	rows, err := db.Query(`SELECT id, book_id, date, voucher_no, description, type, amount, category, party, note, user_id, created_at, updated_by, updated_at, voided
		FROM entries WHERE book_id = ? ORDER BY date ASC, id ASC`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var e Entry
		var voucher, category, party, note sql.NullString
		var createdBy sql.NullInt64
		var updatedBy sql.NullInt64
		var updatedAt sql.NullString
		var voided int
		var createdAt sql.NullString
		rows.Scan(&e.ID, &e.BookID, &e.Date, &voucher, &e.Description, &e.Type, &e.Amount, &category, &party, &note, &createdBy, &createdAt, &updatedBy, &updatedAt, &voided)
		if voucher.Valid {
			e.VoucherNo = voucher.String
		}
		if category.Valid {
			e.Category = category.String
		}
		if party.Valid {
			e.Party = party.String
		}
		if note.Valid {
			e.Note = note.String
		}
		if createdBy.Valid {
			e.CreatedBy = int(createdBy.Int64)
		}
		if updatedBy.Valid {
			e.UpdatedBy = int(updatedBy.Int64)
		}
		if createdAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", createdAt.String)
			if t.IsZero() {
				t, _ = time.Parse("2006-01-02T15:04", createdAt.String)
			}
			e.CreatedAt = t
		}
		if updatedAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", updatedAt.String)
			if t.IsZero() {
				t, _ = time.Parse("2006-01-02T15:04", updatedAt.String)
			}
			e.UpdatedAt = t
		}
		e.Voided = voided == 1
		entries = append(entries, e)
	}
	return entries, nil
}

func getBalance(db *sql.DB, bookID int) (debit int64, credit int64, err error) {
	err = db.QueryRow("SELECT COALESCE(SUM(CASE WHEN type='debit' AND voided=0 THEN amount ELSE 0 END),0), COALESCE(SUM(CASE WHEN type='credit' AND voided=0 THEN amount ELSE 0 END),0) FROM entries WHERE book_id = ?", bookID).
		Scan(&debit, &credit)
	return
}

func getEntryByID(db *sql.DB, entryID int) (*Entry, error) {
	rows, err := db.Query(`SELECT id, book_id, date, voucher_no, description, type, amount, category, party, note, user_id, created_at, updated_by, updated_at, voided
		FROM entries WHERE id = ?`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	var e Entry
	var voucher, category, party, note sql.NullString
	var createdBy sql.NullInt64
	var updatedBy sql.NullInt64
	var updatedAt sql.NullString
	var voided int
	var createdAt sql.NullString
	rows.Scan(&e.ID, &e.BookID, &e.Date, &voucher, &e.Description, &e.Type, &e.Amount, &category, &party, &note, &createdBy, &createdAt, &updatedBy, &updatedAt, &voided)
	if voucher.Valid {
		e.VoucherNo = voucher.String
	}
	if category.Valid {
		e.Category = category.String
	}
	if party.Valid {
		e.Party = party.String
	}
	if note.Valid {
		e.Note = note.String
	}
	if createdBy.Valid {
		e.CreatedBy = int(createdBy.Int64)
	}
	if updatedBy.Valid {
		e.UpdatedBy = int(updatedBy.Int64)
	}
	e.Voided = voided == 1
	return &e, nil
}

func deleteEntry(db *sql.DB, entryID int) error {
	_, err := db.Exec("DELETE FROM entries WHERE id = ?", entryID)
	return err
}

func voidEntry(db *sql.DB, entryID int, actorID int) error {
	_, err := db.Exec("UPDATE entries SET voided=1, updated_by=?, updated_at=? WHERE id=?", actorID, nowStr(), entryID)
	return err
}

// ---------- attachment queries ----------

func addAttachment(db *sql.DB, entryID int, filename, storedName, mimeType string, size int64, uploadedBy int) error {
	_, err := db.Exec("INSERT INTO attachments (entry_id, filename, stored_name, mime_type, size, uploaded_by) VALUES (?, ?, ?, ?, ?, ?)",
		entryID, filename, storedName, mimeType, size, uploadedBy)
	return err
}

func getAttachments(db *sql.DB, entryID int) ([]Attachment, error) {
	rows, err := db.Query("SELECT id, entry_id, filename, stored_name, mime_type, size, uploaded_by, created_at FROM attachments WHERE entry_id = ? ORDER BY id", entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var atts []Attachment
	for rows.Next() {
		var a Attachment
		var uploadedBy sql.NullInt64
		var createdAt sql.NullString
		rows.Scan(&a.ID, &a.EntryID, &a.Filename, &a.StoredName, &a.MimeType, &a.Size, &uploadedBy, &createdAt)
		if uploadedBy.Valid {
			a.UploadedBy = int(uploadedBy.Int64)
		}
		if createdAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", createdAt.String)
			if t.IsZero() {
				t, _ = time.Parse("2006-01-02T15:04", createdAt.String)
			}
			a.CreatedAt = t
		}
		atts = append(atts, a)
	}
	return atts, nil
}

func getAttachmentByID(db *sql.DB, id int) (*Attachment, error) {
	a := &Attachment{}
	var uploadedBy sql.NullInt64
	var createdAt sql.NullString
	err := db.QueryRow("SELECT id, entry_id, filename, stored_name, mime_type, size, uploaded_by, created_at FROM attachments WHERE id = ?", id).
		Scan(&a.ID, &a.EntryID, &a.Filename, &a.StoredName, &a.MimeType, &a.Size, &uploadedBy, &createdAt)
	if err != nil {
		return nil, err
	}
	if uploadedBy.Valid {
		a.UploadedBy = int(uploadedBy.Int64)
	}
	if createdAt.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", createdAt.String)
		if t.IsZero() {
			t, _ = time.Parse("2006-01-02T15:04", createdAt.String)
		}
		a.CreatedAt = t
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

// ---------- audit queries ----------

func insertAuditLog(db *sql.DB, actorID, bookID int, entityType string, entityID int, action, beforeJSON, afterJSON, ip string) error {
	_, err := db.Exec(`INSERT INTO audit_logs (actor_id, book_id, entity_type, entity_id, action, before_json, after_json, ip) VALUES (?,?,?,?,?,?,?,?)`,
		actorID, bookID, entityType, entityID, action, beforeJSON, afterJSON, ip)
	return err
}

func getAuditLogs(db *sql.DB, bookID int, limit int) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(`SELECT id, actor_id, book_id, entity_type, entity_id, action, before_json, after_json, ip, created_at
		FROM audit_logs WHERE book_id = ? ORDER BY id DESC LIMIT ?`, bookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		var actor, book interface{}
		var et, before, after, ip interface{}
		var createdAt interface{}
		rows.Scan(&l.ID, &actor, &book, &et, &l.EntityID, &l.Action, &before, &after, &ip, &createdAt)
		if actor != nil {
			switch v := actor.(type) {
			case int64:
				l.ActorID = int(v)
			case int:
				l.ActorID = v
			}
		}
		if book != nil {
			switch v := book.(type) {
			case int64:
				l.BookID = int(v)
			case int:
				l.BookID = v
			}
		}
		if et != nil {
			l.EntityType = fmt.Sprint(et)
		}
		if before != nil {
			l.BeforeJSON = fmt.Sprint(before)
		}
		if after != nil {
			l.AfterJSON = fmt.Sprint(after)
		}
		if ip != nil {
			l.IP = fmt.Sprint(ip)
		}
		if createdAt != nil {
			switch v := createdAt.(type) {
			case time.Time:
				l.CreatedAt = v
			case string:
				t, _ := time.Parse("2006-01-02 15:04:05", v)
				if t.IsZero() {
					t, _ = time.Parse(time.RFC3339, v)
				}
				l.CreatedAt = t
			}
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// ---------- revision queries ----------

func addRevision(db *sql.DB, entryID, actorID int, beforeJSON, afterJSON string) error {
	_, err := db.Exec("INSERT INTO transaction_revisions (entry_id, actor_id, before_json, after_json) VALUES (?,?,?,?)",
		entryID, actorID, beforeJSON, afterJSON)
	return err
}

func getRevisions(db *sql.DB, entryID int) ([]map[string]interface{}, error) {
	rows, err := db.Query("SELECT id, actor_id, before_json, after_json, created_at FROM transaction_revisions WHERE entry_id = ? ORDER BY id DESC", entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []map[string]interface{}
	for rows.Next() {
		var id, actorID int
		var before, after, createdAt string
		rows.Scan(&id, &actorID, &before, &after, &createdAt)
		res = append(res, map[string]interface{}{
			"id":         id,
			"actor_id":   actorID,
			"before":     before,
			"after":      after,
			"created_at": createdAt,
		})
	}
	return res, nil
}

// ---------- share link queries ----------

func createShareLink(db *sql.DB, bookID int, tokenHash, permission string, expiresAt time.Time, createdBy int) error {
	_, err := db.Exec("INSERT INTO book_share_links (book_id, token_hash, permission, expires_at, created_by) VALUES (?,?,?,?,?)",
		bookID, tokenHash, permission, expiresAt.Format("2006-01-02 15:04:05"), createdBy)
	return err
}

func getShareLinks(db *sql.DB, bookID int) ([]ShareLink, error) {
	rows, err := db.Query("SELECT id, book_id, token_hash, permission, expires_at, revoked_at, created_by, created_at FROM book_share_links WHERE book_id = ? ORDER BY id DESC", bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []ShareLink
	for rows.Next() {
		var l ShareLink
		var revoked, expires, created interface{}
		rows.Scan(&l.ID, &l.BookID, &l.TokenHash, &l.Permission, &expires, &revoked, &l.CreatedBy, &created)
		switch v := expires.(type) {
		case time.Time:
			l.ExpiresAt = v
		case string:
			t, _ := time.Parse("2006-01-02 15:04:05", v)
			if t.IsZero() {
				t, _ = time.Parse(time.RFC3339, v)
			}
			l.ExpiresAt = t
		}
		if revoked != nil {
			switch v := revoked.(type) {
			case time.Time:
				t := v
				l.RevokedAt = &t
			case string:
				t, _ := time.Parse("2006-01-02 15:04:05", v)
				if t.IsZero() {
					t, _ = time.Parse(time.RFC3339, v)
				}
				if !t.IsZero() {
					l.RevokedAt = &t
				}
			}
		}
		switch v := created.(type) {
		case time.Time:
			l.CreatedAt = v
		case string:
			t, _ := time.Parse("2006-01-02 15:04:05", v)
			if t.IsZero() {
				t, _ = time.Parse(time.RFC3339, v)
			}
			l.CreatedAt = t
		}
		links = append(links, l)
	}
	return links, nil
}

func getShareLinkByHash(db *sql.DB, tokenHash string) (*ShareLink, error) {
	l := &ShareLink{}
	var revoked interface{}
	var expires, created interface{}
	err := db.QueryRow("SELECT id, book_id, token_hash, permission, expires_at, revoked_at, created_by, created_at FROM book_share_links WHERE token_hash = ?", tokenHash).
		Scan(&l.ID, &l.BookID, &l.TokenHash, &l.Permission, &expires, &revoked, &l.CreatedBy, &created)
	if err != nil {
		return nil, err
	}
	// expires may come back as time.Time or string depending on driver
	switch v := expires.(type) {
	case time.Time:
		l.ExpiresAt = v
	case string:
		t, _ := time.Parse("2006-01-02 15:04:05", v)
		if t.IsZero() {
			t, _ = time.Parse(time.RFC3339, v)
		}
		l.ExpiresAt = t
	}
	if revoked != nil {
		switch v := revoked.(type) {
		case time.Time:
			t := v
			l.RevokedAt = &t
		case string:
			t, _ := time.Parse("2006-01-02 15:04:05", v)
			if t.IsZero() {
				t, _ = time.Parse(time.RFC3339, v)
			}
			if !t.IsZero() {
				l.RevokedAt = &t
			}
		}
	}
	if created != nil {
		switch v := created.(type) {
		case time.Time:
			l.CreatedAt = v
		case string:
			t, _ := time.Parse("2006-01-02 15:04:05", v)
			if t.IsZero() {
				t, _ = time.Parse(time.RFC3339, v)
			}
			l.CreatedAt = t
		}
	}
	return l, nil
}

func revokeShareLink(db *sql.DB, id int) error {
	_, err := db.Exec("UPDATE book_share_links SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	return err
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
