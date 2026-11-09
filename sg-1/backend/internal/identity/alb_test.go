package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(t *testing.T) (*Verifier, *ecdsa.PrivateKey, header) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	v := &Verifier{Region: "us-east-1", SignerARN: "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/sg1/abc", Issuer: "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_test", ClientID: "test-client"}
	v.HTTP = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "public-keys.auth.elb.us-east-1.amazonaws.com" {
			t.Fatal("untrusted key URL")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})}
	return v, key, header{Algorithm: "ES256", KeyID: "11111111-1111-4111-8111-111111111111", Signer: v.SignerARN, Issuer: v.Issuer, ClientID: v.ClientID, Expires: time.Now().Add(time.Minute).Unix()}
}
func sign(t *testing.T, key *ecdsa.PrivateKey, h header, c Claims) string {
	t.Helper()
	head, _ := json.Marshal(h)
	body, _ := json.Marshal(c)
	payload := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(body)
	digest := sha256.Sum256([]byte(payload))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	return payload + "." + base64.RawURLEncoding.EncodeToString(signature)
}
func TestALBIdentityVerification(t *testing.T) {
	v, key, h := fixture(t)
	raw := sign(t, key, h, Claims{Subject: "employee-subject", Email: "person@example.com", Name: "Employee"})
	claims, err := v.Verify(context.Background(), raw)
	if err != nil || claims.Subject != "employee-subject" {
		t.Fatal("valid identity rejected", err)
	}
	for _, name := range []string{"signer", "issuer", "client", "algorithm", "expiry", "key-id", "subject", "signature"} {
		t.Run(name, func(t *testing.T) {
			v, key, h := fixture(t)
			c := Claims{Subject: "employee-subject"}
			switch name {
			case "signer":
				h.Signer = "another-alb"
			case "issuer":
				h.Issuer = "https://attacker.example"
			case "client":
				h.ClientID = "another-client"
			case "algorithm":
				h.Algorithm = "none"
			case "expiry":
				h.Expires = time.Now().Add(-time.Minute).Unix()
			case "key-id":
				h.KeyID = "../../metadata"
			case "subject":
				c.Subject = ""
			}
			raw := sign(t, key, h, c)
			if name == "signature" {
				other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				raw = sign(t, other, h, c)
			}
			if _, err := v.Verify(context.Background(), raw); err == nil {
				t.Fatal("invalid identity was accepted")
			}
		})
	}
}
func TestALBRejectsUnsignedHeadersAndOversizedTokens(t *testing.T) {
	v, _, _ := fixture(t)
	for _, raw := range []string{"", "employee@example.com", "header.payload", strings.Repeat("a", 25000)} {
		if _, err := v.Verify(context.Background(), raw); err == nil {
			t.Fatal("unverified identity accepted")
		}
	}
}
