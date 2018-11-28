package centraldogma

import (
	"fmt"
)

var (
	ErrLatestNotSet = fmt.Errorf("latest is not set yet")

	ErrQueryShouldSet = fmt.Errorf("query should not be nil")

	ErrWatcherIsClosed = fmt.Errorf("watcher is closed")
)
