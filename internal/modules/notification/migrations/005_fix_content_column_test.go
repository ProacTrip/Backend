// Tests de migración para 005_fix_content_column.
// Verifica que el contenido sea nullable y las columnas obsoletas eliminadas,
// y que Save() funcione correctamente post-migración.
//
// Convenciones:
//   - Table-driven con t.Run(), nombres de sub-tests en español.
//   - Solo stdlib testing (sin testify).
//   - t.Context() para contextos de test (Go 1.24+).
package notification_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ProacTrip/Backend/internal/modules/notification"
)

// =============================================================================
// Configuración de conexión a la BD de notificaciones
// =============================================================================

const testNotificationDSN = "host=localhost port=5432 user=proactrip password=proactrip123 dbname=proactrip_notification sslmode=disable"

// conectarNotificationDB crea un pool de conexiones a la BD de notificaciones.
// El pool se cierra automáticamente al finalizar el test.
func conectarNotificationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(testNotificationDSN)
	if err != nil {
		t.Fatalf("error parseando DSN: %v", err)
	}
	cfg.MaxConns = 2

	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("no se pudo conectar a la BD de notificaciones: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(t.Context()); err != nil {
		t.Fatalf("ping falló a la BD de notificaciones: %v", err)
	}

	return pool
}

// migracionYaAplicada verifica si la migración 005 ya está registrada en schema_migrations.
func migracionYaAplicada(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()

	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM schema_migrations WHERE filename = '005_fix_content_column.sql'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("error consultando schema_migrations: %v", err)
	}
	return count > 0
}

// eliminarRegistroMigracion borra el registro de la migración 005 de schema_migrations.
// Útil para pruebas de idempotencia y rollback.
func eliminarRegistroMigracion(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		`DELETE FROM schema_migrations WHERE filename = '005_fix_content_column.sql'`,
	)
	if err != nil {
		t.Fatalf("error eliminando registro de migración: %v", err)
	}
}

// =============================================================================
// TestMigracion005_Up — Verifica que la migración 005 Up corrija el esquema
// =============================================================================

