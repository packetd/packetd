# REQUIRE TOOLS
# https://github.com/mvdan/gofumpt => $go install mvdan.cc/gofumpt@latest
# https://github.com/incu6us/goimports-reviser => $go install github.com/incu6us/goimports-reviser/v3@v3.1.1
# https://github.com/golangci/golangci-lint => $go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.50.1
# https://github.com/google/addlicense => $go install github.com/google/addlicense@latest

.PHONY: lint
lint: license
	gofumpt -w .
	goimports-reviser -project-name "github.com/packetd/packetd" ./...

.PHONY: test
test:
	gotest -v ./...

.PHONY: license
license:
	find ./ -type f \( -iname \*.go -o -iname \*.sh \) | xargs addlicense -v -f LICENSE
