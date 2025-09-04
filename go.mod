module github.com/konveyor/analyzer-lsp

go 1.23.9

require (
	github.com/PaesslerAG/gval v1.2.2
	github.com/antchfx/jsonquery v1.3.0
	github.com/antchfx/xmlquery v1.3.12
	github.com/bombsimon/logrusr/v3 v3.1.0
	github.com/dlclark/regexp2 v1.11.4
	github.com/go-logr/logr v1.4.2
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/jhump/protoreflect v1.16.0
	github.com/phayes/freeport v0.0.0-20220201140144-74d24b5ae9f5
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v1.9.1
	github.com/swaggest/jsonschema-go v0.3.70
	github.com/swaggest/openapi-go v0.2.50
	go.lsp.dev/uri v0.3.0
	go.opentelemetry.io/otel/trace v1.34.0
	golang.org/x/oauth2 v0.26.0
	golang.org/x/sync v0.11.0
	google.golang.org/grpc v1.72.2
	google.golang.org/protobuf v1.36.5
	gopkg.in/yaml.v2 v2.4.0
)

require (
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	github.com/bufbuild/protocompile v0.10.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/swaggest/refl v1.3.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250218202821-56aae31c358a // indirect
)

require (
	github.com/antchfx/xpath v1.3.3
	github.com/cbroglie/mustache v1.4.0
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-version v1.6.0
	github.com/shopspring/decimal v1.3.1 // indirect
	go.opentelemetry.io/otel v1.34.0
	go.opentelemetry.io/otel/exporters/jaeger v1.17.0
	go.opentelemetry.io/otel/sdk v1.34.0
	golang.org/x/net v0.35.0
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
)

replace github.com/spf13/cobra v1.3.0 => github.com/spf13/cobra v1.9.1

replace github.com/antchfx/xmlquery => github.com/aufi/xmlquery v0.0.0-20250819124127-bd4beb3bd7a5
