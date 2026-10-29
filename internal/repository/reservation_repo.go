package repository

import (
	"database/sql"
	"errors"
	"estacionamienti/internal/db"
	"estacionamienti/internal/entities"
	"fmt"
	"time"
)

type SlotOccupationInfo struct {
	SlotStart    time.Time
	SlotEnd      time.Time
	TotalSpaces  int
	BookedSpaces int
}

type ReservationRepository struct {
	DB *sql.DB
}

func NewReservationRepository(db *sql.DB) *ReservationRepository {
	return &ReservationRepository{DB: db}
}

func (r *ReservationRepository) GetPrices() ([]entities.PriceResponse, error) {
	query := `
	SELECT vt.name as vehicle_type, rt.name as reservation_time, vp.price
	FROM vehicle_prices vp
	JOIN vehicle_types vt ON vp.vehicle_type_id = vt.id
	JOIN reservation_times rt ON vp.reservation_time_id = rt.id
	ORDER BY vt.name, rt.name
	`

	rows, err := r.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prices []entities.PriceResponse
	for rows.Next() {
		var p entities.PriceResponse
		if err := rows.Scan(&p.VehicleType, &p.ReservationTime, &p.Price); err != nil {
			return nil, err
		}
		prices = append(prices, p)
	}

	return prices, nil
}

func (r *ReservationRepository) GetVehicleTypes() ([]db.VehicleType, error) {
	query := `SELECT id, name FROM vehicle_types ORDER BY name`

	rows, err := r.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []db.VehicleType
	for rows.Next() {
		var vt db.VehicleType
		if err := rows.Scan(&vt.ID, &vt.Name); err != nil {
			return nil, err
		}
		types = append(types, vt)
	}

	return types, nil
}

