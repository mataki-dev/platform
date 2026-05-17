// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"errors"
	"testing"
)

func TestVerifierFunc_Verify(t *testing.T) {
	wantID := &Identity{
		Issuer:  "https://accounts.google.com",
		Subject: "12345",
		Email:   "svc@example.iam.gserviceaccount.com",
	}
	f := VerifierFunc(func(ctx context.Context, token string) (*Identity, error) {
		if token != "tok" {
			t.Errorf("got token %q", token)
		}
		return wantID, nil
	})
	got, err := f.Verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != wantID {
		t.Errorf("got %+v want %+v", got, wantID)
	}
}

func TestChain_FirstHitWins(t *testing.T) {
	first := &Identity{Subject: "first"}
	c := Chain{
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			return first, nil
		}),
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			t.Error("should not be called after first hit")
			return &Identity{Subject: "second"}, nil
		}),
	}
	got, err := c.Verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != first {
		t.Errorf("got %+v want first", got)
	}
}

func TestChain_FallThrough(t *testing.T) {
	target := &Identity{Subject: "third"}
	c := Chain{
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			return nil, nil
		}),
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			return nil, nil
		}),
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			return target, nil
		}),
	}
	got, err := c.Verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != target {
		t.Errorf("got %+v want target", got)
	}
}

func TestChain_ErrorShortCircuits(t *testing.T) {
	myErr := errors.New("boom")
	c := Chain{
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			return nil, nil
		}),
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			return nil, myErr
		}),
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			t.Error("should not be called after error")
			return &Identity{}, nil
		}),
	}
	got, err := c.Verify(context.Background(), "tok")
	if got != nil {
		t.Errorf("expected nil identity, got %+v", got)
	}
	if !errors.Is(err, myErr) {
		t.Errorf("got err %v want myErr", err)
	}
}

func TestChain_AllFallThroughReturnsNoVerifierAccepted(t *testing.T) {
	c := Chain{
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			return nil, nil
		}),
		VerifierFunc(func(ctx context.Context, _ string) (*Identity, error) {
			return nil, nil
		}),
	}
	got, err := c.Verify(context.Background(), "tok")
	if got != nil {
		t.Errorf("expected nil identity, got %+v", got)
	}
	if !errors.Is(err, ErrNoVerifierAccepted) {
		t.Errorf("got err %v want ErrNoVerifierAccepted", err)
	}
}

func TestChain_Empty(t *testing.T) {
	c := Chain{}
	_, err := c.Verify(context.Background(), "tok")
	if !errors.Is(err, ErrNoVerifierAccepted) {
		t.Errorf("got err %v want ErrNoVerifierAccepted", err)
	}
}
