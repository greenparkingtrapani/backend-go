package api

import (
 "crypto/subtle"
 "database/sql"
 "encoding/json"
 "errors"
 "log"
 "net/http"
 "os"
 "regexp"
 "strings"
 "time"
 "context"
)

type ManagerOccupancyHandler struct { DB *sql.DB }
type managerStay struct {
 LocalStayID string `json:"local_stay_id"`
 VehicleType string `json:"vehicle_type"`
 ArrivalLocal string `json:"arrival_local"`
 BookingCode string `json:"booking_code,omitempty"`
}
type managerSnapshot struct {
 Version int `json:"version"`
 InstallationID string `json:"installation_id"`
 GeneratedAt string `json:"generated_at"`
 Checksum string `json:"checksum"`
 Stays []managerStay `json:"stays"`
}
var managerIDPattern=regexp.MustCompile(`^[a-zA-Z0-9_-]{6,80}$`)
var managerStayIDPattern=regexp.MustCompile(`^[0-9]{1,18}$`)
var managerBookingPattern=regexp.MustCompile(`^[A-Za-z0-9-]{0,64}$`)
var managerKinds=map[string]bool{"car":true,"sedan":true,"suv":true,"motorcycle":true,"electric_bicycle":true}

// Occupancy is deliberately disabled when credentials have not been configured.
// It never handles names, phone numbers, license plates or payment data.
func (h *ManagerOccupancyHandler) Occupancy(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPut { http.Error(w,"Method not allowed",405);return }
 secret:=os.Getenv("GREENPARKING_MANAGER_SYNC_TOKEN")
 allowedInstallation:=os.Getenv("GREENPARKING_MANAGER_INSTALLATION_ID")
 if len(secret)<32 || !managerIDPattern.MatchString(allowedInstallation) {
  http.Error(w,"Manager sync is not configured",503);return
 }
 auth:=r.Header.Get("Authorization")
 given:=strings.TrimPrefix(auth,"Bearer ")
 if !strings.HasPrefix(auth,"Bearer ") || subtle.ConstantTimeCompare([]byte(given),[]byte(secret))!=1 {
  http.Error(w,"Unauthorized",401);return
 }
 r.Body=http.MaxBytesReader(w,r.Body,256*1024)
 var input managerSnapshot
 decoder:=json.NewDecoder(r.Body)
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&input);err!=nil { http.Error(w,"Invalid snapshot",400);return }
 if input.Version!=1 || input.InstallationID!=allowedInstallation || len(input.Stays)>1000 ||
  len(input.Checksum)!=64 {
  http.Error(w,"Invalid snapshot metadata",400);return
 }
 generated,err:=time.Parse(time.RFC3339Nano,input.GeneratedAt)
 if err!=nil || time.Since(generated)>10*time.Minute || time.Until(generated)>2*time.Minute {
  http.Error(w,"Snapshot timestamp outside tolerance",400);return
 }
 ids:=make(map[string]bool)
 for _,st:=range input.Stays {
  if !managerStayIDPattern.MatchString(st.LocalStayID)||!managerKinds[st.VehicleType]||
   !managerBookingPattern.MatchString(st.BookingCode) || ids[st.LocalStayID] {
   http.Error(w,"Invalid stay",400);return
  }
  ids[st.LocalStayID]=true
  if _,err:=time.ParseInLocation("2006-01-02T15:04:05",st.ArrivalLocal,time.FixedZone("Europe/Rome",3600));err!=nil {
   http.Error(w,"Invalid arrival timestamp",400);return
  }
 }
 ctx,cancel:=context.WithTimeout(r.Context(),8*time.Second);defer cancel()
 tx,err:=h.DB.BeginTx(ctx,nil)
 if err!=nil {http.Error(w,"Database unavailable",503);return}
 defer tx.Rollback()
 if _,err=tx.ExecContext(ctx,`SELECT pg_advisory_xact_lock(hashtext($1))`,input.InstallationID);err!=nil {http.Error(w,"Database lock error",503);return}
 if _,err=tx.ExecContext(ctx,`DELETE FROM public.manager_active_stays WHERE installation_id=$1`,input.InstallationID);err!=nil {http.Error(w,"Database write error",503);return}
 for _,st:=range input.Stays {
  _,err=tx.ExecContext(ctx,`INSERT INTO public.manager_active_stays
   (installation_id,local_stay_id,vehicle_type,arrival_local,booking_code)
   VALUES($1,$2,$3,$4,$5)`,input.InstallationID,st.LocalStayID,st.VehicleType,st.ArrivalLocal,st.BookingCode)
  if err!=nil {http.Error(w,"Database write error",503);return}
 }
 if _,err=tx.ExecContext(ctx,`INSERT INTO public.manager_sync_status(installation_id,checksum,last_sync_at,active_stays)
   VALUES($1,$2,NOW(),$3)
   ON CONFLICT(installation_id) DO UPDATE SET checksum=EXCLUDED.checksum,last_sync_at=NOW(),
   active_stays=EXCLUDED.active_stays`,input.InstallationID,input.Checksum,len(input.Stays));err!=nil {
  http.Error(w,"Database write error",503);return
 }
 if err=tx.Commit();err!=nil {http.Error(w,"Database commit error",503);return}
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 _=json.NewEncoder(w).Encode(map[string]interface{}{"accepted":true,"active_stays":len(input.Stays)})
}

