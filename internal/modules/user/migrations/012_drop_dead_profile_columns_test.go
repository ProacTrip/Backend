// Tests de migración para 012_drop_dead_profile_columns.
// Verifica que las columnas muertas (is_public, timezone_name, phone_verified,
// current_location) sean eliminadas de user_profiles.
//
// Convenciones:
//   - Table-driven con t.Run(), nombres de sub-tests en español.
//   - Solo stdlib testing (sin testify).
//   - t.Context() para contextos de test (Go 1.24+).
package user_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/user"
)

// =============================================================================
// Configuración de conexión a la BD de usuarios
// =============================================================================

const testUserDSN = "host=localhost port=5432 user=proactrip password=proactrip123 dbname=proactrip_user sslmode=disable"

// conectarUserDB crea un pool de conexiones a la BD de usuarios.
// El pool se cierra automáticamente al finalizar el test.
func conectarUserDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(testUserDSN)
	if err != nil {
		t.Fatalf("error parseando DSN: %v", err)
	}
	cfg.MaxConns = 2

	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("no se pudo conectar a la BD de usuarios: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(t.Context()); err != nil {
		t.Fatalf("ping falló a la BD de usuarios: %v", err)
	}

	return pool
}

// migracion012YaAplicada verifica si la migración 012 ya está registrada en schema_migrations.
func migracion012YaAplicada(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()

	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM schema_migrations WHERE filename = '012_drop_dead_profile_columns.sql'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("error consultando schema_migrations: %v", err)
	}
	return count > 0
}

// eliminarRegistroMigracion012 borra el registro de la migración 012 de schema_migrations.
func eliminarRegistroMigracion012(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		`DELETE FROM schema_migrations WHERE filename = '012_drop_dead_profile_columns.sql'`,
	)
	if err != nil {
		t.Fatalf("error eliminando registro de migración: %v", err)
	}
}

// columnaExiste verifica si una columna existe en la tabla user_profiles.
func columnaExiste(t *testing.T, pool *pgxpool.Pool, columnName string) bool {
	t.Helper()

	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_name = 'user_profiles' AND column_name = $1`,
		columnName,
	).Scan(&count)
	if err != nil {
		t.Fatalf("error consultando information_schema para columna %s: %v", columnName, err)
	}
	return count > 0
}

// =============================================================================
// TestMigracion012_DropDeadColumns — Verifica que las columnas muertas desaparezcan
// =============================================================================

func TestMigracion012_DropDeadColumns(t *testing.T) {
	pool := conectarUserDB(t)

	// Limpiar registro previo si existe (por ejecuciones anteriores)
	if migracion012YaAplicada(t, pool) {
		eliminarRegistroMigracion012(t, pool)
	}

	// Aplicar migraciones pendientes (incluye 012)
	if err := user.RunMigrations(t.Context(), pool); err != nil {
		t.Fatalf("RunMigrations falló: %v", err)
	}

	// Casos de prueba: verificar que las columnas muertas NO existan
	tests := []struct {
		nombre     string
		columna    string
		debeExistir bool
	}{
		{"is_public fue eliminada", "is_public", false},
		{"timezone_name fue eliminada", "timezone_name", false},
		{"phone_verified fue eliminada", "phone_verified", false},
		{"current_location fue eliminada", "current_location", false},
		// Sanity checks: columnas que SÍ deben existir
		{"first_name sigue existiendo", "first_name", true},
		{"language_code sigue existiendo", "language_code", true},
		{"currency_code sigue existiendo", "currency_code", true},
	}

	for _, tc := range tests {
		t.Run(tc.nombre, func(t *testing.T) {
			existe := columnaExiste(t, pool, tc.columna)
			if tc.debeExistir && !existe {
				t.Errorf("la columna %s debería existir pero no se encontró", tc.columna)
			}
			if !tc.debeExistir && existe {
				t.Errorf("la columna %s NO debería existir pero aún está presente", tc.columna)
			}
		})
	}
}

// =============================================================================
// TestMigracion012_Idempotencia — Ejecutar la migración dos veces no debe fallar
// =============================================================================

func TestMigracion012_Idempotencia(t *testing.T) {
	pool := conectarUserDB(t)

	if migracion012YaAplicada(t, pool) {
		eliminarRegistroMigracion012(t, pool)
	}

	// Primera ejecución
	if err := user.RunMigrations(t.Context(), pool); err != nil {
		t.Fatalf("RunMigrations (1ra ejecución) falló: %v", err)
	}

	// Segunda ejecución: debe ser idempotente (DROP COLUMN IF EXISTS no falla)
	if err := user.RunMigrations(t.Context(), pool); err != nil {
		t.Fatalf("RunMigrations (2da ejecución, idempotencia) falló: %v", err)
	}

	t.Log("migración 012 es idempotente — segunda ejecución no falló")
}
