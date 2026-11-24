package main

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

var (
	sessions   = map[string]Session{}
	sessionsMu sync.Mutex
)

type Session struct {
	UserID   int
	Username string
	Role     string
	Expires  time.Time
}

func setSession(w http.ResponseWriter, s Session) {
	b := make([]byte, 32)
	rand.Read(b)
	token := base64.URLEncoding.EncodeToString(b)
	sessionsMu.Lock()
	sessions[token] = s
	sessionsMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Expires:  s.Expires,
	})
}

func getSession(r *http.Request) *Session {
	c, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	sessionsMu.Lock()
	s, ok := sessions[c.Value]
	sessionsMu.Unlock()
	if !ok || time.Now().After(s.Expires) {
		return nil
	}
	return &s
}

func clearSession(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("session")
	if c != nil {
		sessionsMu.Lock()
		delete(sessions, c.Value)
		sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
}
