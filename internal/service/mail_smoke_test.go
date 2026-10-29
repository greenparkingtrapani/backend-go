package service

import (
 "context"
 "crypto/sha256"
 "database/sql"
 "encoding/hex"
 "fmt"
 "log"
 "net/mail"
 "os"
 "time"
)

// RunConfiguredEmailSmokeTest performs one authenticated-provider delivery probe
// only when enabled explicitly via environment variables. It never creates a reservation.
// Each named test is consumed once per database; SMTP/API acceptance does not imply inbox delivery.
func RunConfiguredEmailSmokeTest(db *sql.DB) {
 recipient := os.Getenv("GREENPARKING_EMAIL_SMOKE_TO")
 testID := os.Getenv("GREENPARKING_EMAIL_SMOKE_ID")
 if recipient=="" || testID=="" {return}
 address,err:=mail.ParseAddress(recipient)
 if err!=nil || address.Address!=recipient || len(testID)>128 {
  log.Print("EMAIL_SMOKE: invalid recipient or test ID")
  return
 }
 hash:=sha256.Sum256([]byte(testID))
 key:=hex.EncodeToString(hash[:])
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second)
 defer cancel()
 if _,err=db.ExecContext(ctx,`CREATE TABLE IF NOT EXISTS public.email_smoke_tests (
  id TEXT PRIMARY KEY, provider_id TEXT NOT NULL DEFAULT '', completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
 )`);err!=nil {log.Printf("EMAIL_SMOKE: create ledger error: %v",err);return}
 tx,err:=db.BeginTx(ctx,nil)
 if err!=nil {log.Printf("EMAIL_SMOKE: transaction error: %v",err);return}
 defer tx.Rollback()
 var consumed string
 err=tx.QueryRowContext(ctx,`INSERT INTO public.email_smoke_tests(id) VALUES($1)
 ON CONFLICT(id) DO NOTHING RETURNING id`,key).Scan(&consumed)
 if err==sql.ErrNoRows {log.Print("EMAIL_SMOKE: already executed; skipping");return}
 if err!=nil {log.Printf("EMAIL_SMOKE: ledger error: %v",err);return}
 subject:="Green Parking - Test conferma prenotazione"
 body:=fmt.Sprintf("Questa è una prova tecnica dell'invio email dal server Green Parking tramite Resend.\nNessuna prenotazione reale è stata creata.\nData test: %s\n",time.Now().In(time.FixedZone("CET",3600)).Format(time.RFC3339))
 if err:=SendEmailWithResend(recipient,"",subject,body,"");err!=nil {
  log.Printf("EMAIL_SMOKE: provider rejected email: %v",err);return
 }
 if err:=tx.Commit();err!=nil {log.Printf("EMAIL_SMOKE: accepted by provider but ledger commit failed: %v",err);return}
 log.Printf("EMAIL_SMOKE: provider accepted one-time test email. Check inbox/spam to confirm delivery.")
}
