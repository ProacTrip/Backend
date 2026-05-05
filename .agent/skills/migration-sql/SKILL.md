# migration-sql

## Trigger

Generate PostgreSQL migrations for Proactrip modules. Activate when user mentions:
- Creating a new database table, migration, or schema
- Adding columns, constraints, or indexes to existing tables
- Seeding reference data
- "Crear migración", "nueva tabla", "agregar columna", "ALTER TABLE", "seed data"
- Any `CREATE TABLE`, `ALTER TABLE`, `CREATE INDEX`, or `INSERT INTO` request in the Proactrip context

## Questions to Ask (ALWAYS ask first — never generate SQL without answers)

1. **¿Cuál es el nombre del módulo y qué entidades necesitan tablas?**
   - Module name (lowercase, e.g., `booking`) and entity names (PascalCase, e.g., `Booking`)
   - Determines file path: `internal/modules/{modulo}/migrations/NNN_description.sql`

2. **¿Qué columnas necesita cada tabla?** (nombre, tipo SQL, constraints)
   - Column name, PostgreSQL type (`uuid`, `varchar(N)`, `text`, `integer`, `boolean`, `timestamptz`, `jsonb`, `inet`), and constraints (`NOT NULL`, `UNIQUE`, `DEFAULT`)
   - ¿Hay columnas con valores enumerados? (status fields needing CHECK constraints)

3. **¿Hay relaciones entre tablas?** (foreign keys, ON DELETE behavior)
   - Does this table reference another table's `id`?
   - What happens on delete: `CASCADE`, `SET NULL`, `RESTRICT`?
   - Is the FK to a table in THIS module or ANOTHER module?

4. **¿Necesita datos semilla? ¿Qué valores?**
   - Reference/lookup tables (e.g., statuses, types, roles) need seed data
   - What are the fixed values that should exist at deploy time?

5. **¿Es una tabla nueva o estás modificando una existente?** (ALTER TABLE)
   - NEW table → `NNN_description.sql` (next number in sequence)
   - MODIFYING existing → `NNN_description.sql` with ALTER statements (NEVER edit old migration files)
   - ¿Qué operación: ADD COLUMN, ADD CONSTRAINT, o CREATE INDEX?

## Rules (Non-Negotiable — fail if violated)

### Specific Rules (M1-M6)

| # | Rule | Severity |
|---|------|----------|
| M1 | All tables MUST have `id uuid PRIMARY KEY DEFAULT uuidv7()` | CRITICAL |
| M2 | All tables MUST have `created_at timestamptz DEFAULT CURRENT_TIMESTAMP` and `updated_at timestamptz DEFAULT CURRENT_TIMESTAMP` with trigger | CRITICAL |
| M3 | CHECK constraints MUST validate status enums, format patterns | MUST |
| M4 | Indexes created AFTER table definitions; partial indexes with `WHERE x IS NOT NULL` | MUST |
| M5 | Seed data uses `INSERT INTO ... VALUES (...)` | MUST |
| M6 | Migration numbered `NNN` following existing sequence in module directory | MUST |

### Global Architecture Rules (R1-R9)

| # | Rule | Severity |
|---|------|----------|
| R1 | Modules communicate only via injected interfaces or published events — consider FK boundaries | MUST |
| R2 | NEVER reference another module's `features/` or `adapters/` in SQL | MUST NOT |
| R3 | `shared/` packages MUST NOT import from `modules/` | MUST NOT |
| R4 | Domain errors → `RegisterDomainErrorMapper()` → RFC 9457 Problem JSON | MUST |
| R5 | Manual constructor injection, zero globals, zero singletons | MUST |
| R6 | Always generate `_test.go` alongside code | MUST |
| R7 | Go 1.26 patterns: `omitzero`, `new(expr)`, `errors.AsType`, `uuid.Must(uuid.NewV7())` | MUST |
| R8 | Echo v5: `*echo.Context` pointer, `echo.StartConfig`, `echo.PathParam[T]()` | MUST |
| R9 | Adapter files named after technology (`echo.go`, `paseto.go`, `resend.go`, `blake3.go`) | MUST |

### Critical Anti-Patterns

| # | Do NOT | Do INSTEAD |
|---|--------|------------|
| A1 | Use `SERIAL` or `BIGSERIAL` for PKs | Use `uuid PRIMARY KEY DEFAULT uuidv7()` |
| A2 | Omit `updated_at` or its trigger | Always include both, call `update_updated_at_column()` |
| A3 | Modify an existing migration file | Create a NEW migration with ALTER statements |
| A4 | Create indexes inside CREATE TABLE | Place indexes AFTER the table definition |
| A5 | Hardcode UUIDs in seed data | Use `uuidv7()` function to generate IDs |
| A6 | Use English comments | Write comments in Spanish |