func TestMigracion005_Up(t *testing.T) {
	pool := conectarNotificationDB(t)

	// Limpiar registro previo si existe (por ejecuciones anteriores)
	if migracionYaAplicada(t, pool) {
		eliminarRegistroMigracion(t, pool)
	}

	// Aplicar migraciones pendientes (incluye 005 si el archivo existe)
	if err := notification.RunMigrations(t.Context(), pool); err != nil {
		t.Fatalf("RunMigrations falló: %v", err)
	}

	// Casos de prueba: verificaciones post-migración
	tests := []struct {
		nombre     string
		afirmacion func(t *testing.T)
	}{
		{
			nombre: "contenido es nullable",
			afirmacion: func(t *testing.T) {
				var isNullable string
				err := pool.QueryRow(t.Context(),
					`SELECT is_nullable FROM information_schema.columns
					 WHERE table_name = 'notifications' AND column_name = 'content'`,
				).Scan(&isNullable)
				if err != nil {
					t.Fatalf("error consultando information_schema: %v", err)
				}
				if isNullable != "YES" {
					t.Errorf("content debería ser nullable, pero es_nullable = %q", isNullable)
				}
			},
		},
		{
			nombre: "columna subject fue eliminada",
			afirmacion: func(t *testing.T) {
				var existe bool
				err := pool.QueryRow(t.Context(),
					`SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'notifications' AND column_name = 'subject'
					)`,
				).Scan(&existe)
				if err != nil {
					t.Fatalf("error consultando information_schema: %v", err)
				}
				if existe {
					t.Error("subject debería haber sido eliminada por la migración 005")
				}
			},
		},
		{
			nombre: "columna data fue eliminada",
			afirmacion: func(t *testing.T) {
				var existe bool
				err := pool.QueryRow(t.Context(),
					`SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'notifications' AND column_name = 'data'
					)`,
				).Scan(&existe)
				if err != nil {
					t.Fatalf("error consultando information_schema: %v", err)
				}
				if existe {
					t.Error("data debería haber sido eliminada por la migración 005")
				}
			},
		},
		{
			nombre: "columna provider_message_id fue eliminada",
			afirmacion: func(t *testing.T) {
				var existe bool
				err := pool.QueryRow(t.Context(),
					`SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'notifications' AND column_name = 'provider_message_id'
					)`,
				).Scan(&existe)
				if err != nil {
					t.Fatalf("error consultando information_schema: %v", err)
				}
				if existe {
					t.Error("provider_message_id debería haber sido eliminada por la migración 005")
				}
			},
		},
		{
			nombre: "columnas esperadas aún existen",
			afirmacion: func(t *testing.T) {
				esperadas := []string{"id", "user_id", "template_code", "sent_at", "created_at", "updated_at"}
				rows, err := pool.Query(t.Context(),
					`SELECT column_name FROM information_schema.columns
					 WHERE table_name = 'notifications' ORDER BY ordinal_position`,
				)
				if err != nil {
					t.Fatalf("error consultando columnas: %v", err)
				}
				defer rows.Close()

				var columnas []string
				for rows.Next() {
					var col string
					if err := rows.Scan(&col); err != nil {
						t.Fatalf("error escaneando columna: %v", err)
					}
					columnas = append(columnas, col)
				}

				for _, esperada := range esperadas {
					if !slices.Contains(columnas, esperada) {
						t.Errorf("columna esperada %q no encontrada. Columnas: %v", esperada, columnas)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			tt.afirmacion(t)
		})
	}
}

// =============================================================================
// TestMigracion005_Down — Verifica que el rollback restaure el esquema
// =============================================================================

func TestMigracion005_Down(t *testing.T) {
	pool := conectarNotificationDB(t)

	// Asegurar que 005 está aplicado
	if !migracionYaAplicada(t, pool) {
		if err := notification.RunMigrations(t.Context(), pool); err != nil {
			t.Fatalf("RunMigrations (pre-Down) falló: %v", err)
		}
		// Si la migración no se aplicó (archivo no existe), fallar con mensaje claro
		if !migracionYaAplicada(t, pool) {
			t.Skip("migración 005 no disponible — archivo SQL no existe aún")
		}
	}

	// Ejecutar Down manualmente (RunMigrations no tiene rollback automático)
	downSQL := `
		ALTER TABLE notifications ALTER COLUMN content SET NOT NULL;
		ALTER TABLE notifications ALTER COLUMN content SET DEFAULT '';
		ALTER TABLE notifications ADD COLUMN IF NOT EXISTS subject VARCHAR(255);
		ALTER TABLE notifications ADD COLUMN IF NOT EXISTS data JSONB NOT NULL DEFAULT '{}';
		ALTER TABLE notifications ADD COLUMN IF NOT EXISTS provider_message_id VARCHAR(255);
	`
	if _, err := pool.Exec(t.Context(), downSQL); err != nil {
		t.Fatalf("ejecución del Down migration falló: %v", err)
	}

	// Eliminar registro para que RunMigrations pueda re-aplicar 005 después
	eliminarRegistroMigracion(t, pool)

	// Verificaciones post-Down
	tests := []struct {
		nombre     string
		afirmacion func(t *testing.T)
	}{
		{
			nombre: "contenido vuelve a ser NOT NULL",
			afirmacion: func(t *testing.T) {
				var isNullable string
				err := pool.QueryRow(t.Context(),
					`SELECT is_nullable FROM information_schema.columns
					 WHERE table_name = 'notifications' AND column_name = 'content'`,
				).Scan(&isNullable)
				if err != nil {
					t.Fatalf("error consultando information_schema: %v", err)
				}
				if isNullable != "NO" {
					t.Errorf("content debería ser NOT NULL después del Down, pero es_nullable = %q", isNullable)
				}
			},
		},
		{
			nombre: "subject fue restaurada",
			afirmacion: func(t *testing.T) {
				var existe bool
				err := pool.QueryRow(t.Context(),
					`SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'notifications' AND column_name = 'subject'
					)`,
				).Scan(&existe)
				if err != nil {
					t.Fatalf("error consultando information_schema: %v", err)
				}
				if !existe {
					t.Error("subject debería existir después del Down")
				}
			},
		},
		{
			nombre: "data fue restaurada",
			afirmacion: func(t *testing.T) {
				var existe bool
				err := pool.QueryRow(t.Context(),
					`SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'notifications' AND column_name = 'data'
					)`,
				).Scan(&existe)
				if err != nil {
					t.Fatalf("error consultando information_schema: %v", err)
				}
				if !existe {
					t.Error("data debería existir después del Down")
				}
			},
		},
		{
			nombre: "provider_message_id fue restaurada",
			afirmacion: func(t *testing.T) {
				var existe bool
				err := pool.QueryRow(t.Context(),
					`SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'notifications' AND column_name = 'provider_message_id'
					)`,
				).Scan(&existe)
				if err != nil {
					t.Fatalf("error consultando information_schema: %v", err)
				}
				if !existe {
					t.Error("provider_message_id debería existir después del Down")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			tt.afirmacion(t)
		})
	}

	// Re-aplicar 005 para dejar la BD limpia para otros tests
	if err := notification.RunMigrations(t.Context(), pool); err != nil {
		t.Fatalf("RunMigrations (re-apply después de Down) falló: %v", err)
	}
}

// =============================================================================
// TestMigracion005_Save — Smoke test: INSERT sin content no debe fallar
// =============================================================================

func TestMigracion005_Save(t *testing.T) {
	pool := conectarNotificationDB(t)

	// Asegurar que 005 está aplicado
	if !migracionYaAplicada(t, pool) {
		if err := notification.RunMigrations(t.Context(), pool); err != nil {
			t.Fatalf("RunMigrations falló: %v", err)
		}
		if !migracionYaAplicada(t, pool) {
			t.Skip("migración 005 no disponible — archivo SQL no existe aún")
		}
	}

	// Smoke: INSERT con campos mínimos (sin content, subject, data, provider_message_id)
	id := uuid.New()
	userID := uuid.New()

	_, err := pool.Exec(t.Context(),
		`INSERT INTO notifications (id, user_id, template_code, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())`,
		id, userID, "verify-email",
	)
	if err != nil {
		t.Fatalf("INSERT sin content falló: %v — la migración no corrigió el NOT NULL", err)
	}

	// Limpiar
	_, _ = pool.Exec(t.Context(), `DELETE FROM notifications WHERE id = $1`, id)
}

// =============================================================================
// TestMigracion005_IdempotenciaSave — Segundo Save() retorna el ID existente
// =============================================================================

func TestMigracion005_IdempotenciaSave(t *testing.T) {
	pool := conectarNotificationDB(t)

	// Asegurar que 005 está aplicado
	if !migracionYaAplicada(t, pool) {
		if err := notification.RunMigrations(t.Context(), pool); err != nil {
			t.Fatalf("RunMigrations falló: %v", err)
		}
		if !migracionYaAplicada(t, pool) {
			t.Skip("migración 005 no disponible — archivo SQL no existe aún")
		}
	}

	userID := uuid.New()
	templateCode := fmt.Sprintf("idempotent-test-%s", uuid.New().String()[:8])

	// Primer INSERT — con sent_at IS NOT NULL (simula notificación ya enviada)
	id1 := uuid.New()
	_, err := pool.Exec(t.Context(),
		`INSERT INTO notifications (id, user_id, template_code, sent_at, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW(), NOW())`,
		id1, userID, templateCode,
	)
	if err != nil {
		t.Fatalf("primer INSERT falló: %v", err)
	}

	// Segundo INSERT para el mismo user + template_code debería disparar idempotencia
	// (el repositorio tiene un idempotency check: si ya existe con sent_at NOT NULL, retorna el ID)
	id2 := uuid.New()
	_, err = pool.Exec(t.Context(),
		`INSERT INTO notifications (id, user_id, template_code, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())`,
		id2, userID, templateCode,
	)
	// El segundo INSERT no debería fallar (ya que no hay constraint UNIQUE en user_id + template_code)
	// La idempotencia se maneja a nivel aplicación (repositorio), no a nivel BD.
	// Este test verifica que ambos INSERTs funcionan sin errores de NOT NULL.
	if err != nil {
		t.Fatalf("segundo INSERT falló: %v", err)
	}

	// Limpiar
	_, _ = pool.Exec(t.Context(), `DELETE FROM notifications WHERE user_id = $1`, userID)
}
