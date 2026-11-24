package main

import (
	"log"
	"net/http"
	"strconv"

	"golang.org/x/crypto/bcrypt"
)

func requireSuperAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := getSession(r)
		if s == nil || s.Role != "super_admin" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func settingsPage(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	users, _ := getAllUsers(db)
	success := r.URL.Query().Get("success")
	errMsg := r.URL.Query().Get("error")

	gateEnabled := ugClient != nil
	gateBaseURL := getSetting(db, "usergate_base_url")
	gateAPIKey := getSetting(db, "usergate_api_key")
	if gateBaseURL == "" {
		gateBaseURL = "https://usergate.sandalgurun.web.id/api/v1"
	}

	data := PageData{
		"AllUsers":          users,
		"Success":           success,
		"Error":             errMsg,
		"GateEnabled":       gateEnabled,
		"GateBaseURL":       gateBaseURL,
		"GateAPIKey":        gateAPIKey,
		"LocalAdminEnabled": localAdminEnabled,
	}.WithUser(s)

	if ugClient != nil {
		if ugResp, err := ugClient.GetUsers(1, 100, ""); err == nil {
			data["GateUsers"] = ugResp.Data
			data["GateTotal"] = ugResp.Meta.Total
		}
	}

	tmpl.ExecuteTemplate(w, "settings.html", data)
}

func saveGateConfigHandler(w http.ResponseWriter, r *http.Request) {
	enabled := r.FormValue("gate_enabled") == "true"
	baseURL := r.FormValue("gate_base_url")
	apiKey := r.FormValue("gate_api_key")

	setSetting(db, "usergate_enabled", r.FormValue("gate_enabled"))
	setSetting(db, "usergate_base_url", baseURL)
	setSetting(db, "usergate_api_key", apiKey)

	// re-init or clear ugClient
	if enabled && baseURL != "" && apiKey != "" {
		ugClient = NewUserGateClient(baseURL, apiKey)
		log.Printf("usergate re-initialized: %s", baseURL)
	} else {
		ugClient = nil
		log.Printf("usergate disabled")
	}

	http.Redirect(w, r, "/settings?success=Konfigurasi+UserGate+disimpan", http.StatusSeeOther)
}

func createLocalUserHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")
	role := r.FormValue("role")
	if role != "super_admin" {
		role = "regular"
	}
	if username == "" || password == "" {
		http.Redirect(w, r, "/settings?error=Username+dan+password+wajib", http.StatusSeeOther)
		return
	}
	// check duplicate
	if _, err := getUserByUsername(db, username); err == nil {
		http.Redirect(w, r, "/settings?error=Username+sudah+ada", http.StatusSeeOther)
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err := createUser(db, username, string(hash), role); err != nil {
		http.Redirect(w, r, "/settings?error=Gagal+membuat+user", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?success=User+berhasil+dibuat", http.StatusSeeOther)
}

func editUserRoleHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	userID, _ := strconv.Atoi(r.FormValue("user_id"))
	role := r.FormValue("role")
	if role != "super_admin" {
		role = "regular"
	}
	if userID == 0 {
		http.Redirect(w, r, "/settings?error=User+tidak+valid", http.StatusSeeOther)
		return
	}
	// prevent changing own role
	if userID == s.UserID {
		http.Redirect(w, r, "/settings?error=Tidak+bisa+mengubah+role+sendiri", http.StatusSeeOther)
		return
	}
	if err := updateUserRole(db, userID, role); err != nil {
		http.Redirect(w, r, "/settings?error=Gagal+mengubah+role", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?success=Role+berhasil+diubah", http.StatusSeeOther)
}

func deleteLocalUserHandler(w http.ResponseWriter, r *http.Request) {
	s := getSession(r)
	userID, _ := strconv.Atoi(r.FormValue("user_id"))
	if userID == 0 {
		http.Redirect(w, r, "/settings?error=User+tidak+valid", http.StatusSeeOther)
		return
	}
	if userID == s.UserID {
		http.Redirect(w, r, "/settings?error=Tidak+bisa+menghapus+diri+sendiri", http.StatusSeeOther)
		return
	}
	if err := deleteUserByID(db, userID); err != nil {
		http.Redirect(w, r, "/settings?error=Gagal+menghapus+user", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?success=User+berhasil+dihapus", http.StatusSeeOther)
}

func importGateUserHandler(w http.ResponseWriter, r *http.Request) {
	if ugClient == nil {
		http.Redirect(w, r, "/settings?error=UserGate+tidak+aktif", http.StatusSeeOther)
		return
	}
	userID := r.FormValue("gate_user_id")
	if userID == "" {
		http.Redirect(w, r, "/settings?error=ID+user+tidak+valid", http.StatusSeeOther)
		return
	}
	ugUser, err := ugClient.GetUser(userID)
	if err != nil {
		http.Redirect(w, r, "/settings?error=Gagal+mengambil+user+dari+UserGate", http.StatusSeeOther)
		return
	}
	role := "regular"
	if _, err := getUserByUsername(db, ugUser.Username); err == nil {
		http.Redirect(w, r, "/settings?error=Username+sudah+ada+lokal", http.StatusSeeOther)
		return
	}
	if err := createUser(db, ugUser.Username, "", role); err != nil {
		http.Redirect(w, r, "/settings?error=Gagal+menyimpan+user", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?success=User+berhasil+diimport", http.StatusSeeOther)
}

func setGateSuperAdminHandler(w http.ResponseWriter, r *http.Request) {
	if ugClient == nil {
		http.Redirect(w, r, "/settings?error=UserGate+tidak+aktif", http.StatusSeeOther)
		return
	}
	userID := r.FormValue("gate_user_id")
	role := r.FormValue("role")
	if role != "super_admin" {
		role = "regular"
	}
	if userID == "" {
		http.Redirect(w, r, "/settings?error=ID+user+tidak+valid", http.StatusSeeOther)
		return
	}
	ugUser, err := ugClient.GetUser(userID)
	if err != nil {
		http.Redirect(w, r, "/settings?error=Gagal+mengambil+user+dari+UserGate", http.StatusSeeOther)
		return
	}
	// upsert with specified role
	u, err := upsertUserFromGate(db, ugUser.ID, ugUser.Username, role)
	if err != nil {
		http.Redirect(w, r, "/settings?error=Gagal+menyimpan+user", http.StatusSeeOther)
		return
	}
	_ = u
	label := "Super Admin"
	if role != "super_admin" {
		label = "Regular"
	}
	http.Redirect(w, r, "/settings?success=User+"+ugUser.Username+"+diset+sebagai+"+label, http.StatusSeeOther)
}

func syncGateUsersHandler(w http.ResponseWriter, r *http.Request) {
	if ugClient == nil {
		http.Redirect(w, r, "/settings?error=UserGate+tidak+aktif", http.StatusSeeOther)
		return
	}
	// re-login as admin to refresh token
	if err := ugClient.LoginAsAdmin(); err != nil {
		http.Redirect(w, r, "/settings?error=Gagal+login+UserGate:+"+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?success=Token+UserGate+diperbarui", http.StatusSeeOther)
}