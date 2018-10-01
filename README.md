# Central Dogma client for Go

- [Central Dogma](https://line.github.io/centraldogma/)
- [GoDoc](https://godoc.org/go.linecorp.com/centraldogma)
- [CLI manual](https://line.github.io/centraldogma/client-cli.html)

## How to build

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

# Build the client library.
$ cd "$GOPATH/src/go.linecorp.com/centraldogma"
$ go build

# Build the CLI.
$ cd "$GOPATH/src/go.linecorp.com/centraldogma/internal/app/dogma"
$ go build
$ ./dogma -help
```