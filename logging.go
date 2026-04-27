package gotak

import "github.com/icco/gutil/logging"

const (
	// Service is the name of this service.
	Service = "gotak"
)

var (
	log = logging.Must(logging.NewLogger(Service))
)
