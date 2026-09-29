package gotak

import "go.icco.me/gutil/logging"

const (
	// Service is the name of this service.
	Service = "gotak"
)

var (
	log = logging.Must(logging.NewLogger(Service))
)