func (r *ReservationRepository) GetHourlyAvailabilityDetails(startTime, endTime time.Time, vehicleTypeID int, vehicleTypeName string) ([]SlotOccupationInfo, error) {
	if !endTime.After(startTime) {
		return nil, fmt.Errorf("end time must be after start time")
	}
	if vehicleTypeName != "car" && vehicleTypeName != "sedan" && vehicleTypeName != "suv" && vehicleTypeName != "motorcycle" && vehicleTypeName != "electric_bicycle" {
		return nil, fmt.Errorf("unknown vehicle type %d", vehicleTypeID)
	}

	// Count active cars and motorcycles in the same physical space for each hour.
	// Capacity: three COMPLETE motorcycles consume one car bay; 1-2 consume none.
	query := `
		WITH requested_slots AS (
			SELECT gs.slot_hour_start, gs.slot_hour_start + interval '1 hour' AS slot_hour_end
			FROM generate_series(
				date_trunc('hour', $1::timestamptz),
				$2::timestamptz - interval '1 microsecond',
				interval '1 hour'
			) AS gs(slot_hour_start)
		),
		capacities AS (
			SELECT
				COALESCE(MAX(vs.spaces) FILTER (WHERE vt.name IN ('car', 'sedan', 'suv')), 0) AS car_spaces,
				COALESCE(MAX(vs.spaces) FILTER (WHERE vt.name IN ('motorcycle', 'electric_bicycle')), 0) AS moto_spaces
			FROM vehicle_spaces vs
			JOIN vehicle_types vt ON vt.id = vs.vehicle_type_id
		),
		occupancy AS (
			SELECT rs.slot_hour_start, rs.slot_hour_end,
				COUNT(r.id) FILTER (WHERE vt.name IN ('car', 'sedan', 'suv'))::int AS cars,
				COUNT(r.id) FILTER (WHERE vt.name IN ('motorcycle', 'electric_bicycle'))::int AS motos
			FROM requested_slots rs
			LEFT JOIN reservations r ON (r.status = 'active' OR (r.status = 'pending' AND r.payment_method_id = 2 AND r.created_at > NOW() - interval '45 minutes'))
				AND r.start_time < rs.slot_hour_end AND r.end_time > rs.slot_hour_start
			LEFT JOIN vehicle_types vt ON vt.id = r.vehicle_type_id
			GROUP BY rs.slot_hour_start, rs.slot_hour_end
		)
		, manager_occupancy AS (
		 SELECT rs.slot_hour_start,
		 COUNT(m.local_stay_id) FILTER (WHERE m.vehicle_type IN ('car', 'sedan', 'suv'))::int AS cars,
		 COUNT(m.local_stay_id) FILTER (WHERE m.vehicle_type IN ('motorcycle','electric_bicycle'))::int AS motos
		 FROM requested_slots rs
		 LEFT JOIN public.manager_active_stays m
		 ON (m.arrival_local AT TIME ZONE 'Europe/Rome') < rs.slot_hour_end
		 AND (m.booking_code='' OR NOT EXISTS (
		 SELECT 1 FROM reservations booked
		 WHERE booked.code=m.booking_code AND booked.status IN ('active','pending')
		 AND booked.start_time < rs.slot_hour_end AND booked.end_time > rs.slot_hour_start
		 ))
		 GROUP BY rs.slot_hour_start
		)
		SELECT o.slot_hour_start, o.slot_hour_end,
			c.car_spaces, c.moto_spaces, o.cars+m.cars, o.motos+m.motos
		FROM occupancy o CROSS JOIN capacities c
		JOIN manager_occupancy m ON m.slot_hour_start=o.slot_hour_start
		ORDER BY o.slot_hour_start
	`
	rows, err := r.DB.Query(query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("error querying hourly availability: %w", err)
	}
	defer rows.Close()

	var results []SlotOccupationInfo
	for rows.Next() {
		var slot SlotOccupationInfo
		var carSpaces, motoSpaces, cars, motos int
		if err := rows.Scan(&slot.SlotStart, &slot.SlotEnd, &carSpaces, &motoSpaces, &cars, &motos); err != nil {
			return nil, fmt.Errorf("error scanning hourly availability slot: %w", err)
		}
		slot.TotalSpaces, slot.BookedSpaces = effectiveSpaces(vehicleTypeName, carSpaces, motoSpaces, cars, motos)
		results = append(results, slot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error after iterating hourly availability rows: %w", err)
	}
	return results, nil
}

// Only each complete group of three motorcycles removes one car space.
// For a new motorcycle, the current motorcycle count must still be below the
// effective maximum, which accounts for the car bay used by motorcycle #3/#6.
func effectiveSpaces(vehicle string, carCapacity, motoCapacity, cars, motos int) (total, occupied int) {
	if vehicle == "motorcycle" || vehicle == "electric_bicycle" {
		freeCarBays := carCapacity - cars
		if freeCarBays < 0 {
			freeCarBays = 0
		}
		maxMotos := freeCarBays*3 + 2
		if motoCapacity < maxMotos {
			maxMotos = motoCapacity
		}
		return maxMotos, motos
	}
	return carCapacity, cars + motos/3
}

func (r *ReservationRepository) GetPriceForUnit(vehicleTypeID int, reservationTimeID int) (float32, error) {
	var price float32
	err := r.DB.QueryRow(`SELECT price FROM vehicle_prices WHERE vehicle_type_id = $1 AND reservation_time_id = $2`, vehicleTypeID, reservationTimeID).Scan(&price)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("no price configured for vehicle_type_id %d and reservation_time_id %d", vehicleTypeID, reservationTimeID)
		}
		return 0, err
	}
	return price, nil
}

func (r *ReservationRepository) CreateReservation(res *db.Reservation) error {
	query := `
		INSERT INTO reservations
		(code, user_name, user_email, user_phone, vehicle_type_id, vehicle_plate, vehicle_model, payment_method_id, status, start_time, end_time, created_at, updated_at, stripe_session_id, payment_status, language, total_price, deposit_payment)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id, created_at, updated_at`
	return r.DB.QueryRow(query,
		res.Code,
		res.UserName,
		res.UserEmail,
		res.UserPhone,
		res.VehicleTypeID,
		res.VehiclePlate,
		res.VehicleModel,
		res.PaymentMethodID,
		res.Status,
		res.StartTime,
		res.EndTime,
		res.CreatedAt,
		res.UpdatedAt,
		res.StripeSessionID,
		res.PaymentStatus,
		res.Language,
		res.TotalPrice,
		res.DepositPayment,
	).Scan(&res.ID, &res.CreatedAt, &res.UpdatedAt)
}

