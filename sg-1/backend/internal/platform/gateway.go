package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

const appCookie = "__Host-sg1_app"

type ticket struct {
	Project string `json:"p"`
	User    string `json:"u,omitempty"`
	Kind    string `json:"k"`
	Version int    `json:"v"`
	Expires int64  `json:"e"`
	Target  string `json:"t,omitempty"`
}

func (s *Service) sign(t ticket) string {
	data, _ := json.Marshal(t)
	encoded := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, s.SigningKey)
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) verify(raw string) (ticket, error) {
	var t ticket
	if len(raw) > 2048 {
		return t, errors.New("invalid ticket")
	}
	encoded, sig, ok := strings.Cut(raw, ".")
	if !ok {
		return t, errors.New("invalid ticket")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return t, err
	}
	mac := hmac.New(sha256.New, s.SigningKey)
	mac.Write([]byte(encoded))
	if !hmac.Equal(decoded, mac.Sum(nil)) {
		return t, errors.New("invalid signature")
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return t, err
	}
	if json.Unmarshal(data, &t) != nil || t.Expires < time.Now().Unix() {
		return t, errors.New("expired ticket")
	}
	return t, nil
}

func (s *Service) appURL(slug string) string { return strings.ReplaceAll(s.AppOrigin, "{slug}", slug) }

