-- ownership + archive status on books
ALTER TABLE books ADD COLUMN owner_id INTEGER REFERENCES users(id);
ALTER TABLE books ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE books ADD COLUMN opening_balance INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN currency TEXT NOT NULL DEFAULT 'IDR';

-- backfill owner_id from first book_users admin (best effort)
UPDATE books SET owner_id = (SELECT user_id FROM book_users WHERE book_users.book_id = books.id ORDER BY user_id LIMIT 1) WHERE owner_id IS NULL;

-- book_users extra metadata
ALTER TABLE book_users ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE book_users ADD COLUMN invited_at TEXT DEFAULT '';
ALTER TABLE book_users ADD COLUMN accepted_at TEXT DEFAULT '';

-- entries extra fields
ALTER TABLE entries ADD COLUMN voucher_no TEXT DEFAULT '';
ALTER TABLE entries ADD COLUMN category TEXT DEFAULT '';
ALTER TABLE entries ADD COLUMN party TEXT DEFAULT '';
ALTER TABLE entries ADD COLUMN note TEXT DEFAULT '';
ALTER TABLE entries ADD COLUMN updated_by INTEGER REFERENCES users(id);
ALTER TABLE entries ADD COLUMN updated_at DATETIME;
ALTER TABLE entries ADD COLUMN voided INTEGER NOT NULL DEFAULT 0;

-- attachments uploader
ALTER TABLE attachments ADD COLUMN uploaded_by INTEGER REFERENCES users(id);

-- audit log (append-only)
CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id INTEGER REFERENCES users(id),
    book_id INTEGER REFERENCES books(id),
    entity_type TEXT,
    entity_id INTEGER,
    action TEXT NOT NULL,
    before_json TEXT,
    after_json TEXT,
    ip TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_book ON audit_logs(book_id);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_logs(actor_id);

-- transaction revisions
CREATE TABLE IF NOT EXISTS transaction_revisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entry_id INTEGER NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    actor_id INTEGER REFERENCES users(id),
    before_json TEXT NOT NULL,
    after_json TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_rev_entry ON transaction_revisions(entry_id);

-- share links (token stored as hash)
CREATE TABLE IF NOT EXISTS book_share_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id INTEGER NOT NULL REFERENCES books(id),
    token_hash TEXT NOT NULL UNIQUE,
    permission TEXT NOT NULL DEFAULT 'viewer',
    expires_at DATETIME NOT NULL,
    revoked_at DATETIME,
    created_by INTEGER REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_share_book ON book_share_links(book_id);