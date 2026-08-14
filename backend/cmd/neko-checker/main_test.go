package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCurrentMonthRangeUsesPortalDayMonthYearFormat(t *testing.T) {
	now := time.Date(2026, 8, 14, 4, 15, 0, 0, time.FixedZone("WIB", 7*60*60))
	start, end := currentMonthRange(now)
	if start != "01/08/2026" || end != "14/08/2026" {
		t.Fatalf("range=%q..%q", start, end)
	}
}

func TestDecodePortalStateRejectsUnexpectedCDPValue(t *testing.T) {
	if _, err := decodePortalState(true); err == nil {
		t.Fatal("expected unexpected CDP value type to be rejected")
	}
}

func TestAuthenticatedHistoryPageDoesNotNavigateAgain(t *testing.T) {
	state := portalState{Authenticated: true, HistoryReady: true}
	if requiresHistoryNavigation(state) {
		t.Fatal("ready history page must be reused instead of reloading and waiting again")
	}
}

func TestHistoryRefreshWaitsForNewDataTablesDrawAndCompletedRequest(t *testing.T) {
	expression := historyRefreshCompleteExpression(7)
	for _, expected := range []string{"iDraw > 7", "readyState===4"} {
		if !strings.Contains(expression, expected) {
			t.Fatalf("refresh completion expression %q does not contain %q", expression, expected)
		}
	}
}

func TestSelectPortalPagePrefersAuthenticatedDashboard(t *testing.T) {
	page, ok := selectPortalPage([]pageTarget{
		{ID: "blank", Type: "page", URL: "about:blank"},
		{ID: "login", Type: "page", URL: "https://merchant.qris.interactive.co.id/v2/m/login/"},
		{ID: "dashboard", Type: "page", URL: "https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/summary.php"},
	})
	if !ok || page.ID != "dashboard" {
		t.Fatalf("selected=%+v ok=%v", page, ok)
	}
}

func TestNormalizeWebSocketURLUsesInternalRelay(t *testing.T) {
	got := normalizeWebSocketURL("ws://127.0.0.1:9223/devtools/page/abc", "http://neko:9222")
	if got != "ws://neko:9222/devtools/page/abc" {
		t.Fatalf("url=%q", got)
	}
}

func TestRunFallsBackToX11HelperWhenCDPEndpointReturns500(t *testing.T) {
	cdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusInternalServerError)
	}))
	defer cdp.Close()

	previous := os.Getenv("NEKO_CDP_URL")
	t.Setenv("NEKO_CDP_URL", cdp.URL)
	defer os.Setenv("NEKO_CDP_URL", previous)

	called := false
	run(request{MerchantID: "merchant-1"}, func(input request) {
		called = input.MerchantID == "merchant-1"
	})
	if !called {
		t.Fatal("expected CDP failure to fall back to the Neko X11 helper")
	}
}
