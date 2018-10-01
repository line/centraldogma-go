module go.linecorp.com/centraldogma/internal/app/dogma

require (
	github.com/fhs/go-netrc v1.0.0
	github.com/sirupsen/logrus v1.1.0 // indirect
	github.com/urfave/cli v1.19.1
	go.linecorp.com/centraldogma v0.0.0-19700101000000-0000000000000000000000000000000000000000 // local reference
	golang.org/x/crypto v0.0.0-20180927165925-5295e8364332db77d75fce11f1d19c053919a9c9
)

replace go.linecorp.com/centraldogma => ../../..
