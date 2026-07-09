#!/usr/bin/env bash
# Pre-Release Validation Script for termizard.
# Mirrors all CI checks (build, vet, tests, lint, formatting).
#
# Usage:
#   bash scripts/pre-release-check.sh          # Full check
#   bash scripts/pre-release-check.sh --quick  # Skip slow steps (lint, coverage)

set -e

GO_PACKAGES="./cmd/... ./internal/..."

# Colors
RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[1;33m' BLUE='\033[0;34m' NC='\033[0m'
log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $1"; }

QUICK=0
[[ "${1:-}" == "--quick" ]] && QUICK=1

echo ""
echo "================================================"
echo "  termizard — Pre-Release Check"
echo "================================================"
echo ""

ERRORS=0
WARNINGS=0

# 1. Go version
log_info "Checking Go version..."
GO_VERSION=$(go version | awk '{print $3}')
REQUIRED="go1.26"
if [[ "$GO_VERSION" < "$REQUIRED" ]]; then
  log_error "Go $REQUIRED+ required, found $GO_VERSION"
  ERRORS=$((ERRORS+1))
else
  log_success "Go version: $GO_VERSION"
fi
echo ""

# 2. Git status
log_info "Checking git status..."
if git diff-index --quiet HEAD --; then
  log_success "Working directory is clean"
else
  log_warning "Uncommitted changes detected"
  git status --short
  WARNINGS=$((WARNINGS+1))
fi
echo ""

# 3. Formatting
log_info "Checking code formatting (gofmt)..."
UNFORMATTED=$(gofmt -l $(go list -f '{{.Dir}}' $GO_PACKAGES | sort -u) 2>/dev/null || true)
if [ -n "$UNFORMATTED" ]; then
  log_error "Files need formatting:"
  echo "$UNFORMATTED"
  ERRORS=$((ERRORS+1))
else
  log_success "All files properly formatted"
fi
echo ""

# 4. go vet
log_info "Running go vet..."
if CGO_ENABLED=0 go vet $GO_PACKAGES 2>&1; then
  log_success "go vet passed"
else
  log_error "go vet failed"
  ERRORS=$((ERRORS+1))
fi
echo ""

# 5. Build
log_info "Building application (CGO_ENABLED=0)..."
BUILD_TMP=$(mktemp -d)
EXT=""
[[ "$(uname -s)" == MINGW* || "$(uname -s)" == CYGWIN* ]] && EXT=".exe"
if CGO_ENABLED=0 go build -tags production -o "$BUILD_TMP/termizard${EXT}" ./cmd/termizard 2>&1; then
  log_success "Build successful"
else
  log_error "Build failed"
  ERRORS=$((ERRORS+1))
fi
rm -rf "$BUILD_TMP"
echo ""

# 6. go.mod
log_info "Validating go.mod..."
go mod verify && log_success "go.mod verified" || { log_error "go.mod verify failed"; ERRORS=$((ERRORS+1)); }
go mod tidy
if git diff --quiet go.mod go.sum 2>/dev/null; then
  log_success "go.mod is tidy"
else
  log_warning "go.mod needs tidying"
  git diff go.mod go.sum 2>/dev/null || true
  WARNINGS=$((WARNINGS+1))
fi
echo ""

# 7. Tests with race detector
log_info "Running tests..."
if CGO_ENABLED=0 go test -race -count=1 $GO_PACKAGES 2>&1; then
  log_success "All tests passed"
else
  log_error "Tests failed"
  ERRORS=$((ERRORS+1))
fi
echo ""

# 8. Coverage
if [ "$QUICK" -eq 0 ]; then
  log_info "Checking test coverage..."
  COVERAGE=$(CGO_ENABLED=0 go test -count=1 -cover $GO_PACKAGES 2>&1 \
    | grep "coverage:" | tail -1 | awk -F'coverage: ' '{print $2}' | awk '{print $1}' | sed 's/%//')
  if [ -n "$COVERAGE" ]; then
    echo "  coverage: ${COVERAGE}%"
    log_success "Coverage: ${COVERAGE}%"
  else
    log_warning "Could not determine coverage"
    WARNINGS=$((WARNINGS+1))
  fi
  echo ""
fi

# 9. Lint
if [ "$QUICK" -eq 0 ]; then
  log_info "Running golangci-lint..."
  if command -v golangci-lint &>/dev/null; then
    if CGO_ENABLED=0 golangci-lint run --timeout=5m $GO_PACKAGES 2>&1; then
      log_success "golangci-lint passed"
    else
      log_error "golangci-lint found issues"
      ERRORS=$((ERRORS+1))
    fi
  else
    log_warning "golangci-lint not installed (optional but recommended)"
    WARNINGS=$((WARNINGS+1))
  fi
  echo ""
fi

# 10. Documentation
log_info "Checking critical documentation..."
for doc in README.md LICENSE; do
  if [ ! -f "$doc" ]; then
    log_error "Missing: $doc"
    ERRORS=$((ERRORS+1))
  fi
done
[ $ERRORS -eq 0 ] && log_success "Documentation present"
echo ""

# Summary
echo "========================================"
echo "  Summary"
echo "========================================"
echo ""

if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
  log_success "All checks passed! Ready for release."
  echo ""
  echo "  1. git tag -a vX.Y.Z -m \"Release vX.Y.Z\""
  echo "  2. git push origin vX.Y.Z"
  echo ""
  exit 0
elif [ $ERRORS -eq 0 ]; then
  log_warning "Completed with $WARNINGS warning(s). Review before proceeding."
  exit 0
else
  log_error "Failed with $ERRORS error(s) and $WARNINGS warning(s). Fix before release."
  exit 1
fi
