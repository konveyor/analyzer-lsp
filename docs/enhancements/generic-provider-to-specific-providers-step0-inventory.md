# Step 0 inventory: generic → specific providers

**Repo:** `konveyor/analyzer-lsp` (workspace path at inventory time)  
**Purpose:** Exhaustive in-repo reference map for implementing [generic-provider-to-specific-providers-implementation-plan.md](./generic-provider-to-specific-providers-implementation-plan.md).  
**Method:** `rg` / ripgrep-style searches for symbols and paths; manual spot-check of `Makefile` and workflows.

---

## 1. Summary counts (approximate)

| Category | Count / note |
|----------|----------------|
| Files under `external-providers/generic-external-provider/` | Entire tree is migration/removal target (see §2) |
| Non-doc hits for string `generic-external-provider` (excluding `docs/enhancements/*`) | Code, Docker, Makefile, workflows, configs |
| `SupportedLanguages` definition | Single map: `pkg/server_configurations/constants.go` |
| `yaml_language_server` package | `pkg/server_configurations/yaml_language_server/` (+ tests, constants wiring) |

---

## 2. Generic provider module (`external-providers/generic-external-provider/`)

| Path | Role |
|------|------|
| `main.go` | Flags (`--name` → LSP server name), builds `NewGenericProvider` |
| `go.mod` / `go.sum` | Separate Go module with `replace` to analyzer root |
| `Dockerfile` | Multi-stage build; output binary `/usr/local/bin/generic-external-provider`; copies `entrypoint.sh`; **build-arg** `GOLANG_DEP_IMAGE` |
| `entrypoint.sh` | Referenced in Dockerfile / local runs (Go image uses entrypoint for gopls flow) |
| `docs/README.md` | User-facing generic provider docs; `SupportedLanguages` extension instructions |
| `pkg/generic_external_provider/provider.go` | Dispatcher: `SupportedLanguages`, `Init` switch on `lspServerName`, YAML LSP branch |
| `pkg/server_configurations/constants.go` | **`SupportedLanguages` map**; imports all builders including `yaml_language_server` |
| `pkg/server_configurations/generic/` | gopls / `"generic"` LSP client |
| `pkg/server_configurations/pylsp/` | Python LSP client |
| `pkg/server_configurations/nodejs/` | Node LSP client + `symbol_cache_helper.go` |
| `pkg/server_configurations/yaml_language_server/` | YAML LSP (to drop per enhancement; use **yq** only) |
| `pkg/server_configurations/*/service_client_test.go` | Unit tests per language + yaml |
| `e2e-tests/golang-e2e/` | `demo-output.yaml`, `provider_settings.json`, `rule-example.yaml` |
| `e2e-tests/python-e2e/` | Same pattern |
| `e2e-tests/nodejs-e2e/` | Same pattern; rules use capabilities such as `nodejs.referenced` |

---

## 3. Analyzer core (spawn + naming)

| File | What to revisit |
|------|-----------------|
| `provider/grpc/provider.go` | `start()`: default `name := "generic"`; sets `--name` from `initConfig[0].ProviderSpecificConfig["lspServerName"]` when present |
| `provider/provider_test.go` | Test data includes `"lspServerName": "generic"` (lines ~540, 557) |

No other Go packages **import** the generic external provider module from outside `external-providers/generic-external-provider/` (dispatcher is self-contained in that module).

**Comment-only reference:** `lsp/base_service_client/base_service_client.go` (~432) mentions `generic-external-provider` in a comment; `LspServerName` field comment (~40) uses `yaml_language_server` as an example string.

---

## 4. Default and example provider settings (binary paths + `lspServerName`)

