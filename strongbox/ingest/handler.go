// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/mataki-dev/platform/search"
	"github.com/mataki-dev/platform/strongbox"
)

// Register mounts the ingest HTTP handlers on mux under basePath.
func Register(mux *http.ServeMux, basePath string, cfg Config) {
	h := &handler{cfg: cfg}

	// Normalize basePath: ensure trailing slash stripped for route building.
	bp := strings.TrimRight(basePath, "/")

	// Discovery (no auth required, no TLS enforcement).
	mux.HandleFunc("GET "+bp+"/", h.discovery)

	// All other routes require TLS.
	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc("PUT "+bp+"/{environment}/secrets", h.syncSecrets)
	tlsMux.HandleFunc("POST "+bp+"/{environment}/secrets/search", h.searchSecrets)
	tlsMux.HandleFunc("POST "+bp+"/{environment}/secrets/delete", h.batchDelete)
	tlsMux.HandleFunc("POST "+bp+"/{environment}/secrets/webhook", h.webhook)
	tlsMux.HandleFunc("GET "+bp+"/{environment}/secrets/{key...}", h.getSecret)
	tlsMux.HandleFunc("DELETE "+bp+"/{environment}/secrets/{key...}", h.deleteSecret)

	// Wrap with TLS middleware and mount.
	wrapped := requireTLS(tlsMux)
	// We need to register patterns that forward to the TLS-wrapped mux.
	mux.Handle("PUT "+bp+"/{environment}/secrets", wrapped)
	mux.Handle("POST "+bp+"/{environment}/secrets/search", wrapped)
	mux.Handle("POST "+bp+"/{environment}/secrets/delete", wrapped)
	mux.Handle("POST "+bp+"/{environment}/secrets/webhook", wrapped)
	mux.Handle("GET "+bp+"/{environment}/secrets/{key...}", wrapped)
	mux.Handle("DELETE "+bp+"/{environment}/secrets/{key...}", wrapped)
}

// handler holds the ingest configuration and implements all HTTP handlers.
type handler struct {
	cfg Config
}

// ---------------------------------------------------------------------------
// Discovery
// ---------------------------------------------------------------------------

func (h *handler) discovery(w http.ResponseWriter, _ *http.Request) {
	resp := DiscoveryResponse{
		Provider: h.cfg.ProviderName,
		Capabilities: Capabilities{
			Search: true,
		},
		Endpoints: Endpoints{
			Sync:        "{environment}/secrets",
			Search:      "{environment}/secrets/search",
			Get:         "{environment}/secrets/{key}",
			Delete:      "{environment}/secrets/{key}",
			BatchDelete: "{environment}/secrets/delete",
			Webhook:     "{environment}/secrets/webhook",
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Common request processing
// ---------------------------------------------------------------------------

type requestContext struct {
	auth     AuthResult
	clientID strongbox.ClientID
	tenantID strongbox.TenantID
	env      string
	scope    strongbox.Scope
}

func (h *handler) processRequest(w http.ResponseWriter, r *http.Request) (*requestContext, bool) {
	// 1. Authenticate.
	auth, err := h.cfg.Authenticator(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{
			Error: "authentication failed",
			Code:  "unauthorized",
		})
		return nil, false
	}

	// 2. Resolve client.
	clientID, err := h.cfg.ClientResolver(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{
			Error: "client resolution failed",
			Code:  "unauthorized",
		})
		return nil, false
	}

	// 3. Extract environment.
	env := r.PathValue("environment")
	if env == "" {
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error: "environment not specified",
			Code:  "unknown_environment",
		})
		return nil, false
	}

	// 4. Validate environment.
	if !slices.Contains(h.cfg.Environments, env) {
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error: "unknown environment: " + env,
			Code:  "unknown_environment",
		})
		return nil, false
	}

	// 5. Resolve environment to TenantID.
	tenantID, err := h.cfg.EnvironmentResolver(clientID, env)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error: "environment resolution failed",
			Code:  "unknown_environment",
		})
		return nil, false
	}

	return &requestContext{
		auth:     auth,
		clientID: clientID,
		tenantID: tenantID,
		env:      env,
		scope:    strongbox.Scope{ClientID: clientID, TenantID: tenantID},
	}, true
}

// ---------------------------------------------------------------------------
// Sync (PUT /{environment}/secrets)
// ---------------------------------------------------------------------------

