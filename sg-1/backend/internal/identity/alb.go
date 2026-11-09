// Package identity authenticates Cognito-federated Entra ID and OneLogin users.
// Only ES256 claims signed by the configured ALB are accepted, never unsigned
// x-amzn-oidc-identity/email headers or automatic email-based account linking.
package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/store"
	"github.com/jackc/pgx/v5"
)

var errInvalid = errors.New("invalid employee identity")
var keyIDPattern = regexp.MustCompile(`^[a-fA-F0-9-]{36}$`)

type cachedKey struct {
	key     *ecdsa.PublicKey
	expires time.Time
}
type Verifier struct {
	Region, SignerARN, Issuer, ClientID string
	HTTP                                *http.Client
	mu                                  sync.Mutex
	keys                                map[string]cachedKey
}
type Claims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
}
type header struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	Signer    string `json:"signer"`
	Issuer    string `json:"iss"`
	ClientID  string `json:"client"`
	Expires   int64  `json:"exp"`
}

func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	var claims Claims
	if len(raw) > 24<<10 {
		return claims, errInvalid
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return claims, errInvalid
	}
	decode := func(raw string) ([]byte, error) {
		return base64.RawURLEncoding.DecodeString(strings.TrimRight(raw, "="))
	}
	data, err := decode(parts[0])
	if err != nil {
		return claims, errInvalid
	}
	var h header
	if json.Unmarshal(data, &h) != nil || h.Algorithm != "ES256" || !keyIDPattern.MatchString(h.KeyID) ||
		h.Signer != v.SignerARN || h.Issuer != v.Issuer || h.ClientID != v.ClientID || h.Expires <= time.Now().Unix() {
		return claims, errInvalid
	}
	signature, err := decode(parts[2])
	if err != nil || len(signature) != 64 {
		return claims, errInvalid
	}
	key, err := v.publicKey(ctx, h.KeyID)
	if err != nil {
		return claims, errInvalid
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		return claims, errInvalid
	}
	data, err = decode(parts[1])
	if err != nil || json.Unmarshal(data, &claims) != nil || claims.Subject == "" || len(claims.Subject) > 256 {
		return Claims{}, errInvalid
	}
	return claims, nil
}

func (v *Verifier) publicKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if item, ok := v.keys[kid]; ok && time.Now().Before(item.expires) {
		return item.key, nil
	}
	client := v.HTTP
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	request, err := http.NewRequestWithContext(ctx, "GET", "https://public-keys.auth.elb."+v.Region+".amazonaws.com/"+kid, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errInvalid
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<10))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errInvalid
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errInvalid
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errInvalid
	}
	if len(v.keys) >= 16 {
		v.keys = nil
	}
	if v.keys == nil {
		v.keys = map[string]cachedKey{}
	}
	v.keys[kid] = cachedKey{key, time.Now().Add(time.Hour)}
	return key, nil
}

type Authenticator struct {
	Store             *store.Store
	Verifier          *Verifier
	BootstrapSubjects map[string]bool
}

func (a *Authenticator) Authenticate(ctx context.Context, token string) (domain.User, error) {
	c, err := a.Verifier.Verify(ctx, token)
	if err != nil {
		return domain.User{}, err
	}
	issuer := a.Verifier.Issuer
	tx, err := a.Store.Pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize initial identity creation. An email match is never an identity match.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, issuer+"|"+c.Subject); err != nil {
		return domain.User{}, err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT user_id::text FROM user_identities WHERE issuer=$1 AND subject=$2`, issuer, c.Subject).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = "Employee"
		}
		if len(name) > 200 || len(c.Email) > 320 {
			return domain.User{}, errInvalid
		}
		admin := a.BootstrapSubjects[c.Subject]
		err = tx.QueryRow(ctx, `INSERT INTO users(email,name,platform_admin) VALUES($1,$2,$3) RETURNING id::text`, c.Email, name, admin).Scan(&id)
		if err != nil {
			return domain.User{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO user_identities(user_id,issuer,subject) VALUES($1,$2,$3)`, id, issuer, c.Subject); err != nil {
			return domain.User{}, err
		}
		if admin {
			_, err = tx.Exec(ctx, `WITH d AS(INSERT INTO departments(name,slug) VALUES('Workspace','workspace') ON CONFLICT(slug) DO UPDATE SET slug=EXCLUDED.slug RETURNING id) INSERT INTO department_memberships(department_id,user_id,role) SELECT id,$1,'admin' FROM d ON CONFLICT DO NOTHING`, id)
			if err != nil {
				return domain.User{}, err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action) VALUES($1,'identity.created')`, id)
	}
	if err != nil {
		return domain.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return a.Store.User(ctx, id) // Rechecks active status on every request.
}
