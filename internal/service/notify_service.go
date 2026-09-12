package service

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/resend/resend-go/v4"
	"github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

const smsSendingEnabled = false

func SendEmailWithResend(toEmailAddress, toName, subject, plainTextContent, htmlContent string) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		log.Println("ADVERTENCIA: RESEND_API_KEY no está configurada. El correo no se enviará.")
		return fmt.Errorf("RESEND_API_KEY no está configurada")
	}

	fromEmail := os.Getenv("RESEND_FROM_EMAIL")
	if fromEmail == "" {
		log.Println("ADVERTENCIA: RESEND_FROM_EMAIL no está configurada. El correo no se enviará.")
		return fmt.Errorf("RESEND_FROM_EMAIL no está configurada")
	}

	fromName := os.Getenv("RESEND_FROM_NAME")
	if fromName == "" {
		fromName = "GreenParking"
	}

	to := toEmailAddress
	if toName != "" {
		to = fmt.Sprintf("%s <%s>", toName, toEmailAddress)
	}

	client := resend.NewClient(apiKey)
	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("%s <%s>", fromName, fromEmail),
		To:      []string{to},
		Subject: subject,
		Text:    plainTextContent,
		Html:    htmlContent,
	}

	sent, err := client.Emails.Send(params)
	if err != nil {
		log.Printf("Error al intentar enviar correo vía Resend a %s: %v", toEmailAddress, err)
		return fmt.Errorf("falló el envío del correo a través de Resend: %w", err)
	}

	log.Printf("Correo enviado exitosamente a %s (Asunto: %s). ID: %s", toEmailAddress, subject, sent.Id)
	return nil
}

func SendSMS(toNumber string, messageBody string) error {
	if !smsSendingEnabled {
		log.Println("SMS sending is disabled; skipping.")
		return nil
	}

	accountSid := os.Getenv("TWILIO_ACCOUNT_SID")
	authToken := os.Getenv("TWILIO_AUTH_TOKEN")
	fromNumber := os.Getenv("TWILIO_FROM_NUMBER")

	if accountSid == "" || authToken == "" || fromNumber == "" {
		log.Println("ADVERTENCIA: Las credenciales de Twilio (SID, Token o From Number) no están configuradas. El SMS no se enviará.")
		return fmt.Errorf("credenciales de Twilio no configuradas completamente")
	}

	if !strings.HasPrefix(toNumber, "+") {
		log.Printf("ADVERTENCIA: El número de destino '%s' no está en formato E.164 (debe empezar con '+'). El SMS podría fallar.", toNumber)
	}

	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username:   accountSid,
		Password:   authToken,
		AccountSid: accountSid,
	})

	params := &openapi.CreateMessageParams{}
	params.SetTo(toNumber)
	params.SetFrom(fromNumber)
	params.SetBody(messageBody)

	resp, err := client.Api.CreateMessage(params)
	if err != nil {
		log.Printf("Error al enviar SMS a %s vía Twilio: %v", toNumber, err)
		return fmt.Errorf("falló el envío del SMS: %w", err)
	}

	if resp != nil && resp.Sid != nil {
		log.Printf("SMS enviado exitosamente a %s. SID del Mensaje: %s", toNumber, *resp.Sid)
	} else {
		log.Printf("SMS enviado a %s, pero no se recibió SID en la respuesta (esto es inusual si no hubo error).", toNumber)
	}

	return nil
}
