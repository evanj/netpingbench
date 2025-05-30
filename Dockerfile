# This is a hack where we build third-party Go modules the root package depends on
# TODO: Figure out a less manual way to cache dependencies?
FROM golang:1.24-bookworm AS cached_modules
WORKDIR /go/src
COPY go.mod go.sum /go/src/
RUN go build -v \
   google.golang.org/grpc \
   google.golang.org/protobuf/proto

FROM cached_modules AS builder
COPY . /go/src/
WORKDIR /go/src
RUN go build -v -o /go/bin/gonetpingbench ./gonetpingbench

FROM rust:1.86-bookworm AS cached_rust_dependencies
WORKDIR /rustbuild
COPY build.rs Cargo.toml Cargo.lock rust-toolchain.toml /rustbuild/
COPY proto/*.proto /rustbuild/proto/
RUN \
   mkdir -p src && \
   touch src/lib.rs && \
   cargo build --locked --release

FROM cached_rust_dependencies AS rust_builder
COPY . .
RUN cargo build --locked --release

# Use a non-root user: slightly more secure (defense in depth)
# must use cc for rust binary: needs libgcc_s
FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=builder /go/bin/* /
COPY --from=rust_builder /rustbuild/target/release/netpingbench /
USER nonroot
WORKDIR / 
ENTRYPOINT ["/gonetpingbench"]
