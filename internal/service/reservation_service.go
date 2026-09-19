package service

import (
	"database/sql"
	"estacionamienti/internal/db"
	"estacionamienti/internal/entities"
	"estacionamienti/internal/errors"
	"estacionamienti/internal/repository"
	"fmt"
	"log"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
)

const (
	statusActive  = "active"
	statusPending = "pending"
	statusCancel  = "canceled"
	deposit       = 0.3

	reservationTimeHour  = 1
	reservationTimeDaily = 2

	discountThresholdDays = 5
	discountRate          = 0.10
)

type ReservationService struct {
	stripeService *StripeService
	Repo          *repository.ReservationRepository
	senderService *SenderService
}

func NewReservationService(repo *repository.ReservationRepository, stripeService *StripeService, senderService *SenderService) *ReservationService {
	return &ReservationService{Repo: repo,
		stripeService: stripeService,
		senderService: senderService}
}

func (s *ReservationService) GetPrices() ([]entities.PriceResponse, error) {
	priceResponse, err := s.Repo.GetPrices()
	if err != nil {
		log.Printf("Error from GetPrices: %v", err)
		return nil, err
	}
	return priceResponse, nil
}

func (s *ReservationService) GetVehicleTypes() ([]db.VehicleType, error) {
	vehicleTypes, err := s.Repo.GetVehicleTypes()
	if err != nil {
		log.Printf("Error from GetVehicleTypes: %v", err)
		return nil, err
	}
	return vehicleTypes, nil
}

func (s *ReservationService) CheckAvailability(req entities.ReservationRequest) (*entities.AvailabilityResponse, error) {
	// You need vehicle type name for mapping
	var vehicleTypeName string
	vehicleTypes, err := s.Repo.GetVehicleTypes()
	if err != nil {
		log.Printf("Error from GetVehicleTypes: %v", err)
		return nil, fmt.Errorf("internal error checking availability: %w", err)
	}

	for _, vt := range vehicleTypes {
		if vt.ID == req.VehicleTypeID {
			vehicleTypeName = vt.Name
			break
		}
	}

	hourlyDetails, err := s.Repo.GetHourlyAvailabilityDetails(req.StartTime, req.EndTime, req.VehicleTypeID, vehicleTypeName)
	if err != nil {
		log.Printf("Error from GetHourlyAvailabilityDetails: %v", err)
		return nil, fmt.Errorf("internal error checking availability: %w", err)
	}

	response := &entities.AvailabilityResponse{
		RequestedStartTime: req.StartTime,
		RequestedEndTime:   req.EndTime,
		IsOverallAvailable: true,
	}

	if len(hourlyDetails) == 0 {
		response.IsOverallAvailable = false
		return response, nil
	}

	var firstUnavailableTime *time.Time

	for _, detail := range hourlyDetails {
		availableInSlot := detail.TotalSpaces - detail.BookedSpaces
		isSlotAvailable := availableInSlot > 0

		response.SlotDetails = append(response.SlotDetails, entities.TimeSlotAvailability{
			StartTime:       detail.SlotStart,
			EndTime:         detail.SlotEnd,
			IsAvailable:     isSlotAvailable,
			AvailableSpaces: availableInSlot,
		})

		if !isSlotAvailable {
			response.IsOverallAvailable = false
			if firstUnavailableTime == nil {
				tempTime := detail.SlotStart
				firstUnavailableTime = &tempTime
			}
		}
	}

	return response, nil
}

func (s *ReservationService) GetTotalPriceForReservation(vehicleTypeID int, startTime, endTime time.Time) (*entities.TotalPriceResponse, error) {
	if !endTime.After(startTime) {
		return nil, fmt.Errorf("end_time must be after start_time")
	}
	days, hours := getUnitCounts(startTime, endTime)

	priceHour, err := s.Repo.GetPriceForUnit(vehicleTypeID, reservationTimeHour)
	if err != nil {
		log.Printf("Error from GetPriceForUnit (hour): %v", err)
		return nil, fmt.Errorf("could not get price per hour: %w", err)
	}
	priceDay, err := s.Repo.GetPriceForUnit(vehicleTypeID, reservationTimeDaily)
	if err != nil {
		log.Printf("Error from GetPriceForUnit (day): %v", err)
		return nil, fmt.Errorf("could not get price per day: %w", err)
	}

	result := float32(days)*priceDay + float32(hours)*priceHour
	discount := days >= discountThresholdDays
	if discount {
		result = result * (1 - discountRate)
	}
	result = float32(int(result*10)) / 10

	return &entities.TotalPriceResponse{
		TotalPrice: result,
		Discount:   discount,
	}, nil
}

