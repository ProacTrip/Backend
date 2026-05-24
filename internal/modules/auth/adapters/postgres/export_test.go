package postgres

// Exports for black-box testing (S4 convention).

// Pool returns the underlying PgxPool for structural tests.
func (r *OAuthRepository) Pool() PgxPool { return r.pool }

// Pool returns the underlying PgxPool for structural tests.
func (r *UserRepository) Pool() PgxPool { return r.pool }

// Pool returns the underlying PgxPool for structural tests.
func (r *DocumentRepository) Pool() PgxPool { return r.pool }
