# zaphttplog

[![GoDoc](https://pkg.go.dev/badge/github.com/Silicon-Ally/zaphttplog?status.svg)](https://pkg.go.dev/github.com/Silicon-Ally/zaphttplog?tab=doc)
[![CI Workflow](https://github.com/Silicon-Ally/zaphttplog/actions/workflows/test.yml/badge.svg)](https://github.com/Silicon-Ally/zaphttplog/actions?query=branch%3Amain)


`zaphttplog` provides a structured request/response logging implementation based on [zap](https://go.uber.org/zap). It provides much the same functionality and API as [Chi's `httplog`](https://github.com/go-chi/httplog), but backed by zap instead of [zerolog](https://github.com/rs/zerolog).

This is useful for cases where you've standardized on zap for logging and want detailed, structured request logging as part of your HTTP middleware

## Usage

```go
package main

import (
	"log"
	"net/http"

	"github.com/Silicon-Ally/zaphttplog"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func main() {
	// Configure your logger for your environment.
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}

	// Initialize the router and set basic middleware.
	r := chi.NewRouter()
	r.Use(zaphttplog.NewMiddleware(logger))
	r.Use(middleware.Recoverer)

	// An example handler.
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("root."))
	})

	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("http.ListenAndServe: %v", err)
	}
}
```

Running this would look like:

```bash
# Run the server
go run ./examples

# In a new terminal, make a request
curl localhost:8080
```

In the first terminal, you should observe a log line like:

```
{"level":"info","ts":1689792663.2690268,"caller":"zaphttplog/zaphttplog.go:201","msg":"GET / - 200 OK","httpRequest":{"requestURL":"http://localhost:8080/","requestMethod":"GET","requestPath":"/","remoteIP":"127.0.0.1:57954","proto":"HTTP/1.1","scheme":"http","header":{"user-agent":"curl/8.1.2","accept":"*/*"}},"httpResponse":{"status":200,"bytes":5,"elapsed":0.000014111}}
```

## Contributing

Contribution guidelines can be found [on our website](https://siliconally.org/oss/contributor-guidelines).