## Patterns

Real patterns extracted from the Proactrip codebase (`internal/modules/auth/migrations/001_initial.sql` and `internal/modules/notification/migrations/`).

### Pattern 1: Full Table with UUIDv7, CHECKs, FK, Trigger, Indexes

```sql
-- =============================================================================
-- USUARIOS
-- =============================================================================
CREATE TABLE IF NOT EXISTS users(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    email varchar(255) NOT NULL UNIQUE,
    email_verified boolean DEFAULT FALSE,
    email_verified_at timestamptz,
    password_hash varchar(255),
    status varchar(50) NOT NULL DEFAULT 'pending_verification',
    role_id uuid NOT NULL REFERENCES roles(id),
    last_login_at timestamptz,
    login_count integer DEFAULT 0,
    failed_login_attempts integer DEFAULT 0,
    locked_until timestamptz,
    mfa_enabled boolean DEFAULT FALSE,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_user_status CHECK (status IN ('active', 'inactive', 'suspended', 'pending_verification', 'locked')),
    CONSTRAINT chk_email_format CHECK (email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$')
);

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_users_role_id      ON users(role_id);
CREATE INDEX idx_users_status       ON users(status);
CREATE INDEX idx_users_locked_until ON users(locked_until) WHERE locked_until IS NOT NULL;
```

### Pattern 2: Junction Table (composite PK, dual FKs, no UUID PK)

```sql
-- =============================================================================
-- RELACIÓN ROLES ↔ PERMISOS
-- =============================================================================
CREATE TABLE IF NOT EXISTS role_permissions(
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TRIGGER update_role_permissions_updated_at
    BEFORE UPDATE ON role_permissions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_role_permissions_permission_id ON role_permissions(permission_id);
```

### Pattern 3: Seed Data with uuidv7() + Reference Lookup

```sql
-- Seed: admin user with verified email and MFA
INSERT INTO users (
    id, email, email_verified, email_verified_at, password_hash,
    status, role_id, mfa_enabled, failed_login_attempts, created_at, updated_at
)
SELECT
    uuidv7(),
    'admin@proactrip.com',
    true,
    NOW(),
    '$argon2id$v=19$m=65536,t=3,p=4$...',
    'active',
    (SELECT id FROM roles WHERE name = 'admin' LIMIT 1),
    true,
    0,
    NOW(),
    NOW()
ON CONFLICT (email) DO NOTHING;
```

### Pattern 4: Simple Seed for Reference Tables

```sql
INSERT INTO token_types(code, description, ttl_seconds)
    VALUES
        ('access', 'Token de acceso para API', 3600),
        ('refresh', 'Token para renovar access tokens', 2592000),
        ('email_verification', 'Token para verificación de email', 86400),
        ('password_reset', 'Token para reset de contraseña', 3600);
```

### Pattern 5: ALTER TABLE (from existing migration 002)

```sql
-- =============================================================================
-- Foreign key constraints on user_id → auth.users
-- =============================================================================
ALTER TABLE notifications
    ADD CONSTRAINT IF NOT EXISTS fk_notifications_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE notification_reads
    ADD CONSTRAINT IF NOT EXISTS fk_notification_reads_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
```

### Pattern 6: Partial Index for Query Optimization

```sql
CREATE INDEX IF NOT EXISTS idx_notifications_failed_retry
    ON notifications(status, created_at)
    WHERE status = 'failed';
```

### Pattern 7: Multi-column Active Filter Index

```sql
CREATE INDEX idx_user_tokens_active ON user_tokens(user_id, expires_at)
    WHERE revoked_at IS NULL;
```

### Naming Conventions

| Element | Convention | Example |
|---------|-----------|---------|
| Table | snake_case, plural | `users`, `token_types`, `role_permissions` |
| PK column | `id uuid` | `id uuid PRIMARY KEY DEFAULT uuidv7()` |
| FK column | `{entity}_id uuid` | `user_id uuid NOT NULL REFERENCES users(id)` |
| Trigger | `trg_{table}_updated_at` | `trg_users_updated_at` |
| CHECK constraint | `chk_{table}_{field}` | `chk_user_status`, `chk_email_format` |
| UNIQUE constraint | `uq_{table}_{field(s)}` | `uq_permission_resource_action`, `uq_user_provider` |
| FK constraint | `fk_{table}_{ref}` | `fk_notifications_user` |
| Index | `idx_{table}_{column(s)}` | `idx_users_role_id`, `idx_user_tokens_active` |
| Migration file | `NNN_description.sql` | `001_initial.sql`, `002_add_fks_and_indexes.sql` |

