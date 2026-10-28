package db

import (
	"context"
	"database/sql"
	_ "embed"
)

//go:embed migrations/20261028_site_content.sql
var siteContentMigration string

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
