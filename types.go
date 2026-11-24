package main

import "time"

type User struct {
	ID       int
	Username string
	Role     string
}

type Book struct {
	ID          int
	Name        string
	Description string
	CreatedAt   time.Time
}

type Entry struct {
	ID          int
	BookID      int
	Date        string
	Description string
	Type        string
	Amount      int64
	CreatedAt   time.Time
}

type BookUser struct {
	UserID   int
	Username string
	BookID   int
	Role     string
}

type Attachment struct {
	ID         int
	EntryID    int
	Filename   string
	StoredName string
	MimeType   string
	Size       int64
	CreatedAt  time.Time
}