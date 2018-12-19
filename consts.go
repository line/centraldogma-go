package centraldogma

import (
	"fmt"
)

var (
	ErrLatestNotSet = fmt.Errorf("latest is not set yet")

	ErrQueryMustBeSet = fmt.Errorf("query should not be nil")

	ErrWatcherClosed = fmt.Errorf("watcher is closed")
)

const (
	DefaultChannelBuffer = 128

	UnknownHttpStatusCode = 0
)
