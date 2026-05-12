package environment

import (
	"errors"
	"net/http"
	"testing"

	"github.com/ProacTrip/Backend/internal/modules/environment/domain"
	serrors "github.com/ProacTrip/Backend/internal/shared/errors"
)

func TestEnvironmentErrorMapper(t *testing.T) {
	// Registrar los mapeos como lo haría NewModule
	registerEnvironmentErrors()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantType   serrors.ProblemType
	}{
		{
			name:       "ErrInvalidIP → 400 Bad Request",
			err:        domain.ErrInvalidIP,
			wantStatus: http.StatusBadRequest,
			wantType:   serrors.ProblemTypeBadRequest,
		},
		{
			name:       "ErrLocationProvider → 502 Bad Gateway",
			err:        domain.ErrLocationProvider,
			wantStatus: http.StatusBadGateway,
			wantType:   serrors.ProblemTypeBadGateway,
		},
		{
			name:       "ErrRateLimitExceeded → 429 Too Many Requests",
			err:        domain.ErrRateLimitExceeded,
			wantStatus: http.StatusTooManyRequests,
			wantType:   serrors.ProblemTypeTooManyRequests,
		},
		{
			name:       "ErrInternal → 500 Internal Server Error",
			err:        domain.ErrInternal,
			wantStatus: http.StatusInternalServerError,
			wantType:   serrors.ProblemTypeInternalError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problem := serrors.MapDomainError(tc.err)
			if problem == nil {
				t.Fatal("MapDomainError() retornó nil — el mapper no reconoce el error")
			}
			if problem.Status != tc.wantStatus {
				t.Errorf("Status = %d, esperaba %d", problem.Status, tc.wantStatus)
			}
			if problem.Type != tc.wantType {
				t.Errorf("Type = %q, esperaba %q", problem.Type, tc.wantType)
			}
			if problem.Detail != tc.err.Error() {
				t.Errorf("Detail = %q, esperaba %q (mensaje del error centinela)", problem.Detail, tc.err.Error())
			}
		})
	}
}

func TestEnvironmentErrorMapper_UnknownError(t *testing.T) {
	registerEnvironmentErrors()

	// Un error que ningún mapper conoce debe retornar nil
	unknownErr := errors.New("error desconocido sin mapeo")
	problem := serrors.MapDomainError(unknownErr)
	if problem != nil {
		t.Errorf("MapDomainError() para error desconocido debe retornar nil, obtuve %+v", problem)
	}
}

func TestEnvironmentErrorMapper_WrappedErrors(t *testing.T) {
	registerEnvironmentErrors()

	// errors.Is debe funcionar a través de errores envueltos
	wrapped := domain.ErrInvalidIP // wrapped directo, el mapper usa errors.Is
	problem := serrors.MapDomainError(wrapped)
	if problem == nil {
		t.Fatal("errors.Is no detectó el centinela en un error envuelto")
	}
	if problem.Status != http.StatusBadRequest {
		t.Errorf("Status = %d, esperaba 400", problem.Status)
	}
}
