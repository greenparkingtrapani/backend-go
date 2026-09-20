package repository

import (
	"database/sql"
	"errors"
	"estacionamienti/internal/db"
	"estacionamienti/internal/entities"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type AdminRepository struct {
	DB *sql.DB
}

func NewAdminRepository(db *sql.DB) *AdminRepository {
	return &AdminRepository{DB: db}
}

func (r *AdminRepository) ListReservationsWithFilters(startTime, endTime, code, vehicleType, status, limit, offset, sortBy, sortOrder string) (reservationsList entities.ReservationsList, err error) {
	loc, _ := time.LoadLocation("Europe/Rome") // Horario de Italia

	// Build WHERE clause
	whereClause := " WHERE 1=1"
	args := []interface{}{}
	idx := 1

	var startTimeUTC, endTimeUTC time.Time

	// Procesar startTime
	if startTime != "" {
		startDateLocal, err := time.ParseInLocation("2006-01-02", startTime, loc)
		if err != nil {
			return reservationsList, err
		}
		startTimeUTC = startDateLocal.UTC()
	}

	// Procesar endTime
	if endTime != "" {
		endDateLocal, err := time.ParseInLocation("2006-01-02", endTime, loc)
		if err != nil {
			return reservationsList, err
		}
		endTimeUTC = endDateLocal.Add(24 * time.Hour).UTC() // Fin del día (exclusivo)
	}

	if startTime != "" && endTime != "" {
		whereClause += " AND r.start_time >= $" + strconv.Itoa(idx)
		args = append(args, startTimeUTC)
		idx++
		whereClause += " AND r.end_time < $" + strconv.Itoa(idx)
		args = append(args, endTimeUTC)
		idx++
	} else if startTime != "" {
		startEndUTC := startTimeUTC.Add(24 * time.Hour)
		whereClause += " AND r.start_time >= $" + strconv.Itoa(idx)
		args = append(args, startTimeUTC)
		idx++
		whereClause += " AND r.start_time < $" + strconv.Itoa(idx)
		args = append(args, startEndUTC)
		idx++
	} else if endTime != "" {
		endStartUTC := endTimeUTC.Add(-24 * time.Hour)
		whereClause += " AND r.end_time >= $" + strconv.Itoa(idx)
		args = append(args, endStartUTC)
		idx++
		whereClause += " AND r.end_time < $" + strconv.Itoa(idx)
		args = append(args, endTimeUTC)
		idx++
	}

	if code != "" {
		whereClause += " AND r.code LIKE $" + strconv.Itoa(idx)
		args = append(args, "%"+code+"%")
		idx++
	}
	if vehicleType != "" {
		whereClause += " AND vt.name = $" + strconv.Itoa(idx)
		args = append(args, vehicleType)
		idx++
	}
	if status != "" {
		whereClause += " AND r.status = $" + strconv.Itoa(idx)
		args = append(args, status)
		idx++
	}

	// Main query
	query := `
	SELECT
		r.code, r.user_name, r.user_email, r.user_phone, r.vehicle_type_id, vt.name AS vehicle_type_name,
		r.vehicle_plate, r.vehicle_model, r.payment_method_id, pm.name AS payment_method_name, COALESCE(r.payment_status, '') AS payment_status,
		r.status, r.start_time, r.end_time, r.created_at, r.updated_at, COALESCE(r.total_price, 0) AS total_price, COALESCE(r.deposit_payment, 0) AS deposit_payment
	FROM reservations r
	JOIN vehicle_types vt ON vt.id = r.vehicle_type_id
	JOIN payment_method pm ON pm.id = r.payment_method_id
	` + whereClause

	// Only allow known columns/directions because SQL identifiers cannot be parameterized.
	sortColumns := map[string]string{
		"code":     "r.code",
		"customer": "r.user_name",
		"vehicle":  "vt.name",
		"payment":  "pm.name",
		"amount":   "r.total_price",
		"start":    "r.start_time",
		"end":      "r.end_time",
	}
	statusOrder := "CASE r.status WHEN 'active' THEN 1 WHEN 'pending' THEN 2 WHEN 'finished' THEN 3 WHEN 'canceled' THEN 4 ELSE 5 END"
	paymentStatusOrder := "CASE COALESCE(r.payment_status, '') WHEN 'pending' THEN 1 WHEN 'succeeded' THEN 2 WHEN 'refunded' THEN 3 ELSE 4 END"
	direction := strings.ToUpper(sortOrder)
	if direction != "ASC" && direction != "DESC" {
		direction = "DESC"
	}

	column, requestedSort := sortColumns[sortBy]
	if sortBy == "status" {
		query += " ORDER BY " + statusOrder + " " + direction + ", r.id " + direction
	} else if sortBy == "vehicle" {
		query += " ORDER BY " + column + " " + direction + ", COALESCE(r.vehicle_plate, '') " + direction + ", COALESCE(r.vehicle_model, '') " + direction + ", r.id " + direction
	} else if sortBy == "payment" {
		query += " ORDER BY " + column + " " + direction + ", " + paymentStatusOrder + " " + direction + ", r.id " + direction
	} else if requestedSort {
		query += " ORDER BY " + column + " " + direction + ", r.id " + direction
	} else if startTime != "" {
		query += " ORDER BY r.start_time DESC, r.id DESC"
	} else if endTime != "" {
		query += " ORDER BY r.end_time DESC, r.id DESC"
	} else {
		query += " ORDER BY r.created_at DESC, r.id DESC"
	}

	if limit != "" {
		query += " LIMIT " + limit
	}
	if offset != "" {
		query += " OFFSET " + offset
	}

	rows, err := r.DB.Query(query, args...)
	if err != nil {
		return reservationsList, err
	}
	defer rows.Close()

	for rows.Next() {
		var res entities.ReservationResponse
		err := rows.Scan(
			&res.Code, &res.UserName, &res.UserEmail, &res.UserPhone, &res.VehicleTypeID, &res.VehicleTypeName,
			&res.VehiclePlate, &res.VehicleModel, &res.PaymentMethodID, &res.PaymentMethodName, &res.PaymentStatus,
			&res.Status, &res.StartTime, &res.EndTime, &res.CreatedAt, &res.UpdatedAt, &res.TotalPrice, &res.DepositPayment,
		)
		if err == nil {
			reservationsList.Reservations = append(reservationsList.Reservations, res)
		}
	}

	limitInt, _ := strconv.Atoi(limit)
	offsetInt, _ := strconv.Atoi(offset)
	reservationsList.Limit = limitInt
	reservationsList.Offset = offsetInt

	// Count query
	countQuery := `
		SELECT COUNT(*) FROM reservations r
		JOIN vehicle_types vt ON vt.id = r.vehicle_type_id
		JOIN payment_method pm ON pm.id = r.payment_method_id
	` + whereClause

	var total int
	err = r.DB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return reservationsList, err
	}
	reservationsList.Total = int64(total)

	return reservationsList, nil
}

