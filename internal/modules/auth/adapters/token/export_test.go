package token

import "aidanwoods.dev/go-paseto/v2"

// Exports for black-box testing (S4 convention).

// IsExpiredTokenError wraps the unexported isExpiredTokenError.
var IsExpiredTokenError = isExpiredTokenError

// SymmetricKey returns the symmetric key for direct token parsing in tests.
func (s *PasetoService) SymmetricKey() paseto.V4SymmetricKey { return s.symmetricKey }
