# OpenAI Images Passthrough Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pass Codex image generation and edit requests through ccNexus to OpenAI-compatible image endpoints and build ccNexus 5.3.1 for macOS arm64.

**Architecture:** Introduce one client-format discriminator for the two OpenAI Images paths. Reuse the existing byte-for-byte OpenAI2 passthrough transformer and preserve image paths before the text-format route switch.

**Tech Stack:** Go 1.24, standard `net/http`, existing ccNexus transformers, Vue/Vite, Wails v2.11.0.

---

### Task 1: Add the failing image-route regression test

**Files:**
- Modify: `internal/proxy/request_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestOpenAIImageRequestsPreservePathAndPayload(t *testing.T) {
	endpoint := config.Endpoint{
		Name:        "Images",
		APIUrl:      "https://api.example.com",
		APIKey:      "secret",
		AuthMode:    config.AuthModeAPIKey,
		Transformer: "openai2",
	}
	payload := []byte(`{"model":"gpt-image-2","prompt":"a kitten","quality":"auto","size":"auto"}`)

	for _, requestPath := range []string{"/v1/images/generations", "/v1/images/edits"} {
		t.Run(requestPath, func(t *testing.T) {
			clientFormat := detectClientFormat(requestPath)
			if clientFormat != ClientFormatOpenAIImages {
				t.Fatalf("client format = %q, want %q", clientFormat, ClientFormatOpenAIImages)
			}

			trans, err := prepareTransformerForClient(clientFormat, endpoint, "gpt-image-2")
			if err != nil {
				t.Fatalf("prepare transformer: %v", err)
			}
			transformed, err := trans.TransformRequest(payload)
			if err != nil {
				t.Fatalf("transform request: %v", err)
			}
			incoming := httptest.NewRequest(http.MethodPost, requestPath, bytes.NewReader(payload))
			outgoing, err := buildProxyRequest(incoming, endpoint, endpoint.APIKey, transformed, trans.Name(), "gpt-image-2", nil)
			if err != nil {
				t.Fatalf("build proxy request: %v", err)
			}
			if outgoing.URL.Path != requestPath {
				t.Fatalf("upstream path = %q, want %q", outgoing.URL.Path, requestPath)
			}
			body, err := io.ReadAll(outgoing.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if !bytes.Equal(body, payload) {
				t.Fatalf("upstream payload = %s, want %s", body, payload)
			}
		})
	}
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/proxy -run TestOpenAIImageRequestsPreservePathAndPayload -count=1`

Expected: FAIL because `ClientFormatOpenAIImages` and image routing do not exist.

### Task 2: Implement the minimal passthrough

**Files:**
- Modify: `internal/proxy/proxy.go`
- Modify: `internal/proxy/request.go`

- [ ] **Step 1: Recognize image client paths**

Add the client format and shared path predicate in `internal/proxy/proxy.go`:

```go
ClientFormatOpenAIImages ClientFormat = "openai_images"

func isOpenAIImagesPath(requestPath string) bool {
	switch strings.TrimSuffix(requestPath, "/") {
	case "/v1/images/generations", "/images/generations", "/v1/images/edits", "/images/edits":
		return true
	default:
		return false
	}
}
```

Return `ClientFormatOpenAIImages` from `detectClientFormat` before the default Claude branch.

- [ ] **Step 2: Reuse passthrough only for OpenAI-compatible endpoints**

Add this branch to `prepareTransformerForClient` in `internal/proxy/request.go`:

```go
case ClientFormatOpenAIImages:
	if endpointTransformer == "openai" || endpointTransformer == "openai2" {
		return responses.NewOpenAI2Transformer(effectiveModel), nil
	}
	return nil, fmt.Errorf("unsupported endpoint transformer for OpenAI Images: %s", endpointTransformer)
```

Preserve the original image path at the start of `getTargetPath`:

```go
if isOpenAIImagesPath(originalPath) {
	return originalPath
}
```

- [ ] **Step 3: Format and verify GREEN**

Run: `gofmt -w internal/proxy/proxy.go internal/proxy/request.go internal/proxy/request_test.go`

Run: `go test ./internal/proxy -run TestOpenAIImageRequestsPreservePathAndPayload -count=1`

Expected: PASS.

- [ ] **Step 4: Run all proxy tests**

Run: `go test ./internal/proxy -count=1`

Expected: PASS.

### Task 3: Set the real application version

**Files:**
- Modify: `cmd/desktop/wails.json`
- Modify: `cmd/desktop/build/darwin/Info.plist`

- [ ] **Step 1: Change both version sources to 5.3.1**

Set `info.productVersion`, `CFBundleShortVersionString`, and `CFBundleVersion` to `5.3.1`.

- [ ] **Step 2: Verify version metadata sources**

Run: `jq -e '.info.productVersion == "5.3.1"' cmd/desktop/wails.json`

Run: `test "$(plutil -extract CFBundleShortVersionString raw cmd/desktop/build/darwin/Info.plist)" = "5.3.1" && test "$(plutil -extract CFBundleVersion raw cmd/desktop/build/darwin/Info.plist)" = "5.3.1"`

Expected: both commands exit 0.

### Task 4: Verify and build the deliverable

**Files:**
- Generated: `cmd/desktop/build/bin/ccNexus.app`
- Generated: `cmd/desktop/build/bin/ccNexus-v5.3.1-darwin-arm64.zip`

- [ ] **Step 1: Run repository checks**

Run: `git diff --check`

Run: `go test ./... -count=1`

Expected: PASS with no failures.

- [ ] **Step 2: Build the frontend**

Run: `npm run build` from `cmd/desktop/frontend`.

Expected: Vite production build exits 0.

- [ ] **Step 3: Build macOS arm64**

Run: `"$(go env GOPATH)/bin/wails" build -platform darwin/arm64` from `cmd/desktop`.

Expected: Wails exits 0 and creates `build/bin/ccNexus.app`.

- [ ] **Step 4: Inspect and archive the bundle**

Run: `file cmd/desktop/build/bin/ccNexus.app/Contents/MacOS/ccNexus`

Run: `plutil -p cmd/desktop/build/bin/ccNexus.app/Contents/Info.plist`

Run: `codesign --verify --deep --strict --verbose=2 cmd/desktop/build/bin/ccNexus.app`

Run: `ditto -c -k --sequesterRsrc --keepParent cmd/desktop/build/bin/ccNexus.app cmd/desktop/build/bin/ccNexus-v5.3.1-darwin-arm64.zip`

Run: `shasum -a 256 cmd/desktop/build/bin/ccNexus-v5.3.1-darwin-arm64.zip`

Expected: arm64 bundle, version 5.3.1, valid Wails signing state, zip created with a recorded SHA-256.

- [ ] **Step 5: Commit source changes only**

```bash
git add internal/proxy/proxy.go internal/proxy/request.go internal/proxy/request_test.go cmd/desktop/wails.json cmd/desktop/build/darwin/Info.plist docs/superpowers/plans/2026-07-26-openai-images-passthrough.md
git commit -m "fix: proxy OpenAI image requests"
```