type managerOnlineReservation struct {
 Code string `json:"code"`
 VehicleType string `json:"vehicle_type"`
 StartTime time.Time `json:"start_time"`
 EndTime time.Time `json:"end_time"`
 Status string `json:"status"`
}
func (h *ManagerOccupancyHandler) Reservations(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodGet {http.Error(w,"Method not allowed",405);return}
 secret:=os.Getenv("GREENPARKING_MANAGER_SYNC_TOKEN")
 if len(secret)<32 || !managerIDPattern.MatchString(os.Getenv("GREENPARKING_MANAGER_INSTALLATION_ID")) {
  http.Error(w,"Manager sync is not configured",503);return
 }
 auth:=r.Header.Get("Authorization")
 given:=strings.TrimPrefix(auth,"Bearer ")
 if !strings.HasPrefix(auth,"Bearer ") || subtle.ConstantTimeCompare([]byte(given),[]byte(secret))!=1 {
  http.Error(w,"Unauthorized",401);return
 }
 ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 rows,err:=h.DB.QueryContext(ctx,`SELECT r.code,vt.name,r.start_time,r.end_time,r.status
 FROM reservations r JOIN vehicle_types vt ON vt.id=r.vehicle_type_id
 WHERE r.status IN ('active','pending')
 AND r.end_time > NOW() AND r.start_time < NOW()+INTERVAL '14 days'
 AND (r.status='active' OR (r.status='pending' AND r.payment_method_id=2
 AND r.created_at > NOW()-INTERVAL '45 minutes'))
 ORDER BY r.start_time ASC LIMIT 250`)
 if err!=nil { http.Error(w,"Unable to read reservations",503);return }
 defer rows.Close()
 results:=make([]managerOnlineReservation,0)
 for rows.Next() {
  var v managerOnlineReservation
  if err:=rows.Scan(&v.Code,&v.VehicleType,&v.StartTime,&v.EndTime,&v.Status);err!=nil {
   http.Error(w,"Unable to read reservations",503);return
  }
  results=append(results,v)
 }
 if err:=rows.Err();err!=nil {http.Error(w,"Unable to read reservations",503);return}
 w.Header().Set("Content-Type","application/json");w.Header().Set("Cache-Control","no-store")
 _=json.NewEncoder(w).Encode(map[string]interface{}{"reservations":results})
}

func InitManagerOccupancy(ctx context.Context,db *sql.DB) error {
 if db==nil {return errors.New("nil database")}
 _,err:=db.ExecContext(ctx,`
  CREATE TABLE IF NOT EXISTS public.manager_active_stays (
   installation_id TEXT NOT NULL, local_stay_id TEXT NOT NULL,
   vehicle_type TEXT NOT NULL CHECK (vehicle_type IN ('car','sedan','suv','motorcycle','electric_bicycle')),
   arrival_local TIMESTAMP WITHOUT TIME ZONE NOT NULL,
   booking_code TEXT NOT NULL DEFAULT '',
   PRIMARY KEY (installation_id,local_stay_id)
  );
  CREATE TABLE IF NOT EXISTS public.manager_sync_status (
   installation_id TEXT PRIMARY KEY, checksum TEXT NOT NULL,
   last_sync_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),active_stays INTEGER NOT NULL DEFAULT 0
  );
 `)
 if err!=nil {log.Printf("Manager occupancy schema initialization error: %v",err)}
 return err
}
