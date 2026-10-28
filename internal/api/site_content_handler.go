package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// SiteContentHandler serves public localized copy and stores admin-only edits.
// The database schema is installed separately through the migration.
type SiteContentHandler struct{ DB *sql.DB }

var siteSections = map[string]bool{
	"Header": true, "HeroSection": true, "AboutSection": true,
	"PricingSection": true, "ContactSection": true, "Footer": true, "SEO": true,
}

func validSiteContentScope(language, section string) bool {
	return (language == "it" || language == "en" || language == "es") && siteSections[section]
}

func (h *SiteContentHandler) Read(w http.ResponseWriter, r *http.Request) {
	language, section := r.URL.Query().Get("language"), r.URL.Query().Get("section")
	if !validSiteContentScope(language, section) {
		http.Error(w, "Invalid language or section", http.StatusBadRequest)
		return
	}
	rows, err := h.DB.QueryContext(r.Context(), `SELECT field_key, value FROM public.site_content WHERE language=$1 AND section=$2`, language, section)
	if err != nil {
		http.Error(w, "Could not read site content", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	fields := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			http.Error(w, "Could not read site content", http.StatusInternalServerError)
			return
		}
		fields[key] = value
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Could not read site content", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Fields map[string]string `json:"fields"`
	}{fields})
}

type siteContentSave struct {
	Language string            `json:"language"`
	Section  string            `json:"section"`
	Fields   map[string]string `json:"fields"`
}

func validateSiteContentSave(payload siteContentSave) error {
	if !validSiteContentScope(payload.Language, payload.Section) || len(payload.Fields) == 0 || len(payload.Fields) > 200 {
		return errors.New("invalid language, section, or field count")
	}
	prefix := payload.Section + "."
	for key, value := range payload.Fields {
		if !strings.HasPrefix(key, prefix) || len(key) > 160 || len(key) <= len(prefix) || len(value) > 20000 || strings.ContainsAny(key, "\x00\r\n") || strings.ContainsRune(value, 0) {
			return errors.New("invalid content field")
		}
		for _, part := range strings.Split(key, ".") {
			if part == "" || part == "__proto__" || part == "constructor" || part == "prototype" {
				return errors.New("invalid content path")
			}
		}
	}
	return nil
}

func (h *SiteContentHandler) Save(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var payload siteContentSave
	if err := decoder.Decode(&payload); err != nil || validateSiteContentSave(payload) != nil {
		http.Error(w, "Invalid site content", http.StatusBadRequest)
		return
	}
	var trailing interface{}
	if decoder.Decode(&trailing) != io.EOF {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Could not save site content", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	for key, value := range payload.Fields {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO public.site_content (language, section, field_key, value)
          VALUES ($1,$2,$3,$4) ON CONFLICT (language, section, field_key)
          DO UPDATE SET value=EXCLUDED.value, updated_at=NOW()`, payload.Language, payload.Section, key, value)
		if err != nil {
			http.Error(w, "Could not save site content", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Could not save site content", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"saved": len(payload.Fields)})
}
