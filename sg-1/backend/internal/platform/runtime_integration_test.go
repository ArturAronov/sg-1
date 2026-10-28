package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDockerBuildDeployRollbackAndPersistence(t *testing.T) {
	if os.Getenv("SG1_TEST_DOCKER") != "1" {
		t.Skip("set SG1_TEST_DOCKER=1 with the local Compose stack running")
	}
	connection := os.Getenv("SG1_TEST_DATABASE_URL")
	if connection == "" {
		t.Fatal("SG1_TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "sg1_runtime_test_" + randomHex()[:16]
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE") }()
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
	repo := &store.Store{Pool: pool}
	if err = repo.SeedDevelopment(ctx); err != nil {
		t.Fatal(err)
	}
	user, _ := repo.User(ctx, store.DevUserID)
	p, _, err := repo.CreateProject(ctx, user, domain.CreateProject{DepartmentID: "00000000-0000-4000-8000-000000000002", Name: "Runtime integration", Slug: "runtime-" + randomHex()[:12]}, "runtime-create-key")
	if err != nil {
		t.Fatal(err)
	}
	vault, _ := secrets.NewVault(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32)))
	service := &Service{Store: repo, Vault: vault, DatabaseHost: "sg1-postgres-1", Docker: &Docker{NetworkPrefix: "sg1-test-", GatewayContainer: "sg1-gateway-1", PostgresContainer: "sg1-postgres-1"}, Provider: &ProviderClient{}}
	if os.Getenv("SG1_CREDENTIALS_KEY") == "" {
		t.Fatal("load the local encryption key with scripts/test-runtime.mjs to authenticate trusted probes")
	}
	signing := sha256.Sum256([]byte("sg1-gateway/v1:" + os.Getenv("SG1_CREDENTIALS_KEY")))
	service.SigningKey = signing[:]
	service.GatewayURL = "http://127.0.0.1:8081"
	network := service.Docker.NetworkPrefix + strings.ReplaceAll(p.ID, "-", "")
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		rows, err := pool.Query(cleanup, `SELECT container_name FROM deployments WHERE project_id=$1`, p.ID)
		if err == nil {
			for rows.Next() {
				var name string
				_ = rows.Scan(&name)
				service.Docker.Remove(cleanup, name)
			}
			rows.Close()
		}
		_, _ = service.Docker.command(cleanup, nil, nil, "network", "disconnect", network, "sg1-gateway-1")
		_, _ = service.Docker.command(cleanup, nil, nil, "network", "disconnect", network, "sg1-postgres-1")
		_, _ = service.Docker.command(cleanup, nil, nil, "network", "rm", network)
		for _, prefix := range []string{"sg1_p", "sg1_q"} {
			name := pgx.Identifier{prefix + strings.ReplaceAll(p.ID, "-", "")}.Sanitize()
			_, _ = admin.Exec(cleanup, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			_, _ = admin.Exec(cleanup, "DROP ROLE IF EXISTS "+name)
		}
	}()
	if err = service.provision(ctx, p.ID); err != nil {
		t.Fatal("real container provisioning", err)
	}
	state, err := service.State(ctx, user, p.Slug)
	if err != nil || state.Project.Status != "ready" || state.CurrentDeploymentID == nil {
		t.Fatal("starter not deployed", err)
	}
	initial := *state.CurrentDeploymentID
	db, err := service.databases(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	localURL, _ := url.Parse(db.Production)
	testURL, _ := url.Parse(connection)
	localURL.Host = testURL.Host
	appDB, err := pgxpool.New(ctx, localURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer appDB.Close()
	if _, err = appDB.Exec(ctx, `CREATE TABLE records(id integer PRIMARY KEY,label text NOT NULL);INSERT INTO records VALUES(1,'persistent tenant record')`); err != nil {
		t.Fatal("persistent application database", err)
	}
	deniedURL := *localURL
	deniedURL.Path = "/sg1"
	denied, err := pgxpool.New(ctx, deniedURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if err = denied.Ping(ctx); err == nil {
		denied.Close()
		t.Fatal("application could access control-plane database")
	}
	denied.Close()
	ciphertext, _ := vault.Seal(user.ID, "openai", "synthetic-runtime-key")
	if _, err = repo.SaveLLMCredential(ctx, user.ID, "openai", ciphertext); err != nil {
		t.Fatal(err)
	}
	bad := false
	service.Provider.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		files := StarterFiles()
		files["server.js"] = strings.ReplaceAll(files["server.js"], "Your project is running.", "Generated maintenance tracker")
		if bad {
			files["Dockerfile"] = "FROM scratch\nRUN exit 1\n"
		}
		generated, _ := json.Marshal(Generated{Summary: "Generated maintenance tracker", Files: files})
		response, _ := json.Marshal(map[string]any{"output": []any{map[string]any{"content": []any{map[string]string{"type": "output_text", "text": string(generated)}}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(response))}, nil
	})}
	id, err := service.QueueBuild(ctx, user, p.Slug, "openai", "fixture-model", "Build a maintenance tracker", "runtime-build-request", "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.QueueBuild(ctx, user, p.Slug, "openai", "fixture-model", "Build a maintenance tracker", "runtime-build-request", "")
	if err != nil || again != id {
		t.Fatal("build submission was not idempotent", err)
	}
	if _, err = service.QueueBuild(ctx, user, p.Slug, "openai", "fixture-model", "Another request", "another-build-request", ""); !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent project builds were not blocked", err)
	}
	if _, err = service.buildOne(ctx); err != nil {
		t.Fatal("generated Docker build and deployment", err)
	}
	state, err = service.State(ctx, user, p.Slug)
	if err != nil || state.Builds[0].Status != "succeeded" || *state.CurrentDeploymentID == initial {
		t.Fatal("generated app not deployed", err)
	}
	working := *state.CurrentDeploymentID
	bad = true
	if _, err = service.QueueBuild(ctx, user, p.Slug, "openai", "fixture-model", "Invalid image", "broken-build-request", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = service.buildOne(ctx); err == nil {
		t.Fatal("invalid Docker build succeeded")
	}
	state, _ = service.State(ctx, user, p.Slug)
	if *state.CurrentDeploymentID != working || state.Builds[0].Status != "failed" {
		t.Fatal("failed build replaced the live app")
	}
	lyingHealth := StarterFiles()
	lyingHealth["server.js"] = "setInterval(() => {}, 1000);"
	lines := strings.Split(lyingHealth["Dockerfile"], "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "HEALTHCHECK ") {
			lines[i] = `HEALTHCHECK --interval=1s --timeout=1s CMD node -e "process.exit(0)"`
		}
	}
	lyingHealth["Dockerfile"] = strings.Join(lines, "\n")
	lyingImage := "sg1-unhealthy:" + p.ID
	if _, err = service.Docker.Build(ctx, lyingImage, lyingHealth); err != nil {
		t.Fatal(err)
	}
	if err = service.deploy(ctx, p.ID, lyingImage, "Must not publish", lyingHealth, network, db, ""); err == nil {
		t.Fatal("a lying Docker health check bypassed the independent HTTP probe")
	}
	state, _ = service.State(ctx, user, p.Slug)
	if *state.CurrentDeploymentID != working {
		t.Fatal("an app without HTTP health replaced the live version")
	}
	if _, err = service.QueueBuild(ctx, user, p.Slug, "", "", "Restore deployment "+initial, "rollback-request-key", initial); err != nil {
		t.Fatal(err)
	}
	if _, err = service.buildOne(ctx); err != nil {
		t.Fatal("real image rollback", err)
	}
	var label string
	if err = appDB.QueryRow(ctx, `SELECT label FROM records WHERE id=1`).Scan(&label); err != nil || label != "persistent tenant record" {
		t.Fatal("deployment/rollback lost application data", err)
	}
}
