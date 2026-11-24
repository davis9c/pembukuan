package main

import "time"

type User struct {
	ID       int
	Username string
	Role     string
	Active   bool
}

type Book struct {
	ID             int
	Slug           string
	Name           string
	Description    string
	OwnerID        int
	OwnerName      string
	Status         string
	OpeningBalance int64
	Currency       string
	CreatedAt      time.Time
}

type Entry struct {
	ID             int
	BookID         int
	VoucherNo      string
	Date           string
	Description    string
	Type           string
	Amount         int64
	Category       string
	Party          string
	Note           string
	CreatedBy      int
	CreatedAt      time.Time
	UpdatedBy      int
	UpdatedAt      time.Time
	Voided         bool
	RunningBalance int64
}

type BookUser struct {
	UserID     int
	Username   string
	BookID     int
	Role       string
	Status     string
	InvitedAt  time.Time
	AcceptedAt time.Time
}

type Attachment struct {
	ID         int
	EntryID    int
	Filename   string
	StoredName string
	MimeType   string
	Size       int64
	UploadedBy int
	CreatedAt  time.Time
}

type AuditLog struct {
	ID         int
	ActorID    int
	BookID     int
	EntityType string
	EntityID   int
	Action     string
	BeforeJSON string
	AfterJSON  string
	IP         string
	CreatedAt  time.Time
}

type ShareLink struct {
	ID         int
	BookID     int
	TokenHash  string
	Permission string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedBy  int
	CreatedAt  time.Time
}
