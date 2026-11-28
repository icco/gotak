package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssetLinksHandler(t *testing.T) {
	get := func() *httptest.ResponseRecorder {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/.well-known/assetlinks.json", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		rr := httptest.NewRecorder()
		assetLinksHandler(rr, req)
		return rr
	}

	t.Setenv("ANDROID_CERT_SHA256", "")
	if rr := get(); rr.Code != http.StatusNotFound {
		t.Fatalf("unset fingerprint: got %d, want 404", rr.Code)
	}

	t.Setenv("ANDROID_CERT_SHA256", "AA:BB, CC:DD ,")
	rr := get()
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	var links []AssetLink
	if err := json.Unmarshal(rr.Body.Bytes(), &links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].Target.PackageName != androidPackage {
		t.Fatalf("unexpected links: %+v", links)
	}
	if got := links[0].Target.SHA256CertFingerprints; len(got) != 2 || got[0] != "AA:BB" || got[1] != "CC:DD" {
		t.Fatalf("fingerprints = %v", got)
	}
}
