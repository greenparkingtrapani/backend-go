package service

import (
	"estacionamienti/internal/db"
	"estacionamienti/internal/entities"
	"estacionamienti/internal/repository"
	"fmt"
	"log"
	"time"

	"database/sql"
 "github.com/stripe/stripe-go/v82"
 "github.com/stripe/stripe-go/v82/checkout/session"
)

type AdminService struct {
	adminRepo       *repository.AdminRepository
	reservationRepo *repository.ReservationRepository
	stripeService   *StripeService
	senderService   *SenderService
}

func NewAdminService(adminRepo *repository.AdminRepository, reservationRepo *repository.ReservationRepository, stripeService *StripeService, senderService *SenderService) *AdminService {
	return &AdminService{adminRepo: adminRepo,
		stripeService:   stripeService,
		reservationRepo: reservationRepo,
		senderService:   senderService}
}

func (s *AdminService) ListReservations(startTime, endTime, code, vehicleType, status, limit, offset, sortBy, sortOrder, view string) (entities.ReservationsList, error) {
	reservationList, err := s.adminRepo.ListReservationsWithFilters(startTime, endTime, code, vehicleType, status, limit, offset, sortBy, sortOrder, view)
	if err != nil {
		log.Printf("Error listing reservations: %v", err)
		return entities.ReservationsList{}, err
	}
	return reservationList, nil
}

func (s *AdminService) CreateReservation(reservationReq *entities.ReservationRequest) (reservationResponse *entities.ReservationResponse, err error) {
	code := fmt.Sprintf("%08X", time.Now().UnixNano()%100000000)

	reservation := &db.Reservation{
		Code:            code,
		UserName:        reservationReq.UserName,
		UserEmail:       reservationReq.UserEmail,
		UserPhone:       sql.NullString{String: reservationReq.UserPhone, Valid: reservationReq.UserPhone != ""},
		VehicleTypeID:   reservationReq.VehicleTypeID,
		VehiclePlate:    sql.NullString{String: reservationReq.VehiclePlate, Valid: reservationReq.VehiclePlate != ""},
		VehicleModel:    sql.NullString{String: reservationReq.VehicleModel, Valid: reservationReq.VehicleModel != ""},
		PaymentMethodID: reservationReq.PaymentMethodID,
		Status:          statusActive,
		TotalPrice:      sql.NullFloat64{Float64: float64(reservationReq.TotalPrice), Valid: reservationReq.TotalPrice != 0},
		StartTime:       reservationReq.StartTime,
		EndTime:         reservationReq.EndTime,
		Language:        reservationReq.Language,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	err = s.reservationRepo.CreateReservation(reservation)
	if err != nil {
		log.Printf("Error creating reservation in repository: %v", err)
		return nil, err
	}

	reservationResponse, err = s.adminRepo.FindReservationByCode(code)
	if err != nil {
		log.Printf("Error getting reservation from repository: %v", err)
		return nil, err
	}
	statusTraducido := s.senderService.StatusTranslation(statusActive, reservation.Language)
	s.senderService.SendReservationSMS(*reservationResponse, statusTraducido)
	s.senderService.SendReservationEmail(*reservationResponse, statusTraducido)

	return reservationResponse, nil
}

func (s *AdminService) CancelReservation(code string, refund bool) error {
	reservation, err := s.reservationRepo.GetReservationByCodeOnly(code)
	if err != nil {
		log.Printf("Error canceling reservation: %v", err)
		return err
	}
	sessionID := reservation.StripeSessionID
	// Si la session de stripe no está, se puede cancelar (Quiere decir que nunca hubo pago por stripe)
	if sessionID.String == "" {
		_, err = s.reservationRepo.CancelReservation(code)
		if err != nil {
			log.Printf("Error canceling reservation: %v", err)
			return err
		}
		s.notifyCustomerCancellation(code)
		return nil
	}
	if refund {
		err = s.stripeService.RefundPaymentBySessionID(sessionID.String)
		if err != nil {
			log.Printf("Error refunding payment: %v", err)
			return err
		}
	}
	_, err = s.reservationRepo.CancelReservation(code)
	if err != nil {
		log.Printf("Error canceling reservation: %v", err)
		return err
	}
	s.notifyCustomerCancellation(code)
	return nil
}

func (s *AdminService) notifyCustomerCancellation(code string) {
	reservationResponse, err := s.adminRepo.FindReservationByCode(code)
	if err != nil {
		log.Printf("Error getting reservation %s for cancellation email: %v", code, err)
		return
	}
	statusTraducido := s.senderService.StatusTranslation(statusCancel, reservationResponse.Language)
	s.senderService.SendReservationEmail(*reservationResponse, statusTraducido)
}

func (s *AdminService) ListVehicleSpaces() ([]db.VehicleSpaceWithPrices, error) {
	spaces, err := s.adminRepo.ListVehicleSpaces()
	if err != nil {
		log.Printf("Error listing vehicle spaces: %v", err)
		return nil, err
	}
	return spaces, nil
}

func (s *AdminService) UpdateVehicleSpacesAndPrices(vehicleType string, spaces int, prices map[string]float32) error {
	err := s.adminRepo.UpdateVehicleSpaces(vehicleType, spaces)
	if err != nil {
		log.Printf("[AdminService] Error updating spaces for vehicleType '%s': %v", vehicleType, err)
		return err
	}
	for timeName, price := range prices {
		err := s.adminRepo.UpdateVehiclePrice(vehicleType, timeName, price)
		if err != nil {
			log.Printf("[AdminService] Error updating price for vehicleType '%s', time '%s': %v", vehicleType, timeName, err)
			return err
		}
	}
	return nil
}

// GREENPARKING_DELETE_V1: no Stripe refund, no status mutation; protected purge only.
func (s *AdminService) PurgeReservation(code string) (bool, error) {
	// Look up any Stripe Checkout session before deleting. Never trust the local
 // pending flag alone: webhooks can arrive later.
 var checkoutID sql.NullString
 err := s.adminRepo.DB.QueryRow("SELECT stripe_session_id FROM reservations WHERE code = $1", code).Scan(&checkoutID)
 if err != nil { return false, err }
 if checkoutID.Valid && checkoutID.String != "" {
   checkout, err := session.Get(checkoutID.String, nil)
   if err != nil { return false, fmt.Errorf("cannot verify Stripe payment before deletion: %w", err) }
   if checkout.PaymentStatus != stripe.CheckoutSessionPaymentStatusUnpaid {
      return false, nil
   }
   if checkout.Status == stripe.CheckoutSessionStatusOpen {
      // An open Checkout must be expired before deletion to prevent a
      // customer from paying a reservation that no longer exists.
      if _, err := session.Expire(checkoutID.String, nil); err != nil {
         return false, fmt.Errorf("cannot expire Stripe checkout before deletion: %w", err)
      }
   } else if checkout.Status != stripe.CheckoutSessionStatusExpired {
      return false, nil
   }
 }
 return s.adminRepo.PurgeUnpaidClosedReservation(code)
}
