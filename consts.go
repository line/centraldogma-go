package centraldogma

import (
	"fmt"
)

var (
	ErrLatestNotSet = fmt.Errorf("latest is not set yet")

	ErrQueryMustBeSet = fmt.Errorf("query should not be nil")

	ErrWatcherClosed = fmt.Errorf("watcher is closed")

	ErrTokenEmpty = fmt.Errorf("token should not be empty")

	ErrTransportMustBeSet = fmt.Errorf("transport should not be nil")

	ErrTransportMustNotBeOAuth2 = fmt.Errorf("transport cannot be oauth2.Transport")

	ErrMetricCollectorConfigMustBeSet = fmt.Errorf("metric collector config should not be nil")
)

const (
	DefaultChannelBuffer = 128

	UnknownHttpStatusCode = 0

	DefaultClientName = "centralDogmaClient"
)
