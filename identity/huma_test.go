// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

func TestHumaMiddleware_SuccessRunsHandler(t *testing.T) {
	wantID := &Identity{Subject: "42", Email: "svc@x"}
	mw := New(VerifierFunc(func(context.Context, string) (*Identity, error) {
		return wantID, nil
	}))

	_, api := humatest.New(t)
	api.UseMiddleware(mw.HumaMiddleware())

	huma.Get(api, "/whoami", func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Subject string `json:"subject"`
		}
	}, error) {
		id, _ := FromContext(ctx)
		out := &struct {
			Body struct {
				Subject string `json:"subject"`
			}
		}{}
		out.Body.Subject = id.Subject
		return out, nil
	})

	resp := api.Get("/whoami", "Authorization: Bearer x")
	if resp.Code != http.StatusOK {
		t.Errorf("status: got %d want 200, body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"subject":"42"`) {
		t.Errorf("body missing subject: %s", resp.Body.String())
	}
}

func TestHumaMiddleware_MissingHeaderReturns401(t *testing.T) {
	mw := New(VerifierFunc(func(context.Context, string) (*Identity, error) {
		t.Error("verifier should not run")
		return nil, nil
	}))

	_, api := humatest.New(t)
	api.UseMiddleware(mw.HumaMiddleware())

	huma.Get(api, "/x", func(context.Context, *struct{}) (*struct{}, error) {
		t.Error("handler should not run")
		return nil, nil
	})

	resp := api.Get("/x")
	if resp.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", resp.Code)
	}
}

func TestHumaMiddleware_VerifierErrorReturns401(t *testing.T) {
	mw := New(VerifierFunc(func(context.Context, string) (*Identity, error) {
		return nil, errors.New("bad signature")
	}))

	_, api := humatest.New(t)
	api.UseMiddleware(mw.HumaMiddleware())

	huma.Get(api, "/x", func(context.Context, *struct{}) (*struct{}, error) {
		return nil, nil
	})

	resp := api.Get("/x", "Authorization: Bearer x")
	if resp.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", resp.Code)
	}
}
