package subscriber

import "testing"

func TestParseHeaderFlags(t *testing.T) {
	t.Run("valid single header", func(t *testing.T) {
		got, err := parseHeaderFlags([]string{"X-Api-Key: secret"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["X-Api-Key"] != "secret" {
			t.Errorf("got %q, want %q", got["X-Api-Key"], "secret")
		}
	})
	t.Run("valid multiple headers", func(t *testing.T) {
		got, err := parseHeaderFlags([]string{"X-Api-Key: secret", "CF-Access-Client-Id: id"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got["X-Api-Key"] != "secret" || got["CF-Access-Client-Id"] != "id" {
			t.Errorf("unexpected result: %#v", got)
		}
	})
	t.Run("value containing colon is preserved", func(t *testing.T) {
		got, err := parseHeaderFlags([]string{"Authorization: Bearer abc:def"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["Authorization"] != "Bearer abc:def" {
			t.Errorf("got %q", got["Authorization"])
		}
	})
	t.Run("empty input", func(t *testing.T) {
		got, err := parseHeaderFlags(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty map, got %#v", got)
		}
	})
	t.Run("missing colon is invalid", func(t *testing.T) {
		if _, err := parseHeaderFlags([]string{"X-Api-Key secret"}); err == nil {
			t.Error("expected error, got nil")
		}
	})
	t.Run("empty key is invalid", func(t *testing.T) {
		if _, err := parseHeaderFlags([]string{": secret"}); err == nil {
			t.Error("expected error, got nil")
		}
	})
}
