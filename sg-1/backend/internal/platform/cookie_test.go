package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayProtectsPlatformCookies(t *testing.T) {
	for _, name := range []string{appCookie, "sg1_development_session", "__Host-SG1AuthSession", "__Host-SG1AuthSession-0", "SG1AuthSession-3"} {
		if !reservedAppCookie(name) {
			t.Fatalf("platform cookie not reserved: %s", name)
		}
	}
	if reservedAppCookie("app_preferences") {
		t.Fatal("ordinary app cookie rejected")
	}
	w := httptest.NewRecorder()
	(&Service{}).setAppCookie(w, "signed-ticket")
	c := w.Result().Cookies()[0]
	if c.Name != appCookie || c.Domain != "" || c.Path != "/" || !c.Secure || !c.HttpOnly || !c.Partitioned || c.SameSite != http.SameSiteNoneMode {
		t.Fatal("gateway cookie lost its host-only isolation contract")
	}
}