func (s *ReservationService) CreateReservation(req *entities.ReservationRequest) (*entities.StripeSessionResponse, error) {
	code := fmt.Sprintf("%08X", time.Now().UnixNano()%100000000)

	reservation := &db.Reservation{
		Code:            code,
		UserName:        req.UserName,
		UserEmail:       req.UserEmail,
		UserPhone:       sql.NullString{String: req.UserPhone, Valid: req.UserPhone != ""},
		VehicleTypeID:   req.VehicleTypeID,
		VehiclePlate:    sql.NullString{String: req.VehiclePlate, Valid: req.VehiclePlate != ""},
		VehicleModel:    sql.NullString{String: req.VehicleModel, Valid: req.VehicleModel != ""},
		PaymentMethodID: req.PaymentMethodID,
		Status:          statusPending,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		Language:        req.Language,
		TotalPrice:      sql.NullFloat64{Float64: float64(req.TotalPrice), Valid: req.TotalPrice != 0},
		DepositPayment:  sql.NullFloat64{Float64: float64(req.DepositPayment), Valid: req.DepositPayment != 0},
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	sessionURL, err := s.handlePaymentIntent(req, reservation)
	if err != nil {
		log.Printf("Error from handlePaymentIntent: %v", err)
		return nil, err
	}

	err = s.Repo.CreateReservation(reservation)
	if err != nil {
		log.Printf("Error creating reservation in repository: %v", err)
		return nil, err
	}

	return &entities.StripeSessionResponse{
		Code:      code,
		URL:       sessionURL,
		SessionID: reservation.StripeSessionID.String}, nil
}

func (s *ReservationService) GetReservationByCode(code, email string) (*entities.ReservationResponse, error) {
	reservationResponse, err := s.Repo.GetReservationByCode(code, email)
	if err != nil {
		log.Printf("Error getting reservation by code: %v", err)
		return nil, err
	}
	return reservationResponse, nil
}

func (s *ReservationService) CancelReservation(code string) error {
	reservation, err := s.Repo.GetReservationByCodeOnly(code)
	if err != nil {
		return err
	}
	log.Printf("Canceling reservation with code: %s", code)

	currentTime := time.Now().UTC()
	if reservation.StartTime.Sub(currentTime) < 12*time.Hour {
		log.Printf("Reservation can only be cancelled more than 12 hours before the start time")
		return errors.ErrUnauthorized("Reservations can only be cancelled more than 12 hours before the start time")
	}

	sessionID := reservation.StripeSessionID.String
	if sessionID == "" {
		_, err = s.Repo.CancelReservation(code)
		return err
	}

	// If reservation has a Stripe session ID
	reservationResp, err := s.GetReservationBySessionID(sessionID)
	if err != nil {
		log.Printf("Error getting reservation by Stripe session ID: %v", err)
		return err
	}

	err = s.stripeService.RefundPaymentBySessionID(reservation.StripeSessionID.String)
	if err != nil {
		log.Printf("Error refunding payment: %v", err)
		return err
	}

	_, err = s.Repo.CancelReservation(code)
	if err != nil {
		log.Printf("Error canceling reservation: %v", err)
		return err
	}

	statusTraducido := s.senderService.StatusTranslation(statusCancel, reservationResp.Language)
	s.senderService.SendReservationSMS(*reservationResp, statusTraducido)
	s.senderService.SendReservationEmail(*reservationResp, statusTraducido)
	return nil
}

func (s *ReservationService) GetReservationBySessionID(sessionID string) (*entities.ReservationResponse, error) {
	reservation, err := s.Repo.GetReservationByStripeSessionID(sessionID)
	if err != nil {
		log.Printf("Error getting reservation by Stripe session ID: %v", err)
		return nil, err
	}
	resp := &entities.ReservationResponse{
		Code:            reservation.Code,
		UserName:        reservation.UserName,
		UserEmail:       reservation.UserEmail,
		UserPhone:       reservation.UserPhone.String,
		VehicleTypeID:   reservation.VehicleTypeID,
		VehiclePlate:    reservation.VehiclePlate.String,
		VehicleModel:    reservation.VehicleModel.String,
		PaymentMethodID: reservation.PaymentMethodID,
		Status:          reservation.Status,
		StartTime:       reservation.StartTime,
		EndTime:         reservation.EndTime,
		CreatedAt:       reservation.CreatedAt,
		UpdatedAt:       reservation.UpdatedAt,
		PaymentStatus:   reservation.PaymentStatus.String,
		Language:        reservation.Language,
		TotalPrice:      float32(reservation.TotalPrice.Float64),
		DepositPayment:  float32(reservation.DepositPayment.Float64),
	}
	return resp, nil
}

func (s *ReservationService) UpdateReservationAndPaymentStatusBySessionID(sessionID, reservationStatus, paymentStatus string) error {
	reservation, err := s.Repo.GetReservationByStripeSessionID(sessionID)
	if err != nil {
		return err
	}
	return s.Repo.UpdateReservationAndPaymentStatus(reservation.ID, reservationStatus, paymentStatus)
}

func (s *ReservationService) UpdateReservationStatusPaymentAndIntentBySessionID(sessionID, reservationStatus, paymentStatus, paymentIntentID string) error {
	reservation, err := s.Repo.GetReservationByStripeSessionID(sessionID)
	if err != nil {
		return err
	}
	return s.Repo.UpdateReservationStatusPaymentAndIntent(reservation.ID, reservationStatus, paymentStatus, paymentIntentID)
}

// GetSessionIDByPaymentIntentID busca el session_id en Stripe a partir de un PaymentIntentID
func (s *ReservationService) GetSessionIDByPaymentIntentID(paymentIntentID string) (string, error) {
	params := &stripe.CheckoutSessionListParams{
		PaymentIntent: &paymentIntentID,
	}
	params.Limit = stripe.Int64(1)
	it := session.List(params)
	for it.Next() {
		sess := it.CheckoutSession()
		if sess != nil && sess.ID != "" {
			return sess.ID, nil
		}
	}
	return "", fmt.Errorf("No session_id found for PaymentIntentID %s", paymentIntentID)
}

func (s *ReservationService) handlePaymentIntent(req *entities.ReservationRequest, reservation *db.Reservation) (string, error) {
	var amount int64
	if req.PaymentMethodID == 2 { // online
		amount = int64(req.TotalPrice * 100)
	} else if req.PaymentMethodID == 1 { // onsite
		amount = int64(float64(req.TotalPrice) * deposit * 100)
	} else {
		return "", fmt.Errorf("Método de pago no soportado")
	}

	sessionURL, sessionID, err := s.stripeService.CreateCheckoutSession(amount, "eur", req.UserEmail, reservation.Language)
	if err != nil {
		log.Printf("Error creating Stripe checkout session: %v", err)
		return "", err
	}

	reservation.StripeSessionID = sql.NullString{String: sessionID, Valid: true}
	reservation.PaymentStatus = sql.NullString{String: statusPending, Valid: true}

	return sessionURL, nil
}

func getUnitCounts(startTime, endTime time.Time) (days, hours int) {
	d := endTime.Sub(startTime)
	if d <= 0 {
		return 0, 0
	}

	dayDur := 24 * time.Hour

	days = int(d / dayDur)
	d -= time.Duration(days) * dayDur

	if d%time.Hour == 0 {
		hours = int(d / time.Hour)
	} else {
		hours = int(d/time.Hour) + 1 // redondeo hacia arriba
	}

	if hours >= 24 {
		days += hours / 24
		hours = hours % 24
	}
	return
}