| File | Content |
|------|---------|
| `provider_container_settings.json` | Three blocks use `binaryPath` **`/usr/local/bin/generic-external-provider`** with `lspServerName` **`generic`**, **`pylsp`**, **`nodejs`**; Java uses `java-external-provider` |
| `provider_pod_local_settings.json` | Same pattern for local/pod workflows |
| `provider_local_external_images.json` | Same pattern; likely used when images are substituted by env |
| `provider/testdata/provider_settings_simple.yaml` | `binaryPath: /usr/bin/generic-external-provider`, `lspServerName: generic` |
| `provider/testdata/provider_settings_nested_types.json` | `generic-external-provider`, `lspServerName: generic` |
| `provider/testdata/provider_settings_invalid.yaml` | Same binary path pattern (invalid fixture) |
| `external-providers/generic-external-provider/e2e-tests/*/provider_settings.json` | Per-language e2e configs + `binaryPath` / `lspServerName` |

---

## 5. Build: root `Makefile`

| Symbol / target | Notes |
|-----------------|--------|
| `IMG_GENERIC_PROVIDER` | `localhost/generic-provider:$(TAG)` |
| `build` | Depends on `external-generic` among others |
| `external-generic` | `cd external-providers/generic-external-provider && ... go build -o ../../build/generic-external-provider main.go` |
| `build-external` | `image-build build-generic-provider build-java-provider build-yq-provider` |
| `build-generic-provider` | `podman build ... generic-external-provider/Dockerfile` with `GOLANG_DEP_IMAGE` |
| `run-external-providers-local` | Three runs of **`$(IMG_GENERIC_PROVIDER)`** with different `--name` (golang entrypoint, nodejs, pylsp) |
| `run-external-providers-pod` | Same three containers from **`IMG_GENERIC_PROVIDER`** |
| `run-generic-golang-provider-pod` / `run-generic-python-provider-pod` / `run-generic-nodejs-provider-pod` | Single-language pods for targeted demos |
| `run-demo-generic-golang` / `python` / `nodejs` | Mount e2e files from **`generic-external-provider/e2e-tests/...`** |
| `stop-generic-*-provider-pod` | Tear down |
| `test-all-providers` | `test-java test-generic test-yaml` |
| `test-generic` | Runs `test-nodejs`, `test-python`, `test-golang` (sequential) |
| `test-golang` / `test-python` / `test-nodejs` | Each uses generic image + generic e2e paths |

**Note:** `external-providers/TESTING.md` documents targets like `run-go-provider-pod` and `run-demo-go`, which **do not match** current Makefile names (`run-generic-golang-provider-pod`, `run-demo-generic-golang`). Update docs when refactoring.

---

## 6. Docker / images

| Artifact | Location |
|----------|----------|
| Generic provider image build | `external-providers/generic-external-provider/Dockerfile` |
| Analyzer image | Root `Dockerfile` — builds analyzer + dep only (**no** generic binary in image) |
| `golang-dependency-provider` | Separate image; **required** as build input for generic (and future **Go-only** provider image) |

---

## 7. CI workflows (`.github/workflows/`)

| Workflow | Generic-related content |
|----------|-------------------------|
| **`image-build.yaml`** | Matrix builds analyzer, golang-dependency-provider, yq, java; **sequential job** `generic-external-provider-build` after matrix, `image_name: generic-external-provider`, same Dockerfile, `extra-args` with `GOLANG_DEP_IMAGE` |
| **`demo-testing.yml`** | Path filter `external-providers/generic-external-provider/**`; three matrix rows with `provider: generic`, `artifact_pattern: "*{analyzer-lsp,generic-external-provider}"`, e2e paths under generic; `IMG_GENERIC_PROVIDER` env; podman runs three containers from that image; **ttl.sh** `konveyor-generic-external-provider-${{ github.sha }}`; output `provider_generic` for downstream job |
| **`pr-testing.yml`** | `go test ./...`, `make build` (pulls in **external-generic**), **no** separate `go test` for generic module today—only **java-external-provider** gets extra test steps |
| **`java-provider-image-build.yaml`** | No generic reference (Java-only) |
| **`benchmark.yml`**, **`verifyPR.yml`**, **`stale.yml`**, **`issues.yaml`**, **`pr-closed.yaml`** | No `generic-external-provider` hits in grep |

