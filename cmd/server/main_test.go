package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheckHandler(t *testing.T) {
	twoHundreds := map[string]http.HandlerFunc{
		"/healthz": healthCheckHandler,
		"/":        rootHandler,
	}

	for route, handler := range twoHundreds {
		t.Run(route, func(t *testing.T) {

			// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
			// pass 'nil' as the third parameter.
			req, err := http.NewRequest("GET", route, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}

			// We create a ResponseRecorder (which satisfies http.ResponseWriter) to
			// record the response.
			rr := httptest.NewRecorder()
			handlerFunc := http.HandlerFunc(handler)

			// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
			// directly and pass in our Request and ResponseRecorder.
			handlerFunc.ServeHTTP(rr, req)

			// Check the status code is what we expect.
			if status := rr.Code; status != http.StatusOK {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, http.StatusOK)
			}
		})
	}
}