func (r *ReservationRepository) GetReservationByCode(code, email string) (*entities.ReservationResponse, error) {
	var res entities.ReservationResponse

	query := `
        SELECT
            r.code, r.user_name, r.user_email, r.user_phone,
            r.vehicle_type_id, vt.name AS vehicle_type_name,
            r.vehicle_plate, r.vehicle_model,
            r.payment_method_id, pm.name AS payment_method_name, r.stripe_session_id, r.payment_status,
            r.status, r.start_time, r.end_time, r.created_at, r.updated_at, r.language, r.total_price, r.deposit_payment
        FROM reservations r
        JOIN vehicle_types vt ON r.vehicle_type_id = vt.id
        JOIN payment_method pm ON r.payment_method_id = pm.id
        WHERE r.code = $1 AND r.user_email = $2
    `

	var totalPrice sql.NullFloat64
	var stripeSessionID sql.NullString
	var paymentStatus sql.NullString
	var depositPayment sql.NullFloat64
	err := r.DB.QueryRow(query, code, email).Scan(
		&res.Code, &res.UserName, &res.UserEmail, &res.UserPhone,
		&res.VehicleTypeID, &res.VehicleTypeName,
		&res.VehiclePlate, &res.VehicleModel,
		&res.PaymentMethodID, &res.PaymentMethodName, &stripeSessionID, &paymentStatus,
		&res.Status, &res.StartTime, &res.EndTime, &res.CreatedAt, &res.UpdatedAt, &res.Language, &totalPrice, &depositPayment,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("reservation with code '%s' and email '%s' not found: %w", code, email, err)
		}
		return nil, fmt.Errorf("error querying or scanning reservation: %w", err)
	}
	if stripeSessionID.Valid {
		res.StripeSessionID = stripeSessionID.String
	} else {
		res.StripeSessionID = ""
	}
	if paymentStatus.Valid {
		res.PaymentStatus = paymentStatus.String
	} else {
		res.PaymentStatus = ""
	}
	if totalPrice.Valid {
		res.TotalPrice = float32(totalPrice.Float64)
	} else {
		res.TotalPrice = 0
	}
	if depositPayment.Valid {
		res.DepositPayment = float32(depositPayment.Float64)
	} else {
		res.DepositPayment = 0
	}
	return &res, nil
}

func (r *ReservationRepository) CancelReservation(code string) (string, error) {
	timeUpdated := time.Now().UTC()
	query := `
		UPDATE reservations 
		SET status = 'canceled', updated_at = $2 
		WHERE code = $1 
		RETURNING status`
	var status string
	err := r.DB.QueryRow(query, code, timeUpdated).Scan(&status)
	if err != nil {
		return "", err
	}
	return status, nil
}

func (r *ReservationRepository) GetReservationByCodeOnly(code string) (*db.Reservation, error) {
	var res db.Reservation
	query := `
		SELECT id, code, user_name, user_email, user_phone, vehicle_type_id, vehicle_plate, vehicle_model, payment_method_id, status, start_time, end_time, created_at, updated_at, stripe_session_id, payment_status, language, total_price
		FROM reservations WHERE code = $1`
	var totalPrice sql.NullFloat64
	err := r.DB.QueryRow(query, code).Scan(
		&res.ID, &res.Code, &res.UserName, &res.UserEmail, &res.UserPhone, &res.VehicleTypeID, &res.VehiclePlate, &res.VehicleModel, &res.PaymentMethodID, &res.Status, &res.StartTime, &res.EndTime, &res.CreatedAt, &res.UpdatedAt,
		&res.StripeSessionID, &res.PaymentStatus, &res.Language, &totalPrice,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("reservation with code '%s' not found: %w", code, err)
		}
		return nil, fmt.Errorf("error querying reservation: %w", err)
	}
	if totalPrice.Valid {
		res.TotalPrice = totalPrice
	} else {
		res.TotalPrice = sql.NullFloat64{Float64: 0, Valid: false}
	}
	return &res, nil
}

