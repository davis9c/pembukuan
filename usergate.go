package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type UserGateClient struct {
	BaseURL   string
	APIKey    string
	token     string
	tokenExp  time.Time
	mu        sync.Mutex
}

type UGLoginResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			ID          string   `json:"id"`
			Username    string   `json:"username"`
			Email       string   `json:"email"`
			FullName    string   `json:"full_name"`
			Status      string   `json:"status"`
			Roles       []string `json:"roles"`
			IsSuperAdmin bool    `json:"is_super_admin"`
		} `json:"user"`
	} `json:"data"`
}

type UGUser struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	FullName  string `json:"full_name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type UGUsersResponse struct {
	Status  bool     `json:"status"`
	Message string   `json:"message"`
	Data    []UGUser `json:"data"`
	Meta    struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	} `json:"meta"`
}

func NewUserGateClient(baseURL, apiKey string) *UserGateClient {
	return &UserGateClient{BaseURL: baseURL, APIKey: apiKey}
}

func (c *UserGateClient) getToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	// need to login with superadmin creds
	// we store token from admin login at startup or re-login
	return "", fmt.Errorf("no valid token, login required")
}

func (c *UserGateClient) setToken(token string, expiresIn int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
	c.tokenExp = time.Now().Add(time.Duration(expiresIn) * time.Second)
}

func (c *UserGateClient) Login(username, password string) (*UGLoginResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	req, _ := http.NewRequest("POST", c.BaseURL+"/auth/login", bytes.NewReader(body))
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usergate request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result UGLoginResponse
	json.Unmarshal(data, &result)

	if !result.Status {
		return nil, fmt.Errorf("usergate: %s", result.Message)
	}

	// cache token for subsequent API calls
	c.setToken(result.Data.AccessToken, result.Data.ExpiresIn)
	return &result, nil
}

func (c *UserGateClient) GetUsers(page, perPage int, search string) (*UGUsersResponse, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/users?page=%d&per_page=%d", c.BaseURL, page, perPage)
	if search != "" {
		url += "&search=" + search
	}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usergate request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result UGUsersResponse
	json.Unmarshal(data, &result)
	return &result, nil
}

func (c *UserGateClient) GetUser(userID string) (*UGUser, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequest("GET", c.BaseURL+"/users/"+userID, nil)
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usergate request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    UGUser `json:"data"`
	}
	json.Unmarshal(data, &result)
	if !result.Status {
		return nil, fmt.Errorf("usergate: %s", result.Message)
	}
	return &result.Data, nil
}

func (c *UserGateClient) LoginAsAdmin() error {
	// use admin credentials from env to get a token for API calls
	resp, err := c.Login(adminUser, adminPass)
	if err != nil {
		return err
	}
	c.setToken(resp.Data.AccessToken, resp.Data.ExpiresIn)
	return nil
}