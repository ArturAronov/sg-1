package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
)

func TestIntegrationLLMCredentials(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	vault, _ := secrets.NewVault(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)))
	var other string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO users(email,name,platform_admin) VALUES('other@localhost','Other',true) RETURNING id::text`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"openai", "anthropic", "glm", "kimi"} {
		key := "synthetic-personal-key-" + provider
		ciphertext, err := vault.Seal(DevUserID, provider, key)
		if err != nil {
			t.Fatal(err)
		}
		status, err := s.SaveLLMCredential(ctx, DevUserID, provider, ciphertext)
		if err != nil || !status.Configured || status.UpdatedAt == nil {
			t.Fatal(status, err)
		}
		var stored []byte
		if err := s.Pool.QueryRow(ctx, `SELECT key_ciphertext FROM llm_credentials WHERE user_id=$1 AND provider=$2`, DevUserID, provider).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(stored, []byte(key)) {
			t.Fatal("stored plaintext key")
		}
		plain, err := vault.Open(DevUserID, provider, stored)
		if err != nil || string(plain) != key {
			t.Fatal("database round-trip failed", err)
		}
		if err := s.DeleteLLMCredential(ctx, other, provider); err != nil {
			t.Fatal(err)
		}
		list, err := s.LLMCredentials(ctx, other)
		if err != nil || len(list) != 0 {
			t.Fatal("other user saw credential metadata", err)
		}
	}
	list, err := s.LLMCredentials(ctx, DevUserID)
	if err != nil || len(list) != 4 {
		t.Fatal("other user's removal affected owner", err)
	}
	replacement, _ := vault.Seal(DevUserID, "openai", "synthetic-replacement-key")
	if _, err := s.SaveLLMCredential(ctx, DevUserID, "openai", replacement); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM llm_credentials WHERE user_id=$1`, DevUserID).Scan(&count); err != nil || count != 4 {
		t.Fatal("replacement duplicated rows", err)
	}
	var metadata string
	if err := s.Pool.QueryRow(ctx, `SELECT string_agg(metadata::text,' ') FROM audit_events`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, "synthetic-") || strings.Contains(metadata, "ciphertext") {
		t.Fatal("secret material in audit log")
	}
	if err := s.DeleteLLMCredential(ctx, DevUserID, "openai"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLLMCredential(ctx, DevUserID, "openai"); err != nil {
		t.Fatal("delete must be idempotent", err)
	}
	list, err = s.LLMCredentials(ctx, DevUserID)
	if err != nil || len(list) != 3 {
		t.Fatal("key not removed", err)
	}
}
