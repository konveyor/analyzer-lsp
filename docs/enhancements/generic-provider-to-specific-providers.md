---
title: generic-provider-to-specific-providers
authors:
  - "@jmle"
reviewers:
  - "@shawn-hurley"
  - "@eemcmullan"
approvers:
  - TBD
creation-date: 2026-03-13
last-updated: 2026-03-28
status: provisional
see-also:
  - "https://github.com/konveyor/analyzer-lsp/issues/1013"
  - "refactoring-generic-provider-to-specific-providers.md"
---

# Refactor Generic Service Client to Language-Specific Providers

This enhancement proposes splitting the current "generic external provider" in analyzer-lsp into separate, language-specific providers (Go, Python, Node.js), each with its own binary and initialization path. The goal is to align with the reality that LSP server initialization and behavior differ too much across languages to be reliably abstracted behind a single generic client. Maintainer review ([konveyor/enhancements#262](https://github.com/konveyor/enhancements/pull/262)) settled the following: **fully remove** the generic multi-language binary and dispatcher; ship **one container image per language** (smaller images, simpler debugging, fewer unrelated providers in a given deployment); treat YAML analysis as the responsibility of the existing **yq** provider and **deprecate/remove** the generic provider’s `yaml_language_server` path rather than spinning up a parallel YAML LSP provider; accept a **breaking change** for direct consumers of provider binaries (hub/kai/kantra and related tooling are the main integrators—users who need the old layout can pin an older image); and name the Go backend explicitly (e.g. **golang** / `go-external-provider`) so nothing called “generic” implies support for arbitrary language servers.

The YAML `title` is lowercased with spaces replaced by `-`.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] User-facing documentation is created

## Summary

Today, analyzer-lsp uses a single binary, `generic-external-provider`, to serve multiple LSP-backed languages (Go/gopls, Python/pylsp, Node.js, and a YAML LSP path that does not match how YAML is usually configured in defaults, which already favor `yq-external-provider`). The same binary is started multiple times with different `--name` flags; internally, a generic provider type dispatches to language-specific "service client builders" via a map and a runtime switch on `lspServerName`. Experience has shown that a single generic client cannot reliably interface with every language server because initialization and behavior differ too much (e.g. custom LSP handlers for YAML, custom reference-evaluation for Node.js, different capability sets). The name “generic” is misleading: the implementation cannot treat arbitrary language servers interchangeably; a genuinely generic LSP client, if ever needed, would be a **different** design. This enhancement proposes **removing** that binary and dispatcher entirely and replacing them with one provider per language—each with its own binary and **dedicated container image**—following the pattern already used by `java-external-provider`. YAML stays on the **yq** provider; the generic provider’s `yaml_language_server` support is deprecated/removed as part of the split. Language providers keep using the shared **`lsp/base_service_client`** stack, but that library is also a **rewrite/refactor target**: structure and clarity should improve so the shared LSP layer is easier to follow and change than today. Only the multi-language dispatcher and the convention of "one binary, many names" go away. Integrators (e.g. hub, kai, kantra) update configs to point at the new per-language binaries and images; this is an acceptable **breaking change** for those references.

## Motivation

### Goals

- **Reliability:** Each language provider owns its LSP initialization and capabilities so that language-specific quirks (e.g. YAML `workspace/configuration` handling, Node.js batching) are implemented in one place without conditional logic in a shared dispatcher.
- **Clarity:** One binary and one container image per language so that configuration, debugging, and ownership are straightforward.
- **Consistency:** Align Go, Python, and Node.js providers with the existing pattern used by the Java provider (dedicated binary and package).
- **Maintainability:** Remove the generic provider's runtime switch on `lspServerName` and the `SupportedLanguages` map so that adding or changing a language does not touch a central dispatcher.
- **Shared LSP library:** Refactor or rewrite `lsp/base_service_client` so the code is easier to understand, navigate, and extend (clearer module boundaries, naming, and flow), without changing the outward provider gRPC contract or the `BaseClient` / `ServiceClient` interfaces that the analyzer uses.
- **E2E ownership:** End-to-end (demo) tests for each language provider live **with that provider**—the same model as `java-external-provider/e2e-tests/` and `yq-external-provider/e2e-tests/`—not bundled only under `generic-external-provider`. CI should invoke those paths per provider (e.g. `.github/workflows/demo-testing.yml` matrix entries), including if a provider later moves to its **own Git repository**.

Success will be evident when: (1) Go, Python, and Node.js are each served by a dedicated provider binary/package and image; (2) YAML analysis continues to be configured via the **yq** provider, with generic-provider YAML LSP code removed or unused; (3) no single process dispatches to multiple LSP types via `lspServerName`; (4) provider settings and docs reference distinct binaries/images per language; (5) `lsp/base_service_client` has been materially revised and reviewers can agree it is simpler to work in than the pre-refactor layout; (6) Go, Python, and Node.js **e2e test assets** (demo output, provider settings, rules) and workflows reference only their respective provider trees—nothing language-specific remains under `generic-external-provider/e2e-tests/`; (7) existing tests and demos pass with the new layout.

### Non-Goals

- Changing the analyzer-lsp provider gRPC API or the `BaseClient`/`ServiceClient` interfaces.
- Dropping the shared LSP layer entirely: `lsp/base_service_client` (or its successor in the same role) remains the foundation for LSP-based external providers; the goal is to **improve** it, not replace it with unrelated ad-hoc code per language.
- Changing how the built-in provider or non-generic providers (e.g. Java, yq) are implemented or configured.
- Supporting multiple LSP types in a single process as a feature; the refactor explicitly moves away from that model.

## Proposal

### User Stories

#### Story 1: Operator configures Go analysis

As an operator, I want to configure the analyzer to use a Go provider by pointing to a dedicated Go binary (e.g. `go-external-provider` / golang provider), so that I do not rely on `lspServerName` in providerSpecificConfig and the Go backend is not confused with a multi-language “generic” server.

#### Story 2: Operator configures Python and Node.js analysis

As an operator, I want to configure Python and Node.js providers using distinct binaries (e.g. `python-external-provider`, `nodejs-external-provider`) and provider entries in my settings file, so that each language has its own process and I can scale or debug them independently.

#### Story 3: Developer adds or changes a language provider

As a developer, I want to add or modify support for a single language (e.g. Python) without editing a central "generic" provider or a shared `SupportedLanguages` map, so that changes are localized to that language’s provider package and binary.

#### Story 4: Developer runs e2e for one language

As a developer, I want Go, Python, and Node.js e2e/demo tests to live under **that provider’s** directory (or repo), consistent with Java and yq, so that I can change one language’s provider or image and run its e2e without pulling in unrelated languages.

#### Story 5: Developer works on shared LSP client code

As a developer, I want `lsp/base_service_client` to be organized and documented so that behavior, extension points, and call flow are obvious, so that fixes and features do not require reverse-engineering a large, opaque type hierarchy.

### Implementation Details/Notes/Constraints

- **Current architecture (brief):** The analyzer loads provider configs (e.g. `provider_container_settings.json`); for each config with a `binaryPath`, the gRPC client layer runs that binary with `--port` and `--name` (where `--name` is taken from `initConfig[0].providerSpecificConfig.lspServerName`, default `"generic"`). The generic-external-provider binary constructs a `genericProvider` keyed by `--name`, which looks up a builder in `SupportedLanguages` and later may switch builders in `Init()` if config sends a different `lspServerName`. Each builder builds LSP `InitializeParams` and handlers in language-specific ways and returns a `ServiceClient` that embeds `LSPServiceClientBase` and an evaluator.

- **Target architecture:** Each of Go, Python, and Node.js gets its own provider package, binary, and **container image**. Each binary implements `BaseClient` (e.g. `Capabilities()`, `Init()`) and constructs exactly one service client type (no map, no switch on `lspServerName`). The analyzer continues to spawn one process per provider config; configs will reference different `binaryPath` values per language. **`lsp/base_service_client`:** refactored or rewritten so shared LSP client behavior lives in a clearer structure; each language provider imports and specializes that layer (and shared capability helpers where applicable) instead of duplicating it. **YAML:** no new LSP-based YAML provider is introduced from the generic split; operators use **yq-external-provider** (existing), and `yaml_language_server` code under the generic provider is deprecated/removed.

- **Reference implementation:** `java-external-provider` is the pattern: own main, own package, no dependency on the generic provider or `SupportedLanguages`.

- **Key files to change or introduce:**
  - **Analyzer core:** `provider/grpc/provider.go` — `start()` today passes `--name` derived from `lspServerName`; for language-specific binaries this can be simplified once configs no longer depend on the generic multi-language binary. No change to `BaseClient`/`ServiceClient` interfaces.
  - **Shared LSP layer:** `lsp/base_service_client` (and closely related packages under `lsp/`) — refactor for clarity and maintainability; preserve external contracts used by the analyzer and by provider binaries.
  - **Generic provider (remove):** `external-providers/generic-external-provider/main.go`, `pkg/generic_external_provider/provider.go`, `pkg/server_configurations/constants.go`; move `server_configurations/generic`, `pylsp`, and `nodejs` into new provider modules; drop or retire `yaml_language_server` in favor of yq.
  - **New providers:** New directories, e.g. `go-external-provider`, `python-external-provider`, `nodejs-external-provider`, each with its own `main.go`, provider type, and one server configuration (no dispatcher).
  - **Config and build:** Update `provider_container_settings.json`, Makefile, root Dockerfile, replace generic-provider image build with **per-language** Dockerfiles/images, `.github/workflows/image-build.yaml`, `.github/workflows/demo-testing.yml`, and testdata/docs as needed. **E2E / demos:** Today, golang-, python-, and nodejs-e2e assets live under `external-providers/generic-external-provider/e2e-tests/` while Java and yq already use `external-providers/<provider>/e2e-tests/`. **Move** each language’s e2e tree beside its new provider (e.g. `go-external-provider/e2e-tests/`, mirroring `java-external-provider/e2e-tests/`). Update `.github/workflows/demo-testing.yml` (and any related konveyor CI) so each matrix entry points at the **owning** provider path; align with `external-providers/TESTING.md`. If a provider is published from a **separate Git repository** in the future, its e2e should travel with that repo.
  - **Tests:** Move or duplicate `server_configurations/*/service_client_test.go` into the new provider packages; adjust `provider/provider_test.go` if it asserts on generic-provider-specific behavior.

- **Constraint:** The analyzer calls `Capabilities()` before `Init()`, so each process must advertise the correct capabilities at startup. With one binary per language, this is natural; no runtime switch is required.

- **Follow-up (post–per-language split):** Compare LSP-backed provider architecture: **continue building on a refactored `lsp/base_service_client`** vs **leaning on `provider.NewServer` and `ServiceClient` with a thinner LSP layer** (more explicit per-language LSP code, less reliance on a monolithic shared base). Document tradeoffs—duplication vs clarity, blast radius of protocol or analyzer changes, and the fact that TLS/JWT for the **gRPC** side already live in the `provider` server implementation—and record a maintainer decision or short ADR.

### Security, Risks, and Mitigations

- **Risk: More binaries/images.** More artifacts to build, sign, and distribute. Mitigation: Document the new layout clearly; **separate images per language** are the chosen model (smaller deploys, easier debugging, fewer unrelated providers in one image). A single image bundling multiple binaries remains a theoretical alternative but is not the default plan.
- **Risk: Regressions in behavior.** Each language’s logic is moved into a new package, and the shared LSP layer changes shape. Mitigation: Preserve existing tests (move or re-run), extend unit coverage for `lsp/base_service_client` where gaps appear after the rewrite, run **per-provider** e2e/demos after assets are relocated (same scenarios as today’s golang/python/nodejs flows, new paths), and add any missing integration tests.
- **Security:** No new network exposure or auth model; providers remain out-of-process gRPC services. Binary execution is unchanged (analyzer still spawns one process per provider config). Review by maintainers familiar with the provider and analyzer-lsp security model is sufficient.

## Design Details

### Test Plan

- **Unit tests:** Each new provider package must have unit tests for its provider type (Capabilities, Init) and for its service client behavior where non-trivial (e.g. Node.js-specific evaluation). The refactor of `lsp/base_service_client` should keep or improve test coverage for shared behavior (handlers, initialization helpers, and any code moved out of provider-specific packages). Existing tests under `external-providers/generic-external-provider/pkg/server_configurations/*/service_client_test.go` will be moved or reimplemented in the new packages so that behavior is preserved.
- **Integration / e2e:** **Per-provider layout (maintainer expectation):** e2e/demo assets must live with each language provider—the pattern already used for `java-external-provider` and `yq-external-provider`—not under a shared generic-provider tree. **Migrate** today’s `generic-external-provider/e2e-tests/{golang,python,nodejs}-e2e/` content into `go-external-provider`, `python-external-provider`, and `nodejs-external-provider` (or final directory names), each with its own `e2e-tests/` (see `external-providers/TESTING.md`). Update `.github/workflows/demo-testing.yml` so jobs reference those paths and the new images. This matches the direction that e2e is **split across providers** (and can follow a provider into its **own repository** if the project splits repos later).
- **Isolation:** Each language provider can be tested in isolation by running its binary with the same gRPC contract; the analyzer’s existing provider client logic does not need to change beyond how it starts the binary (and possibly how `--name` is passed).
- Tricky areas: (1) Ensuring capability lists and Init parameters match what the analyzer expects for each provider name. (2) YAML: validate **yq** flows; remove or replace coverage that only applied to `yaml_language_server` under the generic provider.

### Follow-up design work

- **Provider gRPC vs LSP layering:** After each language has its own external provider binary, run the comparison described under **Implementation Details** (*Follow-up (post–per-language split)*): refactored `lsp/base_service_client` vs a design that emphasizes `provider.NewServer` / `ServiceClient` and a slimmer LSP stack. Capture the outcome in the enhancement or a linked ADR so future providers follow one clear pattern.

### Upgrade / Downgrade Strategy

- **Upgrade:** Consumers currently using `generic-external-provider` for Go, Python, or Node.js must update provider settings to the new per-language binaries and images (including operator/extension references where applicable). Documentation and default `provider_container_settings.json` (or equivalent) will reflect the new layout. There is **no** commitment to keep a long-lived shim named `generic-external-provider` for gopls-only use; the Go path should be explicitly named (e.g. golang / go provider).
- **Downgrade:** Reverting to a previous release means restoring the old generic binary and configs that used `lspServerName` to select the language. No schema change to the analyzer’s provider config format is required; only the number and identity of binaries and images change.

## Implementation History

| Date       | Status / Milestone |
|-----------|---------------------|
| 2026-03-13 | Enhancement proposed (provisional); refactoring analysis document added. |
| 2026-03-25 | Updated enhancement proposal with feedback from reviewers. |

## Drawbacks

- **Operational overhead:** Operators must manage and optionally image-scan more than one provider binary/image if we ship separate images per language.
- **Duplication of structure:** Each language provider will have its own main and provider type, which repeats some boilerplate (flag parsing, server startup). This is intentional for isolation but increases code surface.
- **Scope of the shared-layer rewrite:** A large refactor of `lsp/base_service_client` can slip in scope or schedule if not bounded; tie milestones to the per-provider split and keep the `BaseClient`/`ServiceClient` boundary stable.
- **Breaking change for current generic users:** Anyone relying on a single generic binary with multiple `lspServerName` values must switch to multiple provider entries, binaries, and images; pinning an older release is the compatibility story for stragglers.

## Alternatives

1. **Keep the generic provider and only fix initialization per language.** We could leave the single binary and dispatcher in place and try to make initialization more pluggable (e.g. more builder options). This was considered insufficient because the issue states that a generic client cannot reliably interface with every language server; the problem is structural, not just a few missing options.

2. **Single image with multiple binaries and a router entrypoint.** We could keep one container image that contains go-external-provider, python-external-provider, nodejs-external-provider and an entrypoint that invokes the right binary based on env or args. This preserves “one image” for deployment while still allowing separate codebases and no in-process dispatcher. **Rejected** as the primary approach in favor of **one image per language** (maintainer preference: image size, isolation, debuggability).

3. **Leave YAML in the generic provider only.** A shrunken generic supporting only gopls and `yaml_language_server` would retain a small dispatcher. **Rejected** in favor of **yq** for YAML and full removal of the generic provider.

## Infrastructure Needed

- CI jobs (or matrix entries) to build and test each new provider binary and **each per-language image**.
- **Demo / e2e CI:** Workflow matrix (e.g. `demo-testing.yml`) must treat each provider as a **separate** e2e target with paths under `external-providers/<provider>/e2e-tests/`, not a single generic-provider folder for multiple languages.
- **Konveyor operator:** The operator’s **Python** and **Node.js** extensions (and any CRs or defaults that pin provider images) must be updated to reference the **new per-language container images** once those images ship—see [konveyor/enhancements#262](https://github.com/konveyor/enhancements/pull/262) (comment by @jortel). Coordinate release timing between analyzer-lsp image publishes and operator releases.
- No new repositories or subprojects are required for the initial split; new providers can live under `external-providers/` in the analyzer-lsp repo, consistent with `java-external-provider` and `yq-external-provider`. E2e layout should still assume providers **could** move to their own repos later without re-bundling tests.
