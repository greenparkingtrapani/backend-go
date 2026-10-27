package api

import (
	"encoding/json"
	"estacionamienti/internal/entities"
	"estacionamienti/internal/errors"
	"estacionamienti/internal/service"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type AdminHandler struct {
	adminService *service.AdminService
}

func NewAdminHandler(svc *service.AdminService) *AdminHandler {
	return &AdminHandler{adminService: svc}
}

func (h *AdminHandler) ListReservations(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	startTimeStr := r.URL.Query().Get("start_time")
	endTimeStr := r.URL.Query().Get("end_time")
	vehicleType := r.URL.Query().Get("vehicle_type_name")
	status := r.URL.Query().Get("status")
	limit := r.URL.Query().Get("limit")
	offset := r.URL.Query().Get("offset")
	sortBy := r.URL.Query().Get("sort_by")
	sortOrder := r.URL.Query().Get("sort_order")
	view := r.URL.Query().Get("view")

	reservations, err := h.adminService.ListReservations(startTimeStr, endTimeStr, code, vehicleType, status, limit, offset, sortBy, sortOrder, view)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(reservations)
}

func (h *AdminHandler) CreateReservation(w http.ResponseWriter, r *http.Request) {
	var req entities.ReservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	req.StartTime = req.StartTime.UTC()
	req.EndTime = req.EndTime.UTC()
	reservation, err := h.adminService.CreateReservation(&req)
	if err != nil {
		if herr, ok := err.(*errors.HTTPError); ok {
			http.Error(w, herr.Message, herr.Code)
			return
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reservation": reservation,
	})
}

func (h *AdminHandler) AdminDeleteReservation(w http.ResponseWriter, r *http.Request) {
	code := mux.Vars(r)["code"]
	refund := r.URL.Query().Get("refund")
	refundBool, err := strconv.ParseBool(refund)
	if err != nil {
		http.Error(w, "Invalid refund query param", http.StatusBadRequest)
	}
	err = h.adminService.CancelReservation(code, refundBool)
	if err != nil {
		http.Error(w, "Could not delete reservation", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "Reservation canceled with code: " + code})
}

func (h *AdminHandler) ListVehicleSpaces(w http.ResponseWriter, r *http.Request) {
	spaces, err := h.adminService.ListVehicleSpaces()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(spaces)
}

func (h *AdminHandler) UpdateVehicleSpaces(w http.ResponseWriter, r *http.Request) {
	vehicleType := mux.Vars(r)["vehicle_type"]
	var req struct {
		Spaces int                `json:"spaces"`
		Prices map[string]float32 `json:"prices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	err := h.adminService.UpdateVehicleSpacesAndPrices(vehicleType, req.Spaces, req.Prices)
	if err != nil {
		http.Error(w, "Could not update vehicle spaces/prices", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "Vehicle spaces and prices updated"})
}

// GREENPARKING_DELETE_V1: admin-only physical deletion, requiring a matching confirmation code.
func (h *AdminHandler) PurgeReservation(w http.ResponseWriter, r *http.Request) {
	code := mux.Vars(r)["code"]
	var payload struct {
		ConfirmCode string `json:"confirm_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.ConfirmCode != code || code == "" {
		http.Error(w, "Confirmation code does not match", http.StatusBadRequest)
		return
	}
	deleted, err := h.adminService.PurgeReservation(code)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if !deleted {
		http.Error(w, "Only canceled or finished unpaid reservations can be permanently deleted", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Reservation permanently deleted"})
}
