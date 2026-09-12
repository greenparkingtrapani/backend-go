package service

import (
	"bytes"
	"estacionamienti/internal/entities"
	"fmt"
	"html/template"
	"log"
	"path/filepath"
	"time"
)

type SenderService struct {
}

func NewSenderService() *SenderService {
	return &SenderService{}
}

func (s *SenderService) SendReservationEmail(reservation entities.ReservationResponse, status string) {
	italyLoc, errLoc := time.LoadLocation("Europe/Rome")
	if errLoc != nil {
		italyLoc = time.FixedZone("CET", 1*60*60) // fallback CET
	}

	emailData := entities.ReservationEmailData{
		UserName:           reservation.UserName,
		ReservationCode:    reservation.Code,
		VehicleModel:       reservation.VehicleModel,
		VehiclePlate:       reservation.VehiclePlate,
		StartTimeFormatted: reservation.StartTime.In(italyLoc).Format("02 Jan 2006 15:04 MST"),
		EndTimeFormatted:   reservation.EndTime.In(italyLoc).Format("02 Jan 2006 15:04 MST"),
		CurrentYear:        time.Now().In(italyLoc).Year(),
		Language:           reservation.Language,
		Status:             status,
	}

	var emailSubject, plainTextBody string
	switch reservation.Language {
	case "es":
		emailSubject = fmt.Sprintf("Tu reserva en GreenParking está %s - Código: %s", status, emailData.ReservationCode)
		plainTextBody = fmt.Sprintf(
			"Hola %s,\n\nTu reserva en GreenParking está %s.\n\n"+
				"Detalles de la reserva:\n"+
				"Código de Reserva: %s\n"+
				"Vehículo: %s (Patente: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"Gracias por elegir GreenParking.\n\n"+
				"GreenParking. Todos los derechos reservados.",
			emailData.UserName, status, emailData.ReservationCode, emailData.VehicleModel, emailData.VehiclePlate,
			emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
	case "it":
		emailSubject = fmt.Sprintf("La tua prenotazione GreenParking è %s - Codice: %s", status, emailData.ReservationCode)
		plainTextBody = fmt.Sprintf(
			"Ciao %s,\n\nLa tua prenotazione presso GreenParking è %s.\n\n"+
				"Dettagli della prenotazione:\n"+
				"Codice prenotazione: %s\n"+
				"Veicolo: %s (Targa: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"Grazie per aver scelto GreenParking.\n\n"+
				"GreenParking. Tutti i diritti riservati.",
			emailData.UserName, status, emailData.ReservationCode, emailData.VehicleModel, emailData.VehiclePlate,
			emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
	default:
		emailSubject = fmt.Sprintf("Your GreenParking reservation is %s - Code: %s", status, emailData.ReservationCode)
		plainTextBody = fmt.Sprintf(
			"Hello %s,\n\nYour reservation at GreenPark is %s.\n\n"+
				"Reservation Details:\n"+
				"Reservation Code: %s\n"+
				"Vehicle: %s (Plate: %s)\n"+
				"Check-in: %s\n"+
				"Check-out: %s\n\n"+
				"Thank you for choosing GreenParking.\n\n"+
				"GreenParking. All rights reserved.",
			emailData.UserName, status, emailData.ReservationCode, emailData.VehicleModel, emailData.VehiclePlate,
			emailData.StartTimeFormatted, emailData.EndTimeFormatted, emailData.CurrentYear,
		)
	}

	tmplPath := filepath.Join("internal", "templates", "reservation_email.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Printf("ALERTA: Error al parsear la plantilla de correo HTML (%s): %v", tmplPath, err)
	}

	var htmlBodyBuffer bytes.Buffer
	if err := tmpl.Execute(&htmlBodyBuffer, emailData); err != nil {
		log.Printf("ALERTA: Error al ejecutar la plantilla de correo HTML para reserva %s: %v", emailData.ReservationCode, err)
	}
	htmlBody := htmlBodyBuffer.String()

	go func(toEmail, userName, subject, plainBody, htmlBodyContent string) {
		errEmail := SendEmailWithResend(toEmail, userName, subject, plainBody, htmlBodyContent)
		if errEmail != nil {
			log.Printf("ALERTA (asíncrono): Falló envío de correo para reserva %s: %v", emailData.ReservationCode, errEmail)
		}
	}(reservation.UserEmail, emailData.UserName, emailSubject, plainTextBody, htmlBody)
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
