module github.com/konveyor/analyzer-lsp/external-providers/java-external-provider

go 1.23.9

toolchain go1.24.3

require (
	github.com/go-logr/logr v1.4.3
	github.com/konveyor/analyzer-lsp v0.7.0-alpha.2.0.20250625194402-05dca9b4ac43
	github.com/swaggest/openapi-go v0.2.58
	go.lsp.dev/uri v0.3.0
	go.opentelemetry.io/otel v1.35.0
	google.golang.org/grpc v1.73.0 // indirect
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/nxadm/tail v1.4.11
	github.com/sirupsen/logrus v1.9.3
	github.com/vifraa/gopom v1.0.0
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250324211829-b45e905df463 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
)

require (
	github.com/PaesslerAG/gval v1.2.4 // indirect
	github.com/bombsimon/logrusr/v3 v3.1.0
	github.com/cbroglie/mustache v1.4.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/hashicorp/go-version v1.7.0
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/swaggest/jsonschema-go v0.3.78 // indirect
	github.com/swaggest/refl v1.4.0 // indirect
	go.opentelemetry.io/otel/exporters/jaeger v1.17.0 // indirect
	go.opentelemetry.io/otel/sdk v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/konveyor/analyzer-lsp => ../../