func (h *handler) syncSecrets(w http.ResponseWriter, r *http.Request) {
	rc, ok := h.processRequest(w, r)
	if !ok {
		return
	}

	var req BatchUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Code:  "invalid_request",
		})
		return
	}

	mode := strongbox.SyncPartial
	if req.SyncMode == "full" {
		mode = strongbox.SyncFull
	}

	var inputs []strongbox.PutInput
	for _, s := range req.Secrets {
		inputs = append(inputs, strongbox.PutInput{
			Ref:       strongbox.SecretRef(s.Key),
			Value:     s.Value,
			Metadata:  s.Metadata,
			ExpiresAt: s.ExpiresAt,
		})
	}

	result, err := h.cfg.Store.Sync(r.Context(), rc.scope, strongbox.SyncInput{
		Secrets:  inputs,
		SyncMode: mode,
	})
	if err != nil {
		mapStoreError(w, err)
		return
	}

	resp := BatchUpsertResponse{}
	for _, s := range result.Synced {
		resp.Synced = append(resp.Synced, IngestPutResult{
			Key:             string(s.Ref),
			Version:         s.Version,
			PreviousVersion: s.PreviousVersion,
			Action:          s.Action,
		})
	}
	for _, d := range result.Deleted {
		resp.Deleted = append(resp.Deleted, headerFromStrongbox(d))
	}
	for _, e := range result.Errors {
		resp.Errors = append(resp.Errors, IngestError{
			Key:     string(e.Ref),
			Code:    e.Code,
			Message: e.Message,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Search (POST /{environment}/secrets/search)
// ---------------------------------------------------------------------------

func (h *handler) searchSecrets(w http.ResponseWriter, r *http.Request) {
	rc, ok := h.processRequest(w, r)
	if !ok {
		return
	}

	var searchReq search.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&searchReq); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Code:  "invalid_request",
		})
		return
	}

	vs, errs := search.Validate(searchReq, secretSearchSchema)
	if len(errs) > 0 {
		details := make([]IngestError, len(errs))
		for i, e := range errs {
			details[i] = IngestError{
				Key:     e.Field,
				Code:    e.Code,
				Message: e.Message,
			}
		}
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error:   "search validation failed",
			Code:    "invalid_request",
			Details: details,
		})
		return
	}

	opts := toListOptions(vs)
	result, err := h.cfg.Store.List(r.Context(), rc.scope, opts)
	if err != nil {
		mapStoreError(w, err)
		return
	}

	var secrets []IngestHeader
	for _, s := range result.Secrets {
		secrets = append(secrets, headerFromStrongbox(s))
	}
	if secrets == nil {
		secrets = []IngestHeader{}
	}

	writeJSON(w, http.StatusOK, SearchSecretsResponse{
		Secrets: secrets,
		Cursor:  result.Cursor,
		HasMore: result.HasMore,
		Limit:   opts.Limit,
	})
}

// ---------------------------------------------------------------------------
// Get (GET /{environment}/secrets/{key...})
// ---------------------------------------------------------------------------

func (h *handler) getSecret(w http.ResponseWriter, r *http.Request) {
	rc, ok := h.processRequest(w, r)
	if !ok {
		return
	}

	key := r.PathValue("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "key is required",
			Code:  "invalid_key_pattern",
		})
		return
	}

	sv, err := h.cfg.Store.Get(r.Context(), rc.scope, strongbox.SecretRef(key))
	if err != nil {
		mapStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, GetSecretResponse{
		Key:       string(sv.Ref),
		Value:     sv.Value,
		Version:   sv.Version,
		Metadata:  sv.Metadata,
		CreatedAt: sv.CreatedAt,
		UpdatedAt: sv.UpdatedAt,
		ExpiresAt: sv.ExpiresAt,
	})
}

// ---------------------------------------------------------------------------
// Delete (DELETE /{environment}/secrets/{key...})
// ---------------------------------------------------------------------------

func (h *handler) deleteSecret(w http.ResponseWriter, r *http.Request) {
	rc, ok := h.processRequest(w, r)
	if !ok {
		return
	}

	key := r.PathValue("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "key is required",
			Code:  "invalid_key_pattern",
		})
		return
	}

	err := h.cfg.Store.Delete(r.Context(), rc.scope, strongbox.SecretRef(key))
	if err != nil {
		mapStoreError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Batch Delete (POST /{environment}/secrets/delete)
// ---------------------------------------------------------------------------

func (h *handler) batchDelete(w http.ResponseWriter, r *http.Request) {
	rc, ok := h.processRequest(w, r)
	if !ok {
		return
	}

	var req BatchDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Code:  "invalid_request",
		})
		return
	}

	refs := make([]strongbox.SecretRef, len(req.Keys))
	for i, k := range req.Keys {
		refs[i] = strongbox.SecretRef(k)
	}

	err := h.cfg.Store.DeleteMany(r.Context(), rc.scope, refs)
	if err != nil {
		mapStoreError(w, err)
		return
	}

	results := make([]IngestDeleteResult, len(req.Keys))
	for i, k := range req.Keys {
		results[i] = IngestDeleteResult{Key: k, Deleted: true}
	}
	writeJSON(w, http.StatusOK, BatchDeleteResponse{Deleted: results})
}

// ---------------------------------------------------------------------------
// Webhook (POST /{environment}/secrets/webhook)
// ---------------------------------------------------------------------------

func (h *handler) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "failed to read body",
			Code:  "invalid_request",
		})
		return
	}

	if h.cfg.WebhookSigningKey != "" {
		sigHeader := r.Header.Get("X-Webhook-Signature")
		if err := verifyWebhookSignature(body, sigHeader, h.cfg.WebhookSigningKey); err != nil {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{
				Error: "webhook signature verification failed",
				Code:  "unauthorized",
			})
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)

	if h.cfg.OnWebhook != nil {
		var payload WebhookPayload
		if err := json.Unmarshal(body, &payload); err == nil {
			go h.cfg.OnWebhook(context.Background(), payload)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func mapStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, strongbox.ErrNotFound):
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "secret_not_found",
		})
	case errors.Is(err, strongbox.ErrDeleted):
		writeJSON(w, http.StatusGone, ErrorResponse{
			Error: err.Error(),
			Code:  "secret_deleted",
		})
	case errors.Is(err, strongbox.ErrExpired):
		writeJSON(w, http.StatusGone, ErrorResponse{
			Error: err.Error(),
			Code:  "secret_expired",
		})
	case errors.Is(err, strongbox.ErrInvalidRef):
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "invalid_key_pattern",
		})
	case errors.Is(err, strongbox.ErrValueTooLarge):
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "value_too_large",
		})
	case errors.Is(err, strongbox.ErrBatchTooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge, ErrorResponse{
			Error: err.Error(),
			Code:  "batch_too_large",
		})
	default:
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "internal_error",
		})
	}
}