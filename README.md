# Central Dogma client for Go

- [Central Dogma](https://line.github.io/centraldogma/)
- [GoDoc](https://godoc.org/go.linecorp.com/centraldogma)
- [CLI manual](https://line.github.io/centraldogma/client-cli.html)

## Usage

### Getting started

```go
import "go.linecorp.com/centraldogma"

// create a client with OAuth2 token
// See also: https://line.github.io/centraldogma/auth.html#application-token
centraldogma.NewClientWithToken(baseURL, token, nil)
```

### Customize transport

If transport is `nil` (like above), `http2.Transport` is used by default.

You could inject your own transport easily:

```go
import "golang.org/x/net/http2"

tr := &http2.Transport{
    DisableCompression: false,
}

// create a client with custom transport
centraldogma.NewClientWithToken(baseURL, token, tr)
```

### Example

```go
package sample

import (
	"context"
	"sync/atomic"
	"time"

	"go.linecorp.com/centraldogma"
)

// CentralDogmaFile represents a file in application repository, stored on Central Dogma server.
type CentralDogmaFile struct {
	client            atomic.Value
	BaseURL           string       `yaml:"base_url" json:"base_url"`
	Token             string       `yaml:"token" json:"token"`
	Project           string       `yaml:"project" json:"project"`
	Repo              string       `yaml:"repo" json:"repo"`
	Path              string       `yaml:"path" json:"path"`
	LastKnownRevision atomic.Value `yaml:"-" json:"-"`
	TimeoutSec        int          `yaml:"timeout_sec" json:"timeout_sec"`
}

func (c *CentralDogmaFile) getClientOrSet() (*centraldogma.Client, error) {
	if v, stored := c.client.Load().(*centraldogma.Client); stored {
		return v, nil
	}

	// create a new client
	dogmaClient, err := centraldogma.NewClientWithToken(c.BaseURL, c.Token, nil)
	if err != nil {
		return nil, err
	}

	// store
	c.client.Store(dogmaClient)

	return dogmaClient, nil
}

// Fetch file content from Central Dogma.
func (c *CentralDogmaFile) Fetch(ctx context.Context) (b []byte, err error) {
	dogmaClient, err := c.getClientOrSet()
	if err != nil {
		return
	}

	entry, _, err := dogmaClient.GetFile(ctx, c.Project, c.Repo, "", &centraldogma.Query{
		Path: c.Path,
		Type: centraldogma.Identity,
	})
	if err != nil {
		return
	}

	// set last known revision
	c.LastKnownRevision.Store(entry.Revision)

	b = entry.Content
	return
}

// Watch changes on remote file.
func (c *CentralDogmaFile) Watch(ctx context.Context, callback func([]byte)) error {
	dogmaClient, err := c.getClientOrSet()
	if err != nil {
		return err
	}

	ch, closer, err := dogmaClient.WatchFile(ctx, c.Project, c.Repo, &centraldogma.Query{
		Path: c.Path,
		Type: centraldogma.Identity,
	}, time.Duration(c.TimeoutSec)*time.Second)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				closer()
				return

			case changes := <-ch:
				if changes.Err == nil {
					callback(changes.Entry.Content)
				}
			}
		}
	}()

	return nil
}
```

### Building CLI

We use [Go Modules](https://github.com/golang/go/wiki/Modules) (formerly known as `vgo`) to manage the dependencies.

```
# Opt-in for Go Modules.
$ export GO111MODULE=on

# Set up the GOPATH.
$ mkdir go
$ export GOPATH="$(pwd)/go"

# Clone the repository.
$ cd "$GOPATH"
$ git clone https://github.com/line/centraldogma-go src/go.linecorp.com/centraldogma

# Build the CLI.
$ cd "$GOPATH/src/go.linecorp.com/centraldogma/internal/app/dogma"
$ go build
$ ./dogma -help
```