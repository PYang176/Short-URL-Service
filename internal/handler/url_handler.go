package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"shortURL/internal/model"
	"shortURL/internal/service"
	"strings"
)

type URLHandler struct {
	svc *service.URLService
}

func NewURLHandler(svc *service.URLService) *URLHandler {
	return &URLHandler{svc: svc}
}

func (h *URLHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req model.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 自动补协议头
	if !strings.HasPrefix(req.OriginalURL, "http://") && !strings.HasPrefix(req.OriginalURL, "https://") {
		req.OriginalURL = "http://" + req.OriginalURL
	}

	shortCode, err := h.svc.Create(&req)
	if err != nil {
		log.Printf("[ERROR] create short url failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"short_code": shortCode,
		"full_url":   "http://" + r.Host + "/r/" + shortCode,
		"message":    "Short URL created successfully",
	})
}

func (h *URLHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	log.Printf("[INFO] redirect request for code: %s", code)

	originalURL, err := h.svc.GetAndIncrement(code)
	if err != nil {
		log.Printf("[WARN] short code not found/expired: %s, err: %v", code, err)
		http.Error(w, "Short URL not found or expired", http.StatusNotFound)
		return
	}

	log.Printf("[INFO] redirecting %s -> %s", code, originalURL)
	http.Redirect(w, r, originalURL, http.StatusFound)
}
