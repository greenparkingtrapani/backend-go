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
	if data.IsCancellation {
		t.Fatal("confirmation email should not be marked as cancellation")
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

func TestAdminCancellationEmailStaysInItalian(t *testing.T) {
	data := reservationEmailData(sampleReservation(), "annullata", true)

	if data.Language != adminNotificationLanguage {
		t.Fatalf("language = %q", data.Language)
	}
	if !data.IsCancellation {
		t.Fatal("expected cancellation email")
	}

	subject, body := adminReservationEmailCopy(data)

	if !strings.Contains(subject, "Prenotazione annullata dal cliente") {
		t.Fatalf("subject = %q", subject)
	}
	if strings.Contains(subject, "Nuova prenotazione confermata") {
		t.Fatalf("cancellation subject should not look like confirmation: %q", subject)
	}
	for _, want := range []string{"ha annullato", "Ana Perez", "ana@example.com", "RES123", "annullata", "amministratore del parcheggio"} {
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

func TestCustomerCancellationEmailCopy(t *testing.T) {
	data := reservationEmailData(sampleReservation(), "cancelada", false)

	if !data.IsCancellation {
		t.Fatal("expected cancellation email")
	}

	subject, body := customerReservationEmailCopy(data)

	if !strings.Contains(subject, "ha sido cancelada") {
		t.Fatalf("subject = %q", subject)
	}
	if !strings.Contains(body, "ha sido cancelada") {
		t.Fatalf("body = %q", body)
	}
	if strings.Contains(body, "Gracias por elegir GreenParking") {
		t.Fatalf("cancellation body should not use confirmation closing:\n%s", body)
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
