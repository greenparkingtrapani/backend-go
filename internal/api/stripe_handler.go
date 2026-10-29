package api

import (
	"encoding/json"
	"estacionamienti/internal/service"
	"io"
	"log"
	"net/http"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

const (
	active          = "active"
	canceled        = "canceled"
	refunded        = "refunded"
	confirmed       = "confirmed"
	statusSucceeded = "succeeded"
)

type StripeWebhookHandler struct {
	StripeSecret       string
	reservationService *service.ReservationService
	senderService      *service.SenderService
}

func NewStripeWebhookHandler(stripeSecret string, reservationService *service.ReservationService, senderService *service.SenderService) *StripeWebhookHandler {
	return &StripeWebhookHandler{
		StripeSecret:       stripeSecret,
		reservationService: reservationService,
		senderService:      senderService,
	}
}

func (h *StripeWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	const maxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	sigHeader := r.Header.Get("Stripe-Signature")
	// IgnoreAPIVersionMismatch: webhook endpoint may use a newer Stripe API
	// version (e.g. dahlia) than stripe-go v82 (basil).
	event, err := webhook.ConstructEventWithOptions(payload, sigHeader, h.StripeSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		log.Printf("Webhook event construction failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Manejar eventos de Stripe Checkout
	switch event.Type {
	case "checkout.session.completed":
  var sess stripe.CheckoutSession
  if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
   http.Error(w,"invalid checkout session",http.StatusBadRequest)
   return
  }
  if sess.ID == "" { http.Error(w,"missing session ID",http.StatusBadRequest); return }
  if string(sess.PaymentStatus) != "paid" {
   // Never confirm a reservation before Stripe has collected payment.
   w.WriteHeader(http.StatusOK)
   return
  }
  reservation, err := h.reservationService.GetReservationBySessionID(sess.ID)
  if err != nil {
   log.Printf("Missing reservation for paid session %s: %v",sess.ID,err)
   w.WriteHeader(http.StatusInternalServerError)
   return
  }
  amount := int64(float64(reservation.TotalPrice)*100 + 0.5)
  if reservation.PaymentMethodID != 2 || sess.Currency != "eur" || sess.AmountTotal != amount {
   log.Printf("Stripe session %s amount, currency or payment mode mismatch",sess.ID)
   w.WriteHeader(http.StatusBadRequest)
   return
  }
  intentID := ""
  if sess.PaymentIntent != nil { intentID = sess.PaymentIntent.ID }
  changed, err := h.reservationService.ActivateOnlineReservation(sess.ID,intentID)
  if err != nil {
   log.Printf("Payment activation failed for %s: %v",sess.ID,err)
   w.WriteHeader(http.StatusInternalServerError)
   return
  }
  if changed {
   statusTranslated := h.senderService.StatusTranslation(confirmed,reservation.Language)
   h.senderService.SendReservationSMS(*reservation,statusTranslated)
   h.senderService.SendReservationEmail(*reservation,statusTranslated)
   h.senderService.SendAdminReservationEmail(*reservation,confirmed)
  }
  
 case "charge.refunded":
		var charge stripe.Charge
		json.Unmarshal(event.Data.Raw, &charge)
		if charge.PaymentIntent != nil && charge.PaymentIntent.ID != "" {
			si, err := h.reservationService.GetSessionIDByPaymentIntentID(charge.PaymentIntent.ID)
			if err != nil {
				log.Printf("No session_id found for PaymentIntent %s: %v", charge.PaymentIntent.ID, err)
				return
			}
			err = h.reservationService.UpdateReservationAndPaymentStatusBySessionID(si, canceled, refunded)
			if err != nil {
				log.Printf("DB error: %v", err)
				return
			}
		}
	default:
		log.Printf("Unhandled event type: %s", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}
func (h *StripeWebhookHandler) GetReservationBySessionIDHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}
	reservation, err := h.reservationService.GetReservationBySessionID(sessionID)
	if err != nil {
		http.Error(w, "Reservation not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reservation)
}