### Trigger Function (Shared — redefined in first migration per module)

```sql
CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER
    AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$
LANGUAGE plpgsql;
```

This function is assumed to already exist. Include it in the module's FIRST migration (001). Subsequent migrations in the same module reference it without redefining.

## Output

| File | Template | Where |
|------|----------|-------|
| `NNN_description.sql` | New Table | `internal/modules/{modulo}/migrations/` |
| `NNN_description.sql` | ALTER TABLE | Same directory, NEXT number in sequence |
| `NNN_description.sql` | Seeder + Table | Same directory (combined: table + seed inline) |

## Templates

### Template 1: New Table (complete)

Use this for creating a new table. Replace all `{{.Placeholder}}` values before writing.

```sql
-- +migrate Up
-- =============================================================================
-- {{.TableComment}}
-- =============================================================================
CREATE TABLE IF NOT EXISTS {{.TableName}}(
    id uuid PRIMARY KEY DEFAULT uuidv7(),
{{.Columns}}
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP{{.TrailingComma}}
{{.CheckConstraints}}
{{.UniqueConstraints}}
);

CREATE TRIGGER trg_{{.TableName}}_updated_at
    BEFORE UPDATE ON {{.TableName}}
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

{{.Indexes}}
```

**Placeholders**:
- `{{.TableComment}}` — Description in Spanish, e.g., `RESERVAS DE VUELOS`
- `{{.TableName}}` — snake_case, e.g., `bookings`
- `{{.Columns}}` — Indented column definitions, one per line, e.g.:
  ```
      user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      origin varchar(3) NOT NULL,
      destination varchar(3) NOT NULL,
      status varchar(20) NOT NULL DEFAULT 'pending',
  ```
- `{{.TrailingComma}}` — `,` if there are CHECK or UNIQUE constraints after columns, empty otherwise
- `{{.CheckConstraints}}` — If any, e.g.:
  ```
      CONSTRAINT chk_booking_status CHECK (status IN ('pending', 'confirmed', 'cancelled', 'completed')),
  ```
- `{{.UniqueConstraints}}` — If any, e.g.:
  ```
      CONSTRAINT uq_booking_ref UNIQUE (user_id, external_ref),
  ```
- `{{.Indexes}}` — One per line after the trigger, e.g.:
  ```sql
  CREATE INDEX idx_bookings_user_id ON bookings(user_id);
  CREATE INDEX idx_bookings_status  ON bookings(status) WHERE status IN ('pending', 'confirmed');
  CREATE INDEX idx_bookings_origin  ON bookings(origin, created_at DESC);
  ```

### Template 2: ALTER TABLE (add column)

Use this to add a column to an existing table. ALWAYS creates a NEW migration file — never modify an existing one.

```sql
-- +migrate Up
-- =============================================================================
-- {{.Description}}
-- =============================================================================
ALTER TABLE {{.TableName}}
    ADD COLUMN IF NOT EXISTS {{.ColumnName}} {{.ColumnType}}{{.ColumnConstraints}};
```

**Placeholders**:
- `{{.Description}}` — What and why, in Spanish, e.g., `Agregar columna cancelled_at para registrar cancelaciones`
- `{{.TableName}}` — Existing table name, e.g., `bookings`
- `{{.ColumnName}}` — New column name, e.g., `cancelled_at`
- `{{.ColumnType}}` — PostgreSQL type, e.g., `timestamptz`, `varchar(50)`, `text`
- `{{.ColumnConstraints}}` — e.g., ` DEFAULT NULL`, ` NOT NULL DEFAULT 'pending'`, ` REFERENCES users(id)`

### Template 3: ALTER TABLE (add constraint or FK)

```sql
-- +migrate Up
-- =============================================================================
-- {{.Description}}
-- =============================================================================
ALTER TABLE {{.TableName}}
    ADD CONSTRAINT IF NOT EXISTS {{.ConstraintName}}
    {{.ConstraintDefinition}};
```

**Placeholders**:
- `{{.ConstraintName}}` — `fk_{table}_{ref}`, `chk_{table}_{field}`, or `uq_{table}_{fields}`
- `{{.ConstraintDefinition}}` — e.g., `FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE` or `CHECK (status IN ('pending', 'archived'))`

### Template 4: Seed Data (INSERT)