func (r *ReservationRepository) GetReservationByStripeSessionID(sessionID string) (*db.Reservation, error) {
	var res db.Reservation
	var paymentIntentID sql.NullString
	var totalPrice sql.NullFloat64
	var depositPayment sql.NullFloat64
	query := `
		SELECT id, code, user_name, user_email, user_phone, vehicle_type_id, vehicle_plate, vehicle_model, payment_method_id, status, start_time, end_time, created_at, 
		       updated_at, stripe_session_id, payment_status, language, stripe_payment_intent_id, total_price, deposit_payment
		FROM reservations WHERE stripe_session_id = $1`
	err := r.DB.QueryRow(query, sessionID).Scan(
		&res.ID, &res.Code, &res.UserName, &res.UserEmail, &res.UserPhone, &res.VehicleTypeID, &res.VehiclePlate, &res.VehicleModel, &res.PaymentMethodID, &res.Status, &res.StartTime, &res.EndTime, &res.CreatedAt,
		&res.UpdatedAt, &res.StripeSessionID, &res.PaymentStatus, &res.Language, &paymentIntentID, &totalPrice, &depositPayment)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("reservation with sessionID '%s' not found: %w", sessionID, err)
		}
		return nil, fmt.Errorf("error querying reservation: %w", err)
	}
	if paymentIntentID.Valid {
		res.StripePaymentIntentID = paymentIntentID
	} else {
		res.StripePaymentIntentID = sql.NullString{String: "", Valid: false}
	}
	if totalPrice.Valid {
		res.TotalPrice = totalPrice
	} else {
		res.TotalPrice = sql.NullFloat64{Float64: 0, Valid: false}
	}
	if depositPayment.Valid {
		res.DepositPayment = depositPayment
	} else {
		res.DepositPayment = sql.NullFloat64{Float64: 0, Valid: false}
	}
	return &res, nil
}

func (r *ReservationRepository) UpdateReservationAndPaymentStatus(reservationID int, reservationStatus, paymentStatus string) error {
	query := `
		UPDATE reservations
		SET payment_status = $1, status = $2, updated_at = NOW()
		WHERE id = $3`
	_, err := r.DB.Exec(query, paymentStatus, reservationStatus, reservationID)
	return err
}

func (r *ReservationRepository) UpdateReservationStatusPaymentAndIntent(reservationID int, reservationStatus, paymentStatus, paymentIntentID string) error {
	query := `
		UPDATE reservations
		SET payment_status = $1, status = $2, stripe_payment_intent_id = $3, updated_at = NOW()
		WHERE id = $4`
	_, err := r.DB.Exec(query, paymentStatus, reservationStatus, paymentIntentID, reservationID)
	return err
}

 // ActivateOnlineReservation marks a paid Stripe checkout active exactly once.
 // The conditional update prevents duplicate webhook deliveries from sending duplicate notices.
func (r *ReservationRepository) ActivateOnlineReservation(sessionID, paymentIntentID string) (bool, error) {
 var id int64
 err := r.DB.QueryRow(`
  UPDATE reservations
  SET status = 'active', payment_status = 'succeeded',
      stripe_payment_intent_id = $2, updated_at = NOW()
  WHERE stripe_session_id = $1 AND payment_method_id = 2
    AND status = 'pending' AND COALESCE(payment_status,'') <> 'succeeded'
  RETURNING id
 `, sessionID, paymentIntentID).Scan(&id)
 if errors.Is(err, sql.ErrNoRows) { return false, nil }
 if err != nil { return false, err }
 return true, nil
}
