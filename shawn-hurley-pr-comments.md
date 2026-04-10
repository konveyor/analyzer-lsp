# Shawn Hurley's Comments on PR #1142

## Comment 1: golang-dependency-provider Architecture

**Location:** `.github/workflows/image-build.yaml:56`

**Shawn's Comment:**
> Could this just be built in the go provider and not be its own command?

**Context:**
The golang-dependency-provider is currently built as a separate container image (lines 44-47 in image-build.yaml) and then passed as a build argument to the go-external-provider image (line 56).

**Analysis:**
This is an architectural question about whether the golang-dependency-provider should remain a standalone component or be merged into the go-external-provider. The current separation allows:
- Independent versioning and deployment of the dependency provider
- Reuse of the dependency provider by other components if needed
- Clearer separation of concerns

However, if the golang-dependency-provider is only used by go-external-provider and nowhere else, combining them could simplify the build process and reduce the number of images to maintain.

**User's Additional Notes:**
Let's skip this for now - we'll do it later.



---

## Comment 2: GetGoServiceClientCapabilities Usage

**Location:** `external-providers/go-external-provider/pkg/go_external_provider/service_client.go:90`

**Shawn's Comment:**
> This wouldn't be used right because in the provider, you are just handing them back?

**Context:**
This refers to the `GetGoServiceClientCapabilities` function which builds and returns a list of LSP service client capabilities including "referenced" and "dependency".

**Analysis:**
Shawn appears to be questioning whether this function's return value is actually being utilized properly, or if the capabilities are just being passed through without real functionality. Looking at the code, the function creates capabilities with associated evaluation functions (like `base.EvaluateReferenced` and `base.EvaluateNoOp`), which are then used by the LSPServiceClientEvaluator. 

The concern might be that some capabilities (particularly the "dependency" capability using `EvaluateNoOp`) are being registered but not actually doing meaningful work.

**User's Additional Notes:**
I still don't quite understand Shawn's comment here... Let's not do anything for the moment.



---
