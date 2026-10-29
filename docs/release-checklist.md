# Pre-submission checklist

Grafana's plugin review scans the backend with `govulncheck` (on the **built
binary**) and `gosec` (on the source) and reports findings as warnings. Vulnerability databases update daily, so a
clean CI run last week does not guarantee a clean review today.

Before tagging a release or resubmitting:

1. Run the scheduled **Vulnerability scan** workflow manually
   (Actions → Vulnerability scan → Run workflow), or locally with the Go
   version from `go.mod`:
   ```bash
   go run github.com/securego/gosec/v2/cmd/gosec@v2.30.0 -quiet ./...
   go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
   CGO_ENABLED=0 go build -o /tmp/plugin-backend ./pkg
   go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -mode=binary /tmp/plugin-backend
   ```
   A local Go newer than `go.mod` hides standard-library findings, so use
   `GOTOOLCHAIN=go$(awk '/^go /{print $2}' go.mod)` when scanning.
2. Run the `validate-plugin` skill (`@grafana/plugin-validator` on the built zip).
3. Keep the Go version in `go.mod` and `.github/workflows/release.yml` in sync;
   CI fails if they differ.
