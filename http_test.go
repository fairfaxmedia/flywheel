package flywheel

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// MockedHandler to verify if non 200 http codes return unmodified values
func MockedHandler(w http.ResponseWriter, r *http.Request) {

	if strings.HasPrefix(r.URL.Path, "/302") {
		w.Header().Add("Location", "http://dev.zero/fakeRedirectHandler")
		w.WriteHeader(http.StatusFound)
	} else if strings.HasPrefix(r.URL.Path, "/404") {
		w.WriteHeader(http.StatusNotFound)
	} else if strings.HasPrefix(r.URL.Path, "/500") {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, r.Body)
}

func TestProxyFunction(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(MockedHandler))

	defer server.Close()

	// Reroute all traffic to the test server
	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	fw := Flywheel{
		config: &Config{
			Vhosts: map[string]string{"www.example.org": "www.backend.example.org"},
		},
	}

	handler := NewHandler(&fw)
	handler.HTTPClient.Transport = transport

	testTable := []struct {
		url, host, method string
		code              int
	}{
		{
			"/302something?flywheel=start",
			"www.example.org",
			"GET",
			302,
		},
		{
			"/404not_found?flywheel=start",
			"www.example.org",
			"GET",
			404,
		},
		{
			"/500error_buddy",
			"www.example.org",
			"GET",
			500,
		},
		{
			"/all_good_mate",
			"www.example.org",
			"GET",
			200,
		},
	}

	for _, tt := range testTable {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(tt.method, tt.url, nil)
		req.Host = tt.host

		handler.proxy(w, req)
		if w.Code != tt.code {
			t.Errorf("Expexted code %d, but got %d", tt.code, w.Code)
		}
	}
}
