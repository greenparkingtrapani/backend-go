package service

import (
	"bytes"
	"estacionamienti/internal/entities"
	"fmt"
	"html/template"
	"log"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	adminNotificationEmailEnv      = "ADMIN_NOTIFICATION_EMAIL"
	adminNotificationRecipientName = "Amministratore"
	adminNotificationLanguage      = "it"
	confirmedReservationStatus     = "confirmed"
	canceledReservationStatus      = "canceled"
)

type SenderService struct {
}

func NewSenderService() *SenderService {
	return &SenderService{}
}

func (s *SenderService) SendReservationEmail(reservation entities.ReservationResponse, status string) {
	emailData := reservationEmailData(reservation, status, false)
	subject, plainTextBody := customerReservationEmailCopy(emailData)
	htmlBody := renderReservationEmail(emailData)
	sendReservationEmailAsync(reservation.UserEmail, emailData.UserName, subject, plainTextBody, htmlBody, emailData.ReservationCode)
}

func (s *SenderService) SendAdminReservationEmail(reservation entities.ReservationResponse, statusKey string) {
	adminEmail := strings.TrimSpace(os.Getenv(adminNotificationEmailEnv))
	if adminEmail == "" {
		log.Printf("ADVERTENCIA: %s no está configurada. No se enviará el aviso al administrador de la reserva %s.", adminNotificationEmailEnv, reservation.Code)
		return
	}
	if _, err := mail.ParseAddress(adminEmail); err != nil {
		log.Printf("ADVERTENCIA: %s no es un correo válido. No se enviará el aviso al administrador de la reserva %s.", adminNotificationEmailEnv, reservation.Code)
		return
	}

	status := s.StatusTranslation(statusKey, adminNotificationLanguage)
	emailData := reservationEmailData(reservation, status, true)
	subject, plainTextBody := adminReservationEmailCopy(emailData)
	htmlBody := renderReservationEmail(emailData)
	sendReservationEmailAsync(adminEmail, adminNotificationRecipientName, subject, plainTextBody, htmlBody, emailData.ReservationCode)
}

func reservationEmailData(reservation entities.ReservationResponse, status string, isAdmin bool) entities.ReservationEmailData {
	italyLoc, errLoc := time.LoadLocation("Europe/Rome")
	if errLoc != nil {
		italyLoc = time.FixedZone("CET", 1*60*60)
	}

	language := reservation.Language
	if isAdmin {
		language = adminNotificationLanguage
	}

	return entities.ReservationEmailData{
		UserName:           reservation.UserName,
		UserEmail:          reservation.UserEmail,
		UserPhone:          reservation.UserPhone,
		ReservationCode:    reservation.Code,
		VehicleModel:       reservation.VehicleModel,
		VehiclePlate:       reservation.VehiclePlate,
		StartTimeFormatted: reservation.StartTime.In(italyLoc).Format("02 Jan 2006 15:04 MST"),
		EndTimeFormatted:   reservation.EndTime.In(italyLoc).Format("02 Jan 2006 15:04 MST"),
		CurrentYear:        time.Now().In(italyLoc).Year(),
		Language:           language,
		Status:             status,
		IsAdmin:            isAdmin,
		IsCancellation:     isCancellationStatus(status),
	}
}

func isCancellationStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case canceledReservationStatus, "cancelled", "cancelada", "annullata":
		return true
	default:
		return false
	}
}

