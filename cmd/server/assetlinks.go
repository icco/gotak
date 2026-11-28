package main

import (
	"net/http"
	"os"
	"strings"

	"go.icco.me/gutil/logging"
	"go.uber.org/zap"
)

const androidPackage = "com.timeclimbers.gotak"

// AssetLink is one statement in /.well-known/assetlinks.json, which lets
// Android open gotak.app game links in the app.
type AssetLink struct {
	Relation []string        `json:"relation"`
	Target   AssetLinkTarget `json:"target"`
}

// AssetLinkTarget names the Android app and its signing certificates.
type AssetLinkTarget struct {
	Namespace              string   `json:"namespace"`
	PackageName            string   `json:"package_name"`
	SHA256CertFingerprints []string `json:"sha256_cert_fingerprints"`
}

func assetLinksHandler(w http.ResponseWriter, r *http.Request) {
	l := logging.FromContext(r.Context())
	var prints []string
	for _, p := range strings.Split(os.Getenv("ANDROID_CERT_SHA256"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			prints = append(prints, p)
		}
	}
	if len(prints) == 0 {
		notFoundHandler(w, r)
		return
	}
	if err := Renderer.JSON(w, http.StatusOK, []AssetLink{{
		Relation: []string{"delegate_permission/common.handle_all_urls"},
		Target: AssetLinkTarget{
			Namespace:              "android_app",
			PackageName:            androidPackage,
			SHA256CertFingerprints: prints,
		},
	}}); err != nil {
		l.Errorw("failed to render JSON", zap.Error(err))
	}
}
