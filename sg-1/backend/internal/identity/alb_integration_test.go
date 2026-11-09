package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestIdentitySubjectsNeverLinkByEmail(t *testing.T) {
	connection := os.Getenv("SG1_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("SG1_TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	random := make([]byte, 8)
	if _, err = rand.Read(random); err != nil {
		t.Fatal(err)
	}
	schema := "sg1_identity_test_" + hex.EncodeToString(random)
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE") }()
	config, err := pgxpool.ParseConfig(connection)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = store.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	verifier, key, head := fixture(t)
	auth := &Authenticator{Store: &store.Store{Pool: pool}, Verifier: verifier, BootstrapSubjects: map[string]bool{"approved-admin": true}}
	adminUser, err := auth.Authenticate(ctx, sign(t, key, head, Claims{Subject: "approved-admin", Email: "same@example.com"}))
	if err != nil || !adminUser.PlatformAdmin {
		t.Fatal("bootstrap administrator failed", err)
	}
	other, err := auth.Authenticate(ctx, sign(t, key, head, Claims{Subject: "different-person", Email: "same@example.com"}))
	if err != nil || other.ID == adminUser.ID || other.PlatformAdmin {
		t.Fatal("email matching escalated an independent identity", err)
	}
	departments, err := auth.Store.Departments(ctx, other)
	if err != nil || len(departments) != 0 {
		t.Fatal("new employee received an implicit membership", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE users SET active=false WHERE id=$1`, adminUser.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Authenticate(ctx, sign(t, key, head, Claims{Subject: "approved-admin"})); err == nil {
		t.Fatal("inactive employee retained API access")
	}
}
