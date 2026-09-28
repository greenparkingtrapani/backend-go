package entities

type ReservationEmailData struct {
	UserName           string
	UserEmail          string
	UserPhone          string
	ReservationCode    string
	VehicleModel       string
	VehiclePlate       string
	StartTimeFormatted string
	EndTimeFormatted   string
	CurrentYear        int
	Language           string
	Status             string
	IsAdmin            bool
}
