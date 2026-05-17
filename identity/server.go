// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	mErr "github.com/mataki-dev/platform/errors"
)

// Middleware verifies bearer tokens on incoming requests and stashes the
// resolved Identity on the request context. It does NOT perform
// authorization — downstream layers consume Identity and apply their
// own policy.
type Middleware struct {
	verifier Verifier
	logger   *slog.Logger
}

// Option configures a Middleware.
type Option func(*options)

type options struct {
	logger *slog.Logger
}

// WithLogger overrides the slog.Logger used for auth events. Defaults
// to slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

// New returns a Middleware that authenticates with the given verifier
// (typically a Chain).
func New(v Verifier, opts ...Option) *Middleware {
	o := &options{logger: slog.Default()}
	for _, opt := range opts {
		opt(o)
	}
	return &Middleware{verifier: v, logger: o.logger}
}

type identityCtxKey struct{}

// FromContext returns the Identity stashed by Handler. ok is false if
// Handler has not run or authentication failed.
func FromContext(ctx context.Context) (*Identity, bool) {
	id, ok := ctx.Value(identityCtxKey{}).(*Identity)
	return id, ok
}

// Handler is a net/http middleware. On verification failure it writes
// a 401 via the platform errors envelope. Authorization decisions (403,
// allow-list, perms) belong in a downstream middleware or handler that
// reads Identity from the request context.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeSemanticError(w, mErr.NewUnauthorized("missing or malformed Authorization header"))
			m.logger.WarnContext(r.Context(), "identity: missing auth header", "path", r.URL.Path)
			return
		}

		id, err := m.verifier.Verify(r.Context(), token)
		if err != nil {
			writeSemanticError(w, mErr.NewUnauthorized("authentication failed"))
			m.logger.WarnContext(r.Context(), "identity: verification failed",
				"path", r.URL.Path, "error", err)
			return
		}
		if id == nil {
			writeSemanticError(w, mErr.NewUnauthorized("authentication failed"))
			m.logger.WarnContext(r.Context(), "identity: no verifier accepted token",
				"path", r.URL.Path)
			return
		}

		id.OBO = ReadOnBehalfOf(r.Header)

		ctx := context.WithValue(r.Context(), identityCtxKey{}, id)
		m.logger.InfoContext(ctx, "identity: authenticated",
			"path", r.URL.Path,
			"issuer", id.Issuer,
			"subject", id.Subject,
			"obo_actor", string(id.OBO.Actor),
			"obo_org", id.OBO.OrgID,
			"obo_user", id.OBO.UserID,
			"request_id", id.OBO.RequestID,
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerToken strips the "Bearer " prefix from an Authorization header.
// Returns ("", false) if the header is empty or doesn't start with the
// prefix.
func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimPrefix(header, prefix)
	if token == "" {
		return "", false
	}
	return token, true
}

// writeSemanticError renders a SemanticError as the standard Mataki
// error envelope (type + message JSON body, semantic HTTP status).
func writeSemanticError(w http.ResponseWriter, e *mErr.SemanticError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.HTTPStatus())
	_ = json.NewEncoder(w).Encode(mErr.NewErrorBody(e))
}
