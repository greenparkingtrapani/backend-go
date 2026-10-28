package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

type LuggageHandler struct{ DB *sql.DB }

type LuggagePiece struct {
	WeightKg float64 `json:"weight_kg"`
	LengthCm float64 `json:"length_cm"`
	WidthCm  float64 `json:"width_cm"`
	HeightCm float64 `json:"height_cm"`
}

type LuggageRequest struct {
	Name      string         `json:"name"`
	Email     string         `json:"email"`
	Phone     string         `json:"phone"`
	Origin    string         `json:"origin"`
	DropoffAt time.Time      `json:"dropoff_at"`
	PickupAt  time.Time      `json:"pickup_at"`
	Bags      []LuggagePiece `json:"bags"`
	Notes     string         `json:"notes"`
}

func luggageQuote(r LuggageRequest) (int, int, error) {
	if r.PickupAt.IsZero() || r.DropoffAt.IsZero() || !r.PickupAt.After(r.DropoffAt) || r.PickupAt.Sub(r.DropoffAt) > 30*24*time.Hour {
		return 0, 0, errors.New("invalid time range")
	}
	if len(r.Bags) == 0 || len(r.Bags) > 10 {
		return 0, 0, errors.New("provide 1 to 10 bags")
	}
	hours := int(math.Ceil(r.PickupAt.Sub(r.DropoffAt).Hours()))
	if hours < 1 {
		hours = 1
	}
	rate := 0
	for _, b := range r.Bags {
		if math.IsNaN(b.WeightKg) || math.IsInf(b.WeightKg, 0) || b.WeightKg <= 0 || b.WeightKg > 100 || b.LengthCm <= 0 || b.WidthCm <= 0 || b.HeightCm <= 0 || b.LengthCm > 300 || b.WidthCm > 300 || b.HeightCm > 300 {
			return 0, 0, errors.New("invalid bag measurements")
		}
		switch {
		case b.WeightKg <= 10:
			rate += 100
		case b.WeightKg <= 20:
			rate += 200
		default:
			rate += 300
		}
	}
	return hours, rate * hours, nil
}

func (h *LuggageHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	var req LuggageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Origin = strings.TrimSpace(req.Origin)
	req.Notes = strings.TrimSpace(req.Notes)
	if len(req.Name) < 2 || len(req.Name) > 120 || len(req.Phone) < 5 || len(req.Phone) > 40 || len(req.Origin) > 120 || len(req.Notes) > 1000 {
		http.Error(w, "Invalid contact details", 400)
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil || len(req.Email) > 254 {
		http.Error(w, "Invalid email", 400)
		return
	}
	if req.DropoffAt.Before(time.Now().Add(-time.Hour)) {
		http.Error(w, "Dropoff time is in the past", 400)
		return
	}
	hours, cents, err := luggageQuote(req)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	bagJSON, _ := json.Marshal(req.Bags)
	random := make([]byte, 8)
	if _, err = rand.Read(random); err != nil {
		http.Error(w, "Cannot create request", 500)
		return
	}
	code := "BG-" + strings.ToUpper(hex.EncodeToString(random))
	_, err = h.DB.ExecContext(r.Context(), `INSERT INTO luggage_requests (code,customer_name,email,phone,origin,dropoff_at,pickup_at,bags,notes,quoted_cents,quoted_hours) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, code, req.Name, req.Email, req.Phone, req.Origin, req.DropoffAt, req.PickupAt, bagJSON, req.Notes, cents, hours)
	if err != nil {
		http.Error(w, "Unable to save request", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"code": code, "status": "pending", "hours": hours, "quoted_total_eur": float64(cents) / 100, "payment": "on_site", "message": "Request received; subject to staff confirmation"})
}

func (h *LuggageHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `SELECT code,customer_name,email,phone,origin,dropoff_at,pickup_at,bags,notes,status,quoted_cents,quoted_hours,paid_cents,payment_method,created_at,collected_at FROM luggage_requests WHERE status <> 'deleted' ORDER BY dropoff_at DESC LIMIT 200`)
	if err != nil {
		http.Error(w, "Database error", 500)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var code, name, email, phone, origin, notes, status string
		var drop, pick, created time.Time
		var bags json.RawMessage
		var cents, hours, paid int
		var method sql.NullString
		var collected sql.NullTime
		if err := rows.Scan(&code, &name, &email, &phone, &origin, &drop, &pick, &bags, &notes, &status, &cents, &hours, &paid, &method, &created, &collected); err != nil {
			http.Error(w, "Database error", 500)
			return
		}
		items = append(items, map[string]any{"code": code, "name": name, "email": email, "phone": phone, "origin": origin, "dropoff_at": drop, "pickup_at": pick, "bags": bags, "notes": notes, "status": status, "quoted_total_eur": float64(cents) / 100, "hours": hours, "paid_eur": float64(paid) / 100, "payment_method": method.String, "created_at": created, "collected_at": collected.Time})
	}
	if rows.Err() != nil {
		http.Error(w, "Database error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (h *LuggageHandler) Update(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/admin/luggage/")
	if !strings.HasPrefix(code, "BG-") || len(code) != 19 {
		http.Error(w, "Invalid code", 400)
		return
	}
	var payload struct {
		Status        string `json:"status"`
		PaymentCents  int    `json:"payment_cents"`
		PaymentMethod string `json:"payment_method"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	if payload.Status != "confirmed" && payload.Status != "received" && payload.Status != "collected" && payload.Status != "cancelled" && payload.Status != "deleted" && payload.Status != "pending" {
		http.Error(w, "Invalid status", 400)
		return
	}
	if payload.PaymentCents < 0 || payload.PaymentCents > 1000000 || len(payload.PaymentMethod) > 40 {
		http.Error(w, "Invalid payment", 400)
		return
	}
	result, err := h.DB.ExecContext(r.Context(), `UPDATE luggage_requests SET
	  quoted_cents=CASE WHEN $2='collected' AND collected_at IS NULL THEN (quoted_cents/quoted_hours)*GREATEST(1,CEIL(EXTRACT(EPOCH FROM (NOW()-dropoff_at))/3600)::INT) ELSE quoted_cents END,
	  quoted_hours=CASE WHEN $2='collected' AND collected_at IS NULL THEN GREATEST(1,CEIL(EXTRACT(EPOCH FROM (NOW()-dropoff_at))/3600)::INT) ELSE quoted_hours END,
	  status=$2,paid_cents=paid_cents+$3,payment_method=CASE WHEN $4='' THEN payment_method ELSE $4 END,
	  collected_at=CASE WHEN $2='collected' THEN COALESCE(collected_at,NOW()) ELSE collected_at END
	  WHERE code=$1`, code, payload.Status, payload.PaymentCents, payload.PaymentMethod)
	if err != nil {
		http.Error(w, "Database error", 500)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		http.Error(w, "Not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}
