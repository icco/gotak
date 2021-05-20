package gotak

import "github.com/icco/gutil/logging"

const (
	// GCPProject is the project this runs in.
	GCPProject = "icco-cloud"

	// Service is the name of this service.
	Service = "gotak"
)

var (
	log = logging.Must(logging.NewLogger(Service))
)