```sql
-- =============================================================================
-- SEED: {{.SeedDescription}}
-- =============================================================================
INSERT INTO {{.TableName}}({{.SeedColumns}})
    VALUES
{{.SeedValues}};
```

**Placeholders**:
- `{{.SeedDescription}}` — What is being seeded, in Spanish, e.g., `Estados de reserva`
- `{{.TableName}}` — Target table, e.g., `booking_statuses`
- `{{.SeedColumns}}` — Comma-separated column names, e.g., `id, code, label`
- `{{.SeedValues}}` — One `(...)` per row, e.g.:
  ```sql
        (uuidv7(), 'pending', 'Pendiente'),
        (uuidv7(), 'confirmed', 'Confirmada'),
        (uuidv7(), 'cancelled', 'Cancelada')
  ```

**Advanced: Seed with FK lookup**
```sql
INSERT INTO {{.TableName}}({{.SeedColumns}})
SELECT
    uuidv7(),
    {{.LiteralValues}},
    (SELECT id FROM {{.RefTable}} WHERE {{.RefCondition}} LIMIT 1)
ON CONFLICT ({{.ConflictColumn}}) DO NOTHING;
```

### Template 5: Index (standalone or partial)

```sql
-- Standard index
CREATE INDEX IF NOT EXISTS idx_{{.TableName}}_{{.ColumnName}}
    ON {{.TableName}}({{.IndexColumns}});

-- Partial index (filtered)
CREATE INDEX IF NOT EXISTS idx_{{.TableName}}_{{.ColumnName}}
    ON {{.TableName}}({{.IndexColumns}})
    WHERE {{.FilterCondition}};

-- Multi-column index for active-filter queries
CREATE INDEX IF NOT EXISTS idx_{{.TableName}}_active
    ON {{.TableName}}({{.IndexColumns}})
    WHERE {{.ActiveCondition}};
```

**Placeholders**:
- `{{.IndexColumns}}` — e.g., `user_id`, `(status, created_at DESC)`, `(user_id, expires_at)`
- `{{.FilterCondition}}` — e.g., `status IN ('pending', 'processing')`, `deleted_at IS NULL`, `provider_message_id IS NOT NULL`

## Uses Skills

None. `migration-sql` is standalone — it has zero skill dependencies. Other skills depend on it:
- `module-scaffold` invokes `migration-sql` to generate the initial schema for a new module.

## Verification

After generating SQL, verify ALL of the following:

1. **UUIDv7 PK check**: Every `CREATE TABLE` has `id uuid PRIMARY KEY DEFAULT uuidv7()`
   ```bash
   rg "CREATE TABLE" migration.sql -A 1 | rg -v "uuidv7\(\)" && echo "FAIL: missing uuidv7" || echo "PASS"
   ```

2. **Timestamp columns check**: Every table has both `created_at timestamptz` and `updated_at timestamptz`
   ```bash
   # Count CREATE TABLE vs count of updated_at triggers
   echo "Tables: $(rg -c 'CREATE TABLE' migration.sql)"
   echo "Triggers: $(rg -c 'CREATE TRIGGER.*_updated_at' migration.sql)"
   # Must match — every table needs a trigger
   ```

3. **CHECK constraint naming**: All CHECK constraints use `CONSTRAINT chk_{table}_{field}`
   ```bash
   rg "CONSTRAINT chk_" migration.sql  # Should match every CHECK
   ```

4. **Index placement**: Indexes appear AFTER table definition and trigger (not inline in CREATE TABLE)
   ```bash
   # Verify CREATE INDEX lines appear after the last ) of each table block
   ```

5. **Seed data uses uuidv7()**: No hardcoded UUIDs in INSERT statements
   ```bash
   rg "INSERT INTO" migration.sql -A 5 | rg "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}" && echo "FAIL: hardcoded UUID" || echo "PASS"
   ```

6. **Spanish comments**: All block headers use Spanish
   ```bash
   rg "^-- [A-Z]" migration.sql | rg -v "[áéíóúñÁÉÍÓÚÑ]" && echo "WARN: possible English comment" || echo "PASS"
   ```

7. **Module isolation**: No cross-module FK references that violate R1 (tables only reference tables in the same module or shared schemas)

8. **Valid PostgreSQL syntax**: Generate must pass `psql` syntax check or at minimum pass visual inspection for:
   - Matching parentheses in CHECK constraints
   - Correct `uuidv7()` function name (not `gen_random_uuid()` or `uuid_generate_v4()`)
   - `update_updated_at_column()` trigger function referenced correctly
   - All `REFERENCES` point to existing tables
