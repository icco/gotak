package main

import (
	"net/http"
	"os"

	auth2 "github.com/go-pkgz/auth/v2"
	"github.com/go-pkgz/auth/v2/provider/google"
)

func newAuthService() *auth2.Service {
	issuer := "gotak-app"
	secret := os.Getenv("AUTH_JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-change-me" // fallback for dev
	}
	return auth2.NewService(auth2.Opts{
		SecretReader: auth2.SecretFunc(func(id string) (string, error) { return secret, nil }),
		TokenDuration: 24 * 60 * 60, // 1 day
		Issuer:        issuer,
		URL:           "https://gotak.app", // change for local/dev
		Validator:     auth2.DefaultValidator,
		DisableXSRF:   true, // for API only
	}).
		WithProvider(google.NewProvider(google.Opts{
			ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
			ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		}))
}

func AuthRoutes() http.Handler {
	auth := newAuthService()
	return auth.Handlers()
}
