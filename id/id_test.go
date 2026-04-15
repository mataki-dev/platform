package id_test

import (
	"strings"
	"testing"

	"github.com/mataki-dev/platform/id"
)

func TestNew_Format(t *testing.T) {
	got := id.New("conn")
	if !strings.HasPrefix(got, "conn_") {
		t.Fatalf("New(%q) = %q, want prefix %q", "conn", got, "conn_")
	}
	parts := strings.SplitN(got, "_", 2)
	if len(parts) != 2 || parts[1] == "" {
		t.Fatalf("New(%q) = %q, want prefix_payload format", "conn", got)
	}
}

func TestNew_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		v := id.New("test")
		if seen[v] {
			t.Fatalf("duplicate ID generated: %s", v)
		}
		seen[v] = true
	}
}

func TestNew_DifferentPrefixes(t *testing.T) {
	a := id.New("conn")
	b := id.New("key")
	if !strings.HasPrefix(a, "conn_") {
		t.Fatalf("expected conn_ prefix, got %s", a)
	}
	if !strings.HasPrefix(b, "key_") {
		t.Fatalf("expected key_ prefix, got %s", b)
	}
}

func TestValidate_Valid(t *testing.T) {
	raw := id.New("conn")
	if err := id.Validate(raw, "conn"); err != nil {
		t.Fatalf("Validate(%q, %q) = %v, want nil", raw, "conn", err)
	}
}

func TestValidate_WrongPrefix(t *testing.T) {
	raw := id.New("conn")
	err := id.Validate(raw, "key")
	if err == nil {
		t.Fatal("Validate with wrong prefix should fail")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_Empty(t *testing.T) {
	if err := id.Validate("", "conn"); err == nil {
		t.Fatal("Validate empty string should fail")
	}
}

func TestValidate_NoUnderscore(t *testing.T) {
	if err := id.Validate("connABC123", "conn"); err == nil {
		t.Fatal("Validate without underscore should fail")
	}
}

func TestValidate_EmptyPayload(t *testing.T) {
	if err := id.Validate("conn_", "conn"); err == nil {
		t.Fatal("Validate with empty payload should fail")
	}
}

func TestValidate_InvalidBase58Chars(t *testing.T) {
	// '0', 'O', 'I', 'l' are not in base58 alphabet
	err := id.Validate("conn_0OIl", "conn")
	if err == nil {
		t.Fatal("Validate with invalid base58 chars should fail")
	}
}

func TestValidate_WrongPayloadLength(t *testing.T) {
	// "1" decodes to a single zero byte, not 16 bytes
	err := id.Validate("conn_1", "conn")
	if err == nil {
		t.Fatal("Validate with wrong payload length should fail")
	}
	if !strings.Contains(err.Error(), "decoded length") {
		t.Fatalf("unexpected error: %v", err)
	}
}
