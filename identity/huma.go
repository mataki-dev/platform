// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"

	mErr "github.com/mataki-dev/platform/errors"
)

// HumaMiddleware returns a huma middleware equivalent to Handler.
// Errors render via the existing errors package's SemanticError envelope.
func (m *Middleware) HumaMiddleware() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		token, ok := bearerToken(ctx.Header("Authorization"))
		if !ok {
			writeHumaSemanticError(ctx, mErr.NewUnauthorized("missing or malformed Authorization header"))
			m.logger.WarnContext(ctx.Context(), "identity: missing auth header", "path", ctx.URL().Path)
			return
		}

		id, err := m.verifier.Verify(ctx.Context(), token)
		if err != nil {
			writeHumaSemanticError(ctx, mErr.NewUnauthorized("authentication failed"))
			m.logger.WarnContext(ctx.Context(), "identity: verification failed",
				"path", ctx.URL().Path, "error", err)
			return
		}
		if id == nil {
			writeHumaSemanticError(ctx, mErr.NewUnauthorized("authentication failed"))
			m.logger.WarnContext(ctx.Context(), "identity: no verifier accepted token",
				"path", ctx.URL().Path)
			return
		}

		id.OBO = readHumaOBO(ctx)

		innerCtx := context.WithValue(ctx.Context(), identityCtxKey{}, id)
		m.logger.InfoContext(innerCtx, "identity: authenticated",
			"path", ctx.URL().Path,
			"issuer", id.Issuer,
			"subject", id.Subject,
			"obo_actor", string(id.OBO.Actor),
			"obo_org", id.OBO.OrgID,
			"obo_user", id.OBO.UserID,
			"request_id", id.OBO.RequestID,
		)
		next(huma.WithContext(ctx, innerCtx))
	}
}

// readHumaOBO reads OBO headers from a huma.Context.
func readHumaOBO(ctx huma.Context) OnBehalfOf {
	actor := ActorKind(ctx.Header("X-On-Behalf-Of-Actor"))
	if actor == "" {
		actor = ActorService
	}
	return OnBehalfOf{
		Actor:     actor,
		OrgID:     ctx.Header("X-On-Behalf-Of-Org"),
		UserID:    ctx.Header("X-On-Behalf-Of-User"),
		RequestID: ctx.Header("X-Request-Id"),
	}
}

// writeHumaSemanticError renders a SemanticError as the standard Mataki
// error envelope directly on the huma.Context. Bypasses huma.WriteErr
// (which needs an API instance) so the middleware stays decoupled.
func writeHumaSemanticError(ctx huma.Context, e *mErr.SemanticError) {
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(e.HTTPStatus())
	_ = json.NewEncoder(ctx.BodyWriter()).Encode(mErr.NewErrorBody(e))
}
