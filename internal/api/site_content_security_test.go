package api

import (
	"estacionamienti/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSiteContentWriteRequiresAuthentication(t *testing.T) {
	handler := auth.AdminAuthMiddleware(http.HandlerFunc((&SiteContentHandler{}).Save))
	for _, token := range []string{"", "Bearer invalid"} {
		req := httptest.NewRequest(http.MethodPut, "/admin/site-content", strings.NewReader(`{"language":"it","section":"Header","fields":{"Header.title":"Test"}}`))
		req.Header.Set("Authorization", token)
		t.Setenv("JWT_SECRET", "test-only-secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", response.Code)
		}
	}
}

func TestSiteContentRejectsUnsafePathsAndNul(t *testing.T) {
	for _, key := range []string{"Header.__proto__.value", "Header.constructor.x", "Header..title"} {
		if validateSiteContentSave(siteContentSave{Language: "it", Section: "Header", Fields: map[string]string{key: "text"}}) == nil {
			t.Fatalf("accepted unsafe key %q", key)
		}
	}
	if validateSiteContentSave(siteContentSave{Language: "it", Section: "Header", Fields: map[string]string{"Header.title": "bad\x00value"}}) == nil {
		t.Fatal("accepted NUL")
	}
}

func TestSiteContentRejectsMalformedBodiesBeforeDatabase(t *testing.T) {
	for _, body := range []string{`{`, `{"language":"it","section":"Header","fields":{"Header.title":"ok"},"unknown":1}`, `{"language":"it","section":"Header","fields":{"Header.title":"ok"}} {}`} {
		response := httptest.NewRecorder()
		(&SiteContentHandler{}).Save(response, httptest.NewRequest(http.MethodPut, "/admin/site-content", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", response.Code)
		}
	}
}
