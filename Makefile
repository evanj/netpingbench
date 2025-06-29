BUILD_DIR:=build
PROTOC:=$(BUILD_DIR)/bin/protoc
PROTOC_GEN_GO:=$(BUILD_DIR)/protoc-gen-go
PROTOC_GEN_GO_GRPC:=$(BUILD_DIR)/protoc-gen-go-grpc

all: $(BUILD_DIR)/.gocheck_stamp $(BUILD_DIR)/.rustcheck_stamp

# include all go files in current dir and below
# Make's wildcard function can't match all
GOFILES:=$(shell find . -name '*.go')

$(BUILD_DIR)/.gocheck_stamp: $(GOFILES) echopb/echo.pb.go | $(BUILD_DIR)
	go test -race -shuffle=on -count=2 ./...
	go vet ./...
	go fmt ./...
	staticcheck --checks=all $(shell go list ./... | grep -v 'netpingbench/echopb')
	go mod tidy
	touch $@

# include all Rust files in current dir and below
# Make's wildcard function can't match all
RUSTFILES:=$(shell find . -name '*.rs')

$(BUILD_DIR)/.rustcheck_stamp: $(RUSTFILES) Cargo.toml Cargo.lock proto/* | $(BUILD_DIR)
	cargo test --all-targets
	# disallow warnings so they fail CI
	cargo clippy --all-targets -- -D warnings
	cargo fmt
	touch $@

echopb/echo.pb.go: proto/echo.proto $(BUILD_DIR)/.proto_format_stamp $(PROTOC) $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC)
	# TODO: Remove default_api_level when we are on the new edition
	$(PROTOC) --plugin=$(PROTOC_GEN_GO) --plugin=$(PROTOC_GEN_GO_GRPC) \
		--go_opt=default_api_level=API_OPAQUE \
		--go_out=. --go_opt=Mproto/echo.proto=./echopb \
		--go-grpc_out=. --go-grpc_opt=Mproto/echo.proto=./echopb \
		$<

# Ensures proto files are formatted: must be added as a dependency before the generated files
$(BUILD_DIR)/.proto_format_stamp: $(wildcard proto/*.proto) | $(BUILD_DIR)
	clang-format --style=Google -i $<
	touch $@

# download protoc to a temporary tools directory
$(PROTOC): $(BUILD_DIR)/getprotoc | $(BUILD_DIR)
	$(BUILD_DIR)/getprotoc --outputDir=$(BUILD_DIR)

$(BUILD_DIR)/getprotoc: | $(BUILD_DIR)
	GOBIN=$(realpath $(BUILD_DIR)) go install github.com/evanj/hacks/getprotoc@latest

# go install uses the version of protoc-gen-go specified by go.mod ... I think
$(PROTOC_GEN_GO): go.mod | $(BUILD_DIR)
	GOBIN=$(realpath $(BUILD_DIR)) go install google.golang.org/protobuf/cmd/protoc-gen-go

# manually specified version since we don't import this from code anywhere
# TODO: Import this from some tool so it gets updated with go get?
$(PROTOC_GEN_GO_GRPC): go.mod | $(BUILD_DIR)
	GOBIN=$(realpath $(BUILD_DIR)) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

$(BUILD_DIR):
	mkdir -p $@

clean:
	$(RM) -r $(BUILD_DIR)