// FindReservationByCode returns a reservation by code and maps it to entities.ReservationResponse
func (r *AdminRepository) FindReservationByCode(code string) (*entities.ReservationResponse, error) {
	var res entities.ReservationResponse

	query := `
        SELECT
            r.code, r.user_name, r.user_email, r.user_phone,
            r.vehicle_type_id, vt.name AS vehicle_type_name,
            r.vehicle_plate, r.vehicle_model,
            r.payment_method_id, pm.name AS payment_method_name,
            r.status, r.start_time, r.end_time, r.created_at, r.updated_at, r.language, r.total_price
        FROM reservations r
        JOIN vehicle_types vt ON vt.id = r.vehicle_type_id
        JOIN payment_method pm ON pm.id = r.payment_method_id
        WHERE r.code = $1`

	err := r.DB.QueryRow(query, code).Scan(
		&res.Code, &res.UserName, &res.UserEmail, &res.UserPhone,
		&res.VehicleTypeID, &res.VehicleTypeName,
		&res.VehiclePlate, &res.VehicleModel,
		&res.PaymentMethodID, &res.PaymentMethodName,
		&res.Status, &res.StartTime, &res.EndTime, &res.CreatedAt, &res.UpdatedAt, &res.Language, &res.TotalPrice,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("reservation with code '%s' not found: %w", code, err)
		}
		return nil, fmt.Errorf("error querying or scanning reservation: %w", err)
	}
	return &res, nil
}

func (r *AdminRepository) ListVehicleSpaces() ([]db.VehicleSpaceWithPrices, error) {
	query := `SELECT vt.id, vt.name, vs.spaces FROM vehicle_spaces vs JOIN vehicle_types vt ON vs.vehicle_type_id = vt.id`
	rows, err := r.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []db.VehicleSpaceWithPrices
	for rows.Next() {
		var vehicleTypeID int
		var vehicleTypeName string
		var spaces int
		err := rows.Scan(&vehicleTypeID, &vehicleTypeName, &spaces)
		if err != nil {
			continue
		}

		// Query prices for this vehicle type
		pricesQuery := `SELECT rt.name, vp.price FROM vehicle_prices vp JOIN reservation_times rt ON vp.reservation_time_id = rt.id WHERE vp.vehicle_type_id = $1`
		priceRows, err := r.DB.Query(pricesQuery, vehicleTypeID)
		if err != nil {
			continue
		}
		prices := make(map[string]float32)
		for priceRows.Next() {
			var reservationTime string
			var price float32
			if err := priceRows.Scan(&reservationTime, &price); err == nil {
				prices[reservationTime] = price
			}
		}
		priceRows.Close()

		result = append(result, db.VehicleSpaceWithPrices{
			VehicleType: vehicleTypeName,
			Spaces:      spaces,
			Prices:      prices,
		})
	}
	return result, nil
}

func (r *AdminRepository) UpdateVehicleSpaces(vehicleType string, spaces int) error {
	query := `
		UPDATE vehicle_spaces vs
		SET spaces = $1
		FROM vehicle_types vt
		WHERE vs.vehicle_type_id = vt.id AND vt.name = $2
	`
	_, err := r.DB.Exec(query, spaces, vehicleType)
	return err
}

func (r *AdminRepository) UpdateVehiclePrice(vehicleType string, timeName string, price float32) error {
	// Upsert price with subqueries to fetch IDs in a single statement
	query := `
		INSERT INTO vehicle_prices (vehicle_type_id, reservation_time_id, price)
		VALUES (
			(SELECT id FROM vehicle_types WHERE name = $1),
			(SELECT id FROM reservation_times WHERE name = $2),
			$3
		)
		ON CONFLICT (vehicle_type_id, reservation_time_id)
		DO UPDATE SET price = EXCLUDED.price
	`
	_, err := r.DB.Exec(query, vehicleType, timeName, price)
	return err
}
