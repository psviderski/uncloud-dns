build:
	CGO_ENABLED=0 go build -o bin/uncloud-dns -ldflags "-s -w" .

image:
	docker build .

setup-ci-env:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.52.2

validate-ci:
	go generate
	go mod tidy
	if [ -n "$$(git status --porcelain --untracked-files=no)" ]; then \
		git status --porcelain --untracked-files=no; \
		echo "Encountered dirty repo!"; \
		exit 1 \
	;fi

validate:
	golangci-lint --timeout 5m run

test:
	go test ./...

dev:
	./scripts/dev.sh
