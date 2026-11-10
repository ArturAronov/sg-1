package platform

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
)

type memberTestRuntime struct {
	Runtime
	host string
}

func (r memberTestRuntime) Resolve(context.Context, string, string) (string, error) {
	return r.host, nil
}

func TestMemberOnlyAppAccessAndRevocation(t *testing.T) {
	s, owner, project, _ := cancellationFixture(t)
	ctx := context.Background()
	s.SigningKey = []byte("fixture-gateway-key-at-least-32-bytes")
	s.AppOrigin = "https://{slug}.apps.chickensoup.dev"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Amzn-Oidc-Data") != "" || r.Header.Get("Cookie") != "" {
			t.Error("platform access credentials reached generated code")
		}
		_, _ = io.WriteString(w, "member application")
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	s.Cloud = memberTestRuntime{host: target.Host}
	handler := s.Gateway("https://sg1.chickensoup.dev")
	request := func(path, accept string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", s.appURL(project.Slug)+path, nil)
		r.Header.Set("Accept", accept)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if project.AccessMode != "members" {
		t.Fatal("new projects must default to member access")
	}
	var outsiderID string
	if err := s.Store.Pool.QueryRow(ctx, `INSERT INTO users(email,name,platform_admin) VALUES('outsider@example.test','Non-member admin',true) RETURNING id::text`).Scan(&outsiderID); err != nil {
		t.Fatal(err)
	}
	outsider, err := s.Store.User(ctx, outsiderID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.MemberURL(ctx, outsider, project.Slug); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("platform admin without explicit membership received app session", err)
	}
	if _, err = s.PreviewURL(ctx, outsider, project.Slug); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("preview bypassed explicit membership", err)
	}
	if response := request("/", "text/html", nil); response.Code != 303 || response.Header().Get("Location") != "https://sg1.chickensoup.dev/app-access/"+project.Slug {
		t.Fatal("direct member app must require platform sign-in", response.Code)
	}
	if response := request("/api/data", "application/json", nil); response.Code != 401 {
		t.Fatal("anonymous API access allowed", response.Code)
	}
	login, err := s.MemberURL(ctx, owner, project.Slug)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(login)
	response := request(u.RequestURI(), "text/html", nil)
	if response.Code != 303 || len(response.Result().Cookies()) != 1 {
		t.Fatal("member session exchange failed", response.Code)
	}
	cookie := response.Result().Cookies()[0]
	if cookie.Name != appCookie || cookie.Domain != "" || !cookie.HttpOnly || !cookie.Secure {
		t.Fatal("member cookie is not host protected")
	}
	if r := request("/api/data", "application/json", cookie); r.Code != 200 || r.Body.String() != "member application" {
		t.Fatal("member cannot access application", r.Code)
	}
	if _, err = s.Store.Pool.Exec(ctx, `DELETE FROM project_memberships WHERE project_id=$1 AND user_id=$2`, project.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if r := request("/api/data", "application/json", cookie); r.Code != 401 {
		t.Fatal("removed membership retained access", r.Code)
	}
	if _, err = s.Store.Pool.Exec(ctx, `INSERT INTO project_memberships(project_id,user_id,role) VALUES($1,$2,'admin')`, project.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.Pool.Exec(ctx, `UPDATE users SET active=false WHERE id=$1`, owner.ID); err != nil {
		t.Fatal(err)
	}
	if r := request("/api/data", "application/json", cookie); r.Code != 401 {
		t.Fatal("inactive member retained access", r.Code)
	}
	if _, err = s.Store.Pool.Exec(ctx, `UPDATE users SET active=true WHERE id=$1`, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SetAccess(ctx, owner, project.Slug, "public", ""); err != nil {
		t.Fatal(err)
	}
	if r := request("/api/data", "application/json", nil); r.Code != 200 {
		t.Fatal("public mode still requires identity", r.Code)
	}
	if err = s.SetAccess(ctx, owner, project.Slug, "members", ""); err != nil {
		t.Fatal(err)
	}
	if r := request("/api/data", "application/json", nil); r.Code != 401 {
		t.Fatal("switching to members left public access open", r.Code)
	}
	if r := request("/api/data", "application/json", cookie); r.Code != 401 {
		t.Fatal("old member cookie survived access mode changes", r.Code)
	}
	var editorID string
	if err = s.Store.Pool.QueryRow(ctx, `INSERT INTO users(email,name) VALUES('editor@example.test','Editor') RETURNING id::text`).Scan(&editorID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.Pool.Exec(ctx, `INSERT INTO project_memberships(project_id,user_id,role) VALUES($1,$2,'write')`, project.ID, editorID); err != nil {
		t.Fatal(err)
	}
	editor, _ := s.Store.User(ctx, editorID)
	if err = s.SetAccess(ctx, editor, project.Slug, "public", ""); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("non-admin changed app access", err)
	}
	if _, err = s.MemberURL(ctx, editor, project.Slug); err != nil {
		t.Fatal("explicit non-admin member denied app access", err)
	}
}
