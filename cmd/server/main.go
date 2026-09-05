package main

import (
	"database/sql"
	"estacionamienti/internal/api"
	"estacionamienti/internal/auth"
	"estacionamienti/internal/repository"
	"estacionamienti/internal/service"
	"github.com/stripe/stripe-go/v82"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/handlers"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/robfig/cron/v3"
)

func initStripe() {
	stripeSecretKey := os.Getenv("STRIPE_SECRET_KEY")
	if stripeSecretKey == "" {
		log.Fatal("STRIPE_SECRET_KEY no está configurada.")
	}
	stripe.Key = stripeSecretKey
}

// setupDeletePendingReservationsCron schedules the cron job to delete old pending reservations at 1am Italy time.
func setupDeletePendingReservationsCron(jobSvc *service.JobService) *cron.Cron {
	c := cron.New(cron.WithLocation(time.FixedZone("CET", 3600))) // Italy time (CET/CEST)
	_, err := c.AddFunc("0 1 * * *", func() {
		oneDayAgo := time.Now().Add(-24 * time.Hour)
		log.Println("Executing scheduled task: Delete old pending reservations (Italy 1am)")
		rows, err := jobSvc.DeleteOldPendingReservations(oneDayAgo)
		if err != nil {
			log.Printf("Error deleting old pending reservations: %v", err)
		} else {
			log.Printf("Deleted %d old pending reservations", rows)
		}
	})
	if err != nil {
		log.Fatalf("Failed to add cron job: %v", err)
	}
	c.Start()
	log.Println("DeletePendingReservations cron scheduler started.")
	return c
}

// setupUpdateFinishedReservationsCron schedules the cron job to update finished reservations every hour.
func setupUpdateFinishedReservationsCron(jobSvc *service.JobService) *cron.Cron {
	c := cron.New(cron.WithLocation(time.UTC))
	_, err := c.AddFunc("@hourly", func() {
		log.Println("Executing scheduled task: Update Finished Reservations")
		if err := jobSvc.UpdateFinishedReservations(); err != nil {
			log.Printf("Error during scheduled task: UpdateFinishedReservations: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Failed to add cron job: %v", err)
	}
	c.Start()
	log.Println("UpdateFinishedReservations cron scheduler started.")
	return c
}

func main() {
	if os.Getenv("RAILWAY_ENVIRONMENT") == "" {
		err := godotenv.Load()
		if err != nil {
			log.Println("No .env file found")
		}
	}


	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}
	safeURL := dbURL
	if len(dbURL) > 30 { 
		safeURL = dbURL[:15] + "...[oculto]..." + dbURL[len(dbURL)-40:]
	}
	log.Printf("Intentando conectar a DB: %s", safeURL)

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}

	initStripe()

	// Repositories
	reservationRepo := repository.NewReservationRepository(db)
	jobRepo := repository.NewJobRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	adminAuthRepo := repository.NewAdminAuthRepository(db)

	// Services
	senderService := service.NewSenderService()
	stripeSvc := service.NewStripeService(reservationRepo)
	reservationSvc := service.NewReservationService(reservationRepo, stripeSvc, senderService)
	jobSvc := service.NewJobService(jobRepo)
	adminSvc := service.NewAdminService(adminRepo, reservationRepo, stripeSvc, senderService)
	adminAuthSvc := service.NewAdminAuthService(adminAuthRepo)

	// Handlers
	userReservationHandler := api.NewUserReservationHandler(reservationSvc)
	adminHandler := api.NewAdminHandler(adminSvc)
	adminAuthHandler := api.NewAdminAuthHandler(adminAuthSvc)
	stripeHandler := api.NewStripeWebhookHandler(os.Getenv("STRIPE_WEBHOOK_SECRET"), reservationSvc, senderService)

	// Cron scheduler setup
	_ = setupDeletePendingReservationsCron(jobSvc)
	_ = setupUpdateFinishedReservationsCron(jobSvc)

	r := mux.NewRouter()

	// Public endpoints
	r.HandleFunc("/api/prices", userReservationHandler.GetPrices).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/vehicle-types", userReservationHandler.GetVehicleTypes).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/availability", userReservationHandler.CheckAvailability).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/total-price", userReservationHandler.GetTotalPriceForReservation).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/reservations", userReservationHandler.CreateReservation).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/reservations/{code}", userReservationHandler.GetReservation).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/reservation/by-session", stripeHandler.GetReservationBySessionIDHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/reservations/{code}", userReservationHandler.CancelReservation).Methods("DELETE", "OPTIONS")

	// Admin login
	r.HandleFunc("/api/login", adminAuthHandler.CreateUserAdmin).Methods("POST", "OPTIONS")
	r.HandleFunc("/admin/login", adminAuthHandler.Login).Methods("POST", "OPTIONS")

	// Admin endpoints (protected)
	adminRouter := r.PathPrefix("/admin").Subrouter()
	adminRouter.Use(auth.AdminAuthMiddleware)
	adminRouter.HandleFunc("/reservations", adminHandler.ListReservations).Methods("GET", "OPTIONS")
	adminRouter.HandleFunc("/reservations", adminHandler.CreateReservation).Methods("POST", "OPTIONS")
	adminRouter.HandleFunc("/reservations/{code}", adminHandler.AdminDeleteReservation).Methods("DELETE", "OPTIONS")
	adminRouter.HandleFunc("/vehicle-config", adminHandler.ListVehicleSpaces).Methods("GET", "OPTIONS")
	adminRouter.HandleFunc("/vehicle-config/{vehicle_type}", adminHandler.UpdateVehicleSpaces).Methods("PUT", "OPTIONS")

	// Stripe
	r.HandleFunc("/webhook/stripe", stripeHandler.HandleWebhook).Methods("POST", "OPTIONS")

	frontendURL := os.Getenv("FRONTEND_URL")
	allowedOrigins := handlers.AllowedOrigins([]string{
		frontendURL,
	})
	allowedMethods := handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	allowedHeaders := handlers.AllowedHeaders([]string{"Content-Type", "Authorization", "X-Requested-With"})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server running on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, handlers.CORS(allowedOrigins, allowedMethods, allowedHeaders)(r)))
}