---

## 8. Documentation (non-enhancement)

| File | Relevance |
|------|-----------|
| `docs/providers.md` | Describes “generic provider binary”, `lspServerName`, examples |
| `docs/development/testing.md` | `localhost/generic-provider:latest` for Go/Python/Node |
| `docs/development/setup.md` | Same image reference |
| `docs/development/provider_development.md` | Example `SupportedLanguages` map pattern (educational; align after split) |
| `CONTRIBUTING.md` | Example JSON with `lspServerName: generic` |
| `external-providers/TESTING.md` | Directory tree shows generic e2e layout; **Makefile command names outdated** (see §5) |

---

## 9. Enhancement / planning docs (informational only)

These reference the migration but are **not** runtime targets:

- `docs/enhancements/generic-provider-to-specific-providers.md`
- `docs/enhancements/generic-provider-to-specific-providers-implementation-plan.md`
- `docs/enhancements/generic-provider-to-specific-providers-downstream-tracking.md`
- `docs/enhancements/generic-provider-to-specific-providers-pr262-review-follow-ups.md`

---

## 10. Downstream (out of tree)

Not present in this repo; track in [generic-provider-to-specific-providers-downstream-tracking.md](./generic-provider-to-specific-providers-downstream-tracking.md):

- **Konveyor operator** — Python / Node extension images (@jortel)
- **Kantra / hub / kai** — embedded image names or defaults (confirm with owners)
- **quay.io** image names — today include `quay.io/konveyor/generic-external-provider` (via release tooling); will become **three** images

---

## 11. Post–migration checklist (for Step 11 verification)

Use this as a final grep gate before deleting `generic-external-provider/`:

- [ ] No remaining `generic-external-provider` in `Makefile`, `.github/workflows/`, `provider_container_settings.json`, `provider_pod_local_settings.json`, `provider_local_external_images.json`, `provider/testdata/`
- [ ] No `IMG_GENERIC_PROVIDER` / `build-generic-provider` / `external-generic`
- [ ] `demo-testing.yml` has no `generic` matrix rows pointing at old paths
- [ ] `image-build.yaml` has no `generic-external-provider-build` job
- [ ] Docs and `CONTRIBUTING.md` examples updated
- [ ] `pr-testing.yml`: `make build` updated to build new provider binaries instead of/in addition to generic
- [ ] `SupportedLanguages` / `generic_external_provider` package gone or unused

---

## 12. Open inventory notes

- **Provider capability names in rules** (e.g. `nodejs.referenced`, `golang.*`) must stay consistent with `lspServerName` / analyzer expectations when renaming the Go provider from `generic` → `golang` (if done).
- **`go test ./...` at repo root** does not recurse into `external-providers/generic-external-provider` as a separate module; generic tests run when testing that module explicitly or via Makefile—**new** providers will need the same discipline as Java (`cd ... && go test`).
- **`golang-dependency-provider/e2e-tests/`** exists separately from generic; do not conflate with Go **LSP** provider e2e under generic.

---

*Inventory generated as Step 0 of the implementation plan. Update this file if the repo changes before implementation starts.*

---

## Appendix: Step 1 scaffolds (per-language modules)

Added under `external-providers/` (stub `BaseClient`, `Init` returns “not implemented”; included in `make build`):

| Directory | Binary output (`build/`) |
|-----------|-------------------------|
| `go-external-provider/` | `go-external-provider` |
| `python-external-provider/` | `python-external-provider` |
| `nodejs-external-provider/` | `nodejs-external-provider` |

Each has `main.go`, `go.mod` / `go.sum`, `Dockerfile` (binary-only final image), and `pkg/<name>_external_provider/stub_provider.go`.
