// Migration runner para el módulo user.
// Delega en shared/database.RunMigrations usando el embed.FS del módulo.
package user

import (
	"context"
	"embed"

	"github.com/ProacTrip/Backend/internal/shared/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// RunMigrations ejecuta las migraciones pendientes del módulo user.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	return database.RunMigrations(ctx, pool, migrationFiles, "user")
}
