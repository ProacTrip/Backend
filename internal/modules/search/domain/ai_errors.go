// AI Search Error Reference
//
// This file documents all error types used in the ai_search flow.
// Domain errors (defined in errors.go):
//   - ErrAIUnavailable → 503 Service Unavailable (AI provider down)
//   - ErrAIParseFailure → 502 Bad Gateway (AI returned invalid JSON)
//   - ErrConversationNotFound → 400 Bad Request
//   - ErrTurnLimitExceeded → 400 Bad Request
//   - ErrSearchFailed → 502 Bad Gateway (all providers failed)
//   - ErrNoResults → handled as empty response, not error
//
// Provider errors (from search features):
//   - ErrProviderUnavailable → 503 (SerpAPI 5xx or down, defined in errors.go)
//   - ErrProviderBadRequest → 502 (SerpAPI 4xx, our bad params)
//   - ErrRateLimitExceeded → 429 (defined in shared/ratelimit)
//
// Transport errors (from adapters):
//   - Network errors → wrapped as ErrProviderUnavailable by client
//   - JSON decode errors (SerpAPI 200 but bad body) → wrapped as ErrProviderUnavailable
//   - SerpAPI error messages in response body → wrapped as ErrProviderUnavailable
package domain