func customerReservationEmailCopy(emailData entities.ReservationEmailData) (string, string) {
	if emailData.IsCancellation {
		return customerCancellationEmailCopy(emailData)
	}

	switch emailData.Language {
	case "es":
		subject := fmt.Sprintf("Tu reserva en GreenParking está %s - Código: %s", emailData.Status, emailData.ReservationCode)
		body := fmt.Sprintf(
			"Hola %s,\n\nTu reserva en GreenParking está %s.\n\n"+
				"Detalles de la reserva:\n"+
				"Código de Reserva: %s\n"+
				"Vehículo: %s (Patente: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"Gracias por elegir GreenParking.\n\n"+
				"© %d GreenParking. Todos los derechos reservados.",
			emailData.UserName, emailData.Status, emailData.ReservationCode, emailData.VehicleModel, emailData.VehiclePlate,
			emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
		return subject, body
	case "it":
		subject := fmt.Sprintf("La tua prenotazione GreenParking è %s - Codice: %s", emailData.Status, emailData.ReservationCode)
		body := fmt.Sprintf(
			"Ciao %s,\n\nLa tua prenotazione presso GreenParking è %s.\n\n"+
				"Dettagli della prenotazione:\n"+
				"Codice prenotazione: %s\n"+
				"Veicolo: %s (Targa: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"Grazie per aver scelto GreenParking.\n\n"+
				"© %d GreenParking. Tutti i diritti riservati.",
			emailData.UserName, emailData.Status, emailData.ReservationCode, emailData.VehicleModel, emailData.VehiclePlate,
			emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
		return subject, body
	default:
		subject := fmt.Sprintf("Your GreenParking reservation is %s - Code: %s", emailData.Status, emailData.ReservationCode)
		body := fmt.Sprintf(
			"Hello %s,\n\nYour reservation at GreenParking is %s.\n\n"+
				"Reservation Details:\n"+
				"Reservation Code: %s\n"+
				"Vehicle: %s (Plate: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"Thank you for choosing GreenParking.\n\n"+
				"© %d GreenParking. All rights reserved.",
			emailData.UserName, emailData.Status, emailData.ReservationCode, emailData.VehicleModel, emailData.VehiclePlate,
			emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
		return subject, body
	}
}

func customerCancellationEmailCopy(emailData entities.ReservationEmailData) (string, string) {
	switch emailData.Language {
	case "es":
		subject := fmt.Sprintf("Tu reserva en GreenParking ha sido cancelada - Código: %s", emailData.ReservationCode)
		body := fmt.Sprintf(
			"Hola %s,\n\nTe informamos que tu reserva en GreenParking ha sido cancelada.\n\n"+
				"Detalles de la reserva cancelada:\n"+
				"Código de Reserva: %s\n"+
				"Vehículo: %s (Patente: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"Si no solicitaste esta cancelación o tienes alguna consulta, contáctanos.\n\n"+
				"© %d GreenParking. Todos los derechos reservados.",
			emailData.UserName, emailData.ReservationCode, emailData.VehicleModel, emailData.VehiclePlate,
			emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
		return subject, body
	case "it":
		subject := fmt.Sprintf("La tua prenotazione GreenParking è stata annullata - Codice: %s", emailData.ReservationCode)
		body := fmt.Sprintf(
			"Ciao %s,\n\nTi informiamo che la tua prenotazione presso GreenParking è stata annullata.\n\n"+
				"Dettagli della prenotazione annullata:\n"+
				"Codice prenotazione: %s\n"+
				"Veicolo: %s (Targa: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"Se non hai richiesto tu questa cancellazione o hai domande, contattaci.\n\n"+
				"© %d GreenParking. Tutti i diritti riservati.",
			emailData.UserName, emailData.ReservationCode, emailData.VehicleModel, emailData.VehiclePlate,
			emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
		return subject, body
	default:
		subject := fmt.Sprintf("Your GreenParking reservation has been canceled - Code: %s", emailData.ReservationCode)
		body := fmt.Sprintf(
			"Hello %s,\n\nWe inform you that your GreenParking reservation has been canceled.\n\n"+
				"Canceled reservation details:\n"+
				"Reservation Code: %s\n"+
				"Vehicle: %s (Plate: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"If you did not request this cancellation or have any questions, please contact us.\n\n"+
				"© %d GreenParking. All rights reserved.",
			emailData.UserName, emailData.ReservationCode, emailData.VehicleModel, emailData.VehiclePlate,
			emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
		return subject, body
	}
}

func adminReservationEmailCopy(emailData entities.ReservationEmailData) (string, string) {
	if emailData.IsCancellation {
		subject := fmt.Sprintf("Prenotazione annullata dal cliente - Codice: %s", emailData.ReservationCode)
		body := fmt.Sprintf(
			"Un cliente ha annullato una prenotazione su GreenParking. Lo stato è %s.\n\n"+
				"Dettagli della prenotazione:\n"+
				"Codice prenotazione: %s\n"+
				"Cliente: %s\n"+
				"Email: %s\n"+
				"Telefono: %s\n"+
				"Veicolo: %s (Targa: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"Questo avviso è per l'amministratore del parcheggio.\n\n"+
				"© %d GreenParking. Tutti i diritti riservati.",
			emailData.Status, emailData.ReservationCode, emailData.UserName, emailData.UserEmail, emailData.UserPhone,
			emailData.VehicleModel, emailData.VehiclePlate, emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
		return subject, body
	}

	subject := fmt.Sprintf("Nuova prenotazione confermata su GreenParking - Codice: %s", emailData.ReservationCode)
	body := fmt.Sprintf(
		"C'è una nuova prenotazione su GreenParking e il pagamento è già stato effettuato. Lo stato è %s.\n\n"+
			"Dettagli della prenotazione:\n"+
			"Codice prenotazione: %s\n"+
			"Cliente: %s\n"+
			"Email: %s\n"+
			"Telefono: %s\n"+
			"Veicolo: %s (Targa: %s)\n"+
			"Check-in: %s\n"+
			"Check-out: %s\n\n"+
			"Questo avviso è per l'amministratore del parcheggio.\n\n"+
			"© %d GreenParking. Tutti i diritti riservati.",
		emailData.Status, emailData.ReservationCode, emailData.UserName, emailData.UserEmail, emailData.UserPhone,
		emailData.VehicleModel, emailData.VehiclePlate, emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
	)
	return subject, body
}

func renderReservationEmail(emailData entities.ReservationEmailData) string {
	tmplPath := filepath.Join("internal", "templates", "reservation_email.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Printf("ALERTA: Error al parsear la plantilla de correo HTML (%s): %v", tmplPath, err)
		return ""
	}

	var htmlBodyBuffer bytes.Buffer
	if err := tmpl.Execute(&htmlBodyBuffer, emailData); err != nil {
		log.Printf("ALERTA: Error al ejecutar la plantilla de correo HTML para reserva %s: %v", emailData.ReservationCode, err)
		return ""
	}
	return htmlBodyBuffer.String()
}

func sendReservationEmailAsync(toEmail, toName, subject, plainBody, htmlBody, reservationCode string) {
	go func() {
		errEmail := SendEmailWithResend(toEmail, toName, subject, plainBody, htmlBody)
		if errEmail != nil {
			log.Printf("ALERTA (asíncrono): Falló envío de correo para reserva %s: %v", reservationCode, errEmail)
		}
	}()
}

func (s *SenderService) SendReservationSMS(reservation entities.ReservationResponse, status string) {
	italyLoc, errLoc := time.LoadLocation("Europe/Rome")
	if errLoc != nil {
		italyLoc = time.FixedZone("CET", 1*60*60)
	}

	userPhoneNumber := reservation.UserPhone
	reservationCode := reservation.Code

	var smsMessage string
	switch reservation.Language {
	case "es":
		smsMessage = fmt.Sprintf("GreenParking: ¡Tu reserva %s está %s!\nCheck-in: %s.\nMás detalles en tu correo.",
			reservationCode, status,
			reservation.StartTime.In(italyLoc).Format("02/01 15:04"),
		)
	case "it":
		smsMessage = fmt.Sprintf("GreenParking: La tua prenotazione %s è stata %s!\nCheck-in: %s.\nAltri dettagli nella tua email.",
			reservationCode, status,
			reservation.StartTime.In(italyLoc).Format("02/01 15:04"),
		)
	default:
		smsMessage = fmt.Sprintf("GreenParking: Reservation %s has been %s!\nCheck-in: %s.\nMore details in your email.",
			reservationCode, status,
			reservation.StartTime.In(italyLoc).Format("02/01 15:04"),
		)
	}

	errSMS := SendSMS(userPhoneNumber, smsMessage)
	if errSMS != nil {
		log.Printf("ALERTA: La reserva %s se creó, pero falló el envío del SMS de confirmación a %s: %v", reservationCode, userPhoneNumber, errSMS)
	}
}

func (s *SenderService) StatusTranslation(status, lang string) string {
	switch lang {
	case "es":
		switch status {
		case "pending":
			return "pendiente"
		case "active":
			return "activa"
		case "finished":
			return "finalizada"
		case "canceled", "cancelled":
			return "cancelada"
		case "confirmed":
			return "confirmada"
		}
	case "it":
		switch status {
		case "pending":
			return "in attesa"
		case "active":
			return "attiva"
		case "finished":
			return "finito"
		case "canceled", "cancelled":
			return "annullata"
		case "confirmed":
			return "confermata"
		}
	}
	// Default: English
	return status
}