func (s *Service) PreviewURL(ctx context.Context, u domain.User, slug string) (string, error) {
	p, err := s.Store.Project(ctx, u, slug)
	if err != nil {
		return "", err
	}
	if p.Status != "ready" {
		return "", ErrNotReady
	}
	var version int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT credential_version FROM project_access_settings WHERE project_id=$1`, p.ID).Scan(&version); err != nil {
		return "", err
	}
	t := s.sign(ticket{Project: p.ID, User: u.ID, Kind: "preview", Version: version, Expires: time.Now().Add(30 * time.Minute).Unix()})
	return s.appURL(slug) + "/__sg1/preview?ticket=" + url.QueryEscape(t), nil
}

type application struct {
	ID, Slug, Mode, Container string
	Password                  *string
	Version                   int
}

func (s *Service) lookupApp(ctx context.Context, slug string) (application, error) {
	var app application
	err := s.Store.Pool.QueryRow(ctx, `SELECT p.id::text,p.slug,a.mode,a.password_hash,a.credential_version,d.container_name
	 FROM projects p JOIN project_access_settings a ON a.project_id=p.id JOIN project_runtimes r ON r.project_id=p.id JOIN deployments d ON d.id=r.current_deployment_id WHERE p.slug=$1 AND p.status='ready'`, slug).Scan(&app.ID, &app.Slug, &app.Mode, &app.Password, &app.Version, &app.Container)
	return app, err
}

func (s *Service) allowed(ctx context.Context, t ticket, app application) bool {
	if t.Project != app.ID || t.Version != app.Version {
		return false
	}
	if t.Kind == "password" {
		return app.Mode == "password" && app.Password != nil
	}
	if t.Kind == "preview" {
		u, err := s.Store.User(ctx, t.User)
		if err != nil {
			return false
		}
		_, err = s.Store.Project(ctx, u, app.Slug)
		return err == nil
	}
	return false
}

func (s *Service) setAppCookie(w http.ResponseWriter, value string) {
	// Partitioning supports protected cross-origin previews without sharing the
	// platform cookie or relaxing app-origin isolation. HTTPS is mandatory outside localhost.
	http.SetCookie(w, &http.Cookie{Name: appCookie, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteNoneMode, Partitioned: true, MaxAge: 1800})
}

var passwordPage = template.Must(template.New("password").Parse(`<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Protected application</title></head><body style="font-family:system-ui;background:#f5f8f0;color:#234d3d;display:grid;place-items:center;min-height:100vh;margin:0"><main style="padding:30px;max-width:350px"><h1>Password required</h1><p>This application is protected by its owner.</p>{{if .Error}}<p role="alert">{{.Error}}</p>{{end}}<form method="post" action="/__sg1/login"><input type="hidden" name="csrf" value="{{.CSRF}}"><label>Application password<input aria-label="Application password" type="password" name="password" required maxlength="72" style="display:block;padding:12px;margin:12px 0;width:100%;box-sizing:border-box"></label><button style="padding:12px 22px;background:#234d3d;color:white;border:0;border-radius:5px">Open application</button></form></main></body></html>`))

func (s *Service) passwordForm(w http.ResponseWriter, app application, message string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = passwordPage.Execute(w, map[string]string{"Error": message, "CSRF": s.sign(ticket{Project: app.ID, Kind: "login", Version: app.Version, Expires: time.Now().Add(5 * time.Minute).Unix()})})
}

func (s *Service) Gateway(platformOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/__sg1_probe" {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			t, err := s.verify(raw)
			if !ok || err != nil || r.Method != "POST" || t.Kind != "probe" || !strings.HasPrefix(t.Target, "sg1-app-"+t.Project+"-") || strings.Trim(t.Target, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
				http.Error(w, "Probe access denied", 403)
				return
			}
			client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			request, err := http.NewRequestWithContext(r.Context(), "GET", "http://"+t.Target+":8080/healthz", nil)
			if err != nil {
				http.Error(w, "Invalid probe", 400)
				return
			}
			response, err := client.Do(request)
			if err != nil {
				http.Error(w, "Application HTTP health check failed", 503)
				return
			}
			defer response.Body.Close()
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				http.Error(w, "Application HTTP health check failed", 503)
				return
			}
			w.WriteHeader(200)
			return
		}
		if r.URL.Path == "/healthz" && r.Host == "127.0.0.1:8081" {
			w.WriteHeader(200)
			return
		}
		slug := strings.Split(r.Host, ".")[0]
		if domain.ValidateSlug(slug) != nil {
			http.NotFound(w, r)
			return
		}
		expected, err := url.Parse(s.appURL(slug))
		if err != nil || r.Host != expected.Host {
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		app, err := s.lookupApp(ctx, slug)
		cancel()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "frame-ancestors "+platformOrigin+"; base-uri 'self'; form-action 'self'")
		if r.URL.Path == "/__sg1/preview" {
			w.Header().Set("Referrer-Policy", "no-referrer")
			t, err := s.verify(r.URL.Query().Get("ticket"))
			if err != nil || t.Kind != "preview" || !s.allowed(r.Context(), t, app) {
				http.Error(w, "Preview access denied", 403)
				return
			}
			s.setAppCookie(w, s.sign(t))
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if r.URL.Path == "/__sg1/login" {
			if r.Method != "POST" || r.Header.Get("Origin") != expected.Scheme+"://"+expected.Host {
				http.Error(w, "Origin not allowed", 403)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if r.ParseForm() != nil {
				http.Error(w, "Invalid form", 400)
				return
			}
			t, err := s.verify(r.FormValue("csrf"))
			if err != nil || t.Project != app.ID || t.Version != app.Version || t.Kind != "login" {
				http.Error(w, "Invalid login request", 403)
				return
			}
			var attempts int
			err = s.Store.Pool.QueryRow(r.Context(), `INSERT INTO app_password_attempts(project_id,attempts) VALUES($1,1) ON CONFLICT(project_id) DO UPDATE SET attempts=CASE WHEN app_password_attempts.window_start<now()-interval '5 minutes' THEN 1 ELSE app_password_attempts.attempts+1 END,window_start=CASE WHEN app_password_attempts.window_start<now()-interval '5 minutes' THEN now() ELSE app_password_attempts.window_start END RETURNING attempts`, app.ID).Scan(&attempts)
			if err != nil || attempts > 20 {
				http.Error(w, "Too many attempts; try again in five minutes", 429)
				return
			}
			if app.Password == nil || len(r.FormValue("password")) > 72 || bcrypt.CompareHashAndPassword([]byte(*app.Password), []byte(r.FormValue("password"))) != nil {
				s.passwordForm(w, app, "Incorrect password", 401)
				return
			}
			s.setAppCookie(w, s.sign(ticket{Project: app.ID, Kind: "password", Version: app.Version, Expires: time.Now().Add(30 * time.Minute).Unix()}))
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		allowed := app.Mode == "public"
		if cookie, err := r.Cookie(appCookie); err == nil {
			if t, err := s.verify(cookie.Value); err == nil && s.allowed(r.Context(), t, app) {
				allowed = true
			}
		}
		if !allowed {
			if r.Method == "GET" && strings.Contains(r.Header.Get("Accept"), "text/html") {
				s.passwordForm(w, app, "", 401)
			} else {
				http.Error(w, "Application authentication required", 401)
			}
			return
		}
		target := &url.URL{Scheme: "http", Host: app.Container + ":8080"}
		proxy := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("Cookie")
			for _, cookie := range pr.In.Cookies() {
				if cookie.Name != appCookie && !strings.HasPrefix(cookie.Name, "sg1_") {
					pr.Out.AddCookie(cookie)
				}
			}
		}, ModifyResponse: func(response *http.Response) error {
			response.Header.Del("X-Frame-Options")
			response.Header.Set("Content-Security-Policy", "frame-ancestors "+platformOrigin)
			cookies := response.Cookies()
			response.Header.Del("Set-Cookie")
			for _, cookie := range cookies {
				if cookie.Name == appCookie || strings.HasPrefix(cookie.Name, "sg1_") {
					continue
				}
				cookie.Domain = ""
				response.Header.Add("Set-Cookie", cookie.String())
			}
			return nil
		}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "Application temporarily unavailable", 503)
		}}
		proxy.ServeHTTP(w, r)
	})
}

func (s *Service) probe(ctx context.Context, projectID, container string) error {
	if s.GatewayURL == "" {
		return errors.New("trusted HTTP health probe is not configured")
	}
	for range 5 {
		t := s.sign(ticket{Project: projectID, Target: container, Kind: "probe", Expires: time.Now().Add(time.Minute).Unix()})
		r, err := http.NewRequestWithContext(ctx, "POST", s.GatewayURL+"/__sg1_probe", nil)
		if err != nil {
			return err
		}
		r.Header.Set("Authorization", "Bearer "+t)
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Do(r)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("independent HTTP /healthz check failed; application was not published")
}
