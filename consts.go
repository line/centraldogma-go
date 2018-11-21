package centraldogma

import (
	"fmt"
)

var (
	// ErrLatestNotSet latest not set
	ErrLatestNotSet = fmt.Errorf("latest is not set yet")

	// ErrQueryShouldSet indicates nil query
	ErrQueryShouldSet = fmt.Errorf("query should not be nil")

	// ErrWatcherIsClosed indicates watcher is closed
	ErrWatcherIsClosed = fmt.Errorf("watcher is closed")
)
