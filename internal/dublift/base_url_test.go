package dublift

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDynamicBaseURLResolution(t *testing.T) {
	c, err := OpenConfig(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// 1. Base URL derived from r.Host
	req := httptest.NewRequest("GET", "http://192.168.1.50:7000/media/test/master.m3u8", nil)
	if got := s.base(req); got != "http://192.168.1.50:7000" {
		t.Fatalf("expected http://192.168.1.50:7000, got %q", got)
	}

	req = httptest.NewRequest("GET", "http://localhost:7000/manifest.json", nil)
	if got := s.base(req); got != "http://localhost:7000" {
		t.Fatalf("expected http://localhost:7000, got %q", got)
	}

	// 2. Reverse proxy headers: X-Forwarded-Proto and X-Forwarded-Host
	req = httptest.NewRequest("GET", "http://upstream.local/manifest.json", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "dublift.example.com")
	if got := s.base(req); got != "https://dublift.example.com" {
		t.Fatalf("expected https://dublift.example.com, got %q", got)
	}

	// Multi-value forwarded headers (use first)
	req = httptest.NewRequest("GET", "http://upstream.local/manifest.json", nil)
	req.Header.Set("X-Forwarded-Proto", "https, http")
	req.Header.Set("X-Forwarded-Host", "first.example.com, second.example.com")
	if got := s.base(req); got != "https://first.example.com" {
		t.Fatalf("expected https://first.example.com, got %q", got)
	}

	// 3. Fallback when Host is missing: local socket address via http.LocalAddrContextKey
	req = httptest.NewRequest("GET", "/", nil)
	req.Host = ""
	ctx := context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("192.168.1.77"), Port: 7000})
	req = req.WithContext(ctx)
	if got := s.base(req); got != "http://192.168.1.77:7000" {
		t.Fatalf("expected http://192.168.1.77:7000, got %q", got)
	}

	// Socket address with 0.0.0.0 wildcard normalized to 127.0.0.1
	req = httptest.NewRequest("GET", "/", nil)
	req.Host = ""
	ctx = context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 7000})
	req = req.WithContext(ctx)
	if got := s.base(req); got != "http://127.0.0.1:7000" {
		t.Fatalf("expected http://127.0.0.1:7000, got %q", got)
	}

	// Missing Host and no LocalAddrContextKey: falls back to configured Listen address
	req = httptest.NewRequest("GET", "/", nil)
	req.Host = ""
	// cfg.Listen is 0.0.0.0:7000 by default, normalized to 127.0.0.1:7000
	if got := s.base(req); got != "http://127.0.0.1:7000" {
		t.Fatalf("expected http://127.0.0.1:7000, got %q", got)
	}

	// 4. Explicit override: PublicURL takes precedence over headers and host
	cfg := s.Config.Get()
	cfg.PublicURL = "https://custom.override.org:8443"
	if err := s.Config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("GET", "http://192.168.1.50:7000/manifest.json", nil)
	req.Header.Set("X-Forwarded-Host", "proxy.ignored.com")
	if got := s.base(req); got != "https://custom.override.org:8443" {
		t.Fatalf("expected https://custom.override.org:8443, got %q", got)
	}

	// Trailing slash is trimmed
	cfg.PublicURL = "http://trimmed.override.org:7000/"
	if err := s.Config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if got := s.base(req); got != "http://trimmed.override.org:7000" {
		t.Fatalf("expected http://trimmed.override.org:7000, got %q", got)
	}
}

func TestStatusManifestURLs(t *testing.T) {
	c, err := OpenConfig(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	req := httptest.NewRequest("GET", "http://192.168.1.200:7000/api/status", nil)
	status := s.status(req)

	manifestURL, ok := status["manifestURL"].(string)
	if !ok || manifestURL != "http://192.168.1.200:7000/manifest.json" {
		t.Fatalf("expected manifestURL http://192.168.1.200:7000/manifest.json, got %v", status["manifestURL"])
	}

	localManifestURL, ok := status["localManifestURL"].(string)
	if !ok || localManifestURL != "http://localhost:7000/manifest.json" {
		t.Fatalf("expected localManifestURL http://localhost:7000/manifest.json, got %v", status["localManifestURL"])
	}

	lanManifestURL, ok := status["lanManifestURL"].(string)
	if !ok {
		t.Fatal("expected lanManifestURL to be present in status response")
	}
	// If a LAN IP was detected on the host machine, it should end in :7000/manifest.json
	if lanManifestURL != "" && !strings.HasSuffix(lanManifestURL, ":7000/manifest.json") {
		t.Fatalf("unexpected format for lanManifestURL: %q", lanManifestURL)
	}
}

func TestStatusManifestURLsUseStartedListener(t *testing.T) {
	c, err := OpenConfig(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := c.Get()
	cfg.Listen = "127.0.0.1:7123"
	if err := c.Save(cfg); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// A changed setting is pending restart; setup links must describe the
	// listener this server started with, not the updated config value.
	cfg = c.Get()
	cfg.Listen = "0.0.0.0:8900"
	if err := c.Save(cfg); err != nil {
		t.Fatal(err)
	}
	status := s.status(httptest.NewRequest("GET", "http://localhost:7123/api/status", nil))
	if got := status["localManifestURL"]; got != "http://127.0.0.1:7123/manifest.json" {
		t.Fatalf("expected local manifest URL to use started listener, got %v", got)
	}
	if got := status["lanManifestURL"]; got != "" {
		t.Fatalf("expected no LAN URL for loopback listener, got %v", got)
	}
}
