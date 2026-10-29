package db

import (
	"context"
	"database/sql"
	_ "embed"
)

//go:embed migrations/20261028_site_content.sql
var siteContentMigration string

//go:embed migrations/20261028_luggage_requests.sql
var luggageRequestsMigration string

//go:embed migrations/20261029_vehicle_categories.sql
var vehicleCategoriesMigration string

func MigrateVehicleCategories(ctx context.Context, database *sql.DB) error {
 tx, err := database.BeginTx(ctx, nil)
 if err != nil { return err }
 defer tx.Rollback()
 if _, err := tx.ExecContext(ctx, vehicleCategoriesMigration); err != nil { return err }
 return tx.Commit()
}

// MigrateSiteContent commits only if both the migration and schema checks succeed.
func MigrateSiteContent(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, siteContentMigration); err != nil {
		return err
	}
	return tx.Commit()
}

func MigrateLuggageRequests(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, luggageRequestsMigration); err != nil {
		return err
	}
	return tx.Commit()
}
