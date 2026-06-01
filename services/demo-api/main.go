package main

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

//go:embed index.html
var static embed.FS

type Article struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type Config struct {
	Version string `json:"version"`
	Secret  string `json:"secret"`
	Debug   bool   `json:"debug"`
}

var (
	mu       sync.RWMutex
	articles = []Article{
		{ID: 1, Title: "Getting Started", Content: "Welcome to the demo API. Everyone can read this."},
		{ID: 2, Title: "Advanced Topics", Content: "More complex content here. Still readable by all."},
	}
	nextID = 3
)

func hasRole(r *http.Request, roles ...string) bool {
	groups := r.Header.Get("X-authentik-groups")
	for _, group := range strings.Split(groups, "|") {
		group = strings.TrimSpace(group)
		for _, role := range roles {
			if group == role {
				return true
			}
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func forbidden(w http.ResponseWriter) {
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
}

func main() {
	mux := http.NewServeMux()

	// GET /api/articles — reader, writer, admin
	mux.HandleFunc("GET /api/articles", func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "role:reader", "role:writer", "role:admin") {
			forbidden(w)
			return
		}
		mu.RLock()
		defer mu.RUnlock()
		writeJSON(w, http.StatusOK, articles)
	})

	// POST /api/articles — writer, admin
	mux.HandleFunc("POST /api/articles", func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "role:writer", "role:admin") {
			forbidden(w)
			return
		}
		var a Article
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		mu.Lock()
		a.ID = nextID
		nextID++
		articles = append(articles, a)
		mu.Unlock()
		writeJSON(w, http.StatusCreated, a)
	})

	// DELETE /api/articles/{id} — admin
	mux.HandleFunc("DELETE /api/articles/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "role:admin") {
			forbidden(w)
			return
		}
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for i, a := range articles {
			if a.ID == id {
				articles = append(articles[:i], articles[i+1:]...)
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})

	// GET /api/admin/config — admin only
	mux.HandleFunc("GET /api/admin/config", func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "role:admin") {
			forbidden(w)
			return
		}
		writeJSON(w, http.StatusOK, Config{
			Version: "1.0.0",
			Secret:  "super-secret-admin-config",
			Debug:   true,
		})
	})

	// GET /api/me — returns current user info from authentik headers
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		groups := r.Header.Get("X-authentik-groups")
		parsed := []string{}
		for _, g := range strings.Split(groups, "|") {
			g = strings.TrimSpace(g)
			if g != "" {
				parsed = append(parsed, g)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"username": r.Header.Get("X-authentik-username"),
			"name":     r.Header.Get("X-authentik-name"),
			"email":    r.Header.Get("X-authentik-email"),
			"groups":   parsed,
		})
	})

	// Frontend
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		data, _ := static.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
