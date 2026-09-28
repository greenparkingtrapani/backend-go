package service

import (
	"estacionamienti/internal/entities"
	"strings"
	"testing"
	"time"
)

func TestAdminReservationEmailStaysInItalian(t *testing.T) {
	data := reservationEmailData(sampleReservation(), "confermata", true)

	if data.Language != adminNotificationLanguage {
		t.Fatalf("language = %q", data.Language)
	}

	subject, body := adminReservationEmailCopy(data)

	if !strings.Contains(subject, "Nuova prenotazione confermata") {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{"Ana Perez", "ana@example.com", "+39 333", "AA000BB", "RES123", "amministratore del parcheggio", "confermata"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestCustomerReservationEmailCopyStaysAddressedToCustomer(t *testing.T) {
	data := reservationEmailData(sampleReservation(), "confirmada", false)

	subject, body := customerReservationEmailCopy(data)

	if !strings.Contains(subject, "Tu reserva en GreenParking está confirmada") {
		t.Fatalf("subject = %q", subject)
	}
	if strings.Contains(body, "administrador") {
		t.Fatalf("customer body should not mention the administrator:\n%s", body)
	}
	if !strings.Contains(body, "Hola Ana Perez") {
		t.Fatalf("body = %q", body)
	}
}

func sampleReservation() entities.ReservationResponse {
	start := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	return entities.ReservationResponse{
		Code:         "RES123",
		UserName:     "Ana Perez",
		UserEmail:    "ana@example.com",
		UserPhone:    "+39 333",
		VehicleModel: "Fiat 500",
		VehiclePlate: "AA000BB",
		Language:     "es",
		StartTime:    start,
		EndTime:      start.Add(24 * time.Hour),
	}
}
