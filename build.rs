fn main() -> Result<(), Box<dyn std::error::Error>> {
    dlprotoc::download_protoc()?;
    tonic_build::compile_protos("proto/echo.proto")
        .unwrap_or_else(|e| panic!("Failed to compile protos {e:?}"));
    Ok(())
}
