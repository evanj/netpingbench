fn main() {
    // generate Rust stubs from the protobuf source
    tonic_build::compile_protos("proto/echo.proto")
        .unwrap_or_else(|e| panic!("Failed to compile protos {:?}", e));
}
