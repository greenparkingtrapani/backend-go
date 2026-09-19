package entities

type PriceResponse struct {
	VehicleType     string  `json:"vehicle_type"`
	ReservationTime string  `json:"reservation_time"`
	Price           float32 `json:"price"`
}

type TotalPriceResponse struct {
	TotalPrice float32 `json:"total_price"`
	Discount   bool    `json:"discount"`
}
