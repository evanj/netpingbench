pub mod echopb {
    #![allow(clippy::pedantic, clippy::nursery)]
    tonic::include_proto!("echopb");
}

use std::io::BufRead;
use std::io::BufReader;
use std::io::Write;
use std::net::TcpListener;
use std::net::TcpStream;
use std::thread;
use std::time::Duration;

use clap::Parser;
use echopb::echo_server::Echo;
use echopb::echo_server::EchoServer;
use echopb::EchoRequest;
use echopb::EchoResponse;
use tokio::io::AsyncBufReadExt;
use tokio::io::AsyncWriteExt;
use tonic::transport::Server;
use tonic::Request;
use tonic::Response;
use tonic::Status;

// Same as the Go version
const TCP_INITIAL_BUFFER_BYTES: usize = 4096;

#[derive(Debug)]
struct EchoService {
    burn_cpu_amount: Duration,
    burn_spawn_blocking: bool,
}

impl EchoService {
    const fn new(burn_cpu_amount: Duration, burn_spawn_blocking: bool) -> Self {
        Self {
            burn_cpu_amount,
            burn_spawn_blocking,
        }
    }
}

#[tonic::async_trait]
impl Echo for EchoService {
    async fn echo(&self, request: Request<EchoRequest>) -> Result<Response<EchoResponse>, Status> {
        tokio_burn_cpu(self.burn_cpu_amount, self.burn_spawn_blocking).await;
        let r = request.into_inner();
        Ok(tonic::Response::new(EchoResponse { output: r.input }))
    }
}

/// Uses amount CPU time synchronously. If `spawn_blocking` is true, it will run the [`burn_cpu`]
/// function on tokio's background thread pool using [`tokio::task::spawn_blocking`].
async fn tokio_burn_cpu(amount: Duration, spawn_blocking: bool) {
    if spawn_blocking {
        tokio::task::spawn_blocking(move || burn_cpu(amount))
            .await
            .unwrap();
    } else {
        burn_cpu(amount);
    }
}

#[derive(Parser)]
#[command(version, about, long_about = None)]
#[allow(clippy::struct_field_names)]
struct Args {
    /// Address to listen on for the gRPC server.
    #[arg(long, default_value = "127.0.0.1:8003")]
    tcp_thread_listen_addr: String,

    /// Address to listen on for the gRPC server.
    #[arg(long, default_value = "127.0.0.1:8004")]
    tcp_tokio_listen_addr: String,

    /// Address to listen on for the gRPC server.
    #[arg(long, default_value = "127.0.0.1:8005")]
    grpc_listen_addr: String,

    /// CPU time to burn while handling a request.
    #[arg(long, value_parser=parse_go_duration, default_value = "0")]
    burn_cpu_amount: Duration,

    /// Disable tokio's LIFO slot optimization. See:
    /// <https://docs.rs/tokio/latest/tokio/runtime/struct.Builder.html#method.disable_lifo_slot>
    #[arg(long, default_value_t = false)]
    disable_lifo_slot: bool,

    /// Burn CPU using tokio `spawn_blocking`.
    #[arg(long, default_value_t = false)]
    burn_spawn_blocking: bool,
}

/// Parses a Go duration string into a Rust value. The `go-parse-duration` crate is more complete,
/// but it uses `chrono::Duration`. This version has no dependencies.
///
/// TODO: Support all GO formats. Currently only supports us, ms and s.
///
/// # Errors
/// * [`ParseDurationError`]: If the input arg is not parsable.
pub fn parse_go_duration(arg: &str) -> Result<Duration, ParseDurationError> {
    if let Some(no_units) = arg.strip_suffix("us") {
        let micros = no_units.parse().map_err(|_| ParseDurationError::new(arg))?;
        Ok(Duration::from_micros(micros))
    } else if let Some(no_units) = arg.strip_suffix("ms") {
        let millis = no_units.parse().map_err(|_| ParseDurationError::new(arg))?;
        Ok(Duration::from_millis(millis))
    } else if let Some(no_units) = arg.strip_suffix('s') {
        let seconds = no_units.parse().map_err(|_| ParseDurationError::new(arg))?;
        Ok(Duration::from_secs(seconds))
    } else if arg == "0" {
        Ok(Duration::ZERO)
    } else {
        Err(ParseDurationError::new(arg))
    }
}

#[derive(Debug)]
pub struct ParseDurationError {
    invalid_input: String,
}

impl ParseDurationError {
    #[must_use]
    pub fn new(invalid_input: &str) -> Self {
        Self {
            invalid_input: invalid_input.to_string(),
        }
    }
}

impl std::error::Error for ParseDurationError {}

impl std::fmt::Display for ParseDurationError {
    fn fmt(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
        write!(f, "invalid Go duration string {:#?}", self.invalid_input)
    }
}

/// Set the NOFILES rlimit on max open files to the max value. Go does this by default. See
/// syscall/rlimit.go:
/// <https://github.com/golang/go/blob/master/src/syscall/rlimit.go>
fn raise_nofile_rlimit() -> Result<(), std::io::Error> {
    // TODO: use nix for friendlier interface? libc has fewer dependencies

    // set max files to no limit: Go does this by default see syscall/rlimit.go
    // TODO: use nix for friendlier interface? libc has fewer dependencies
    let mut nofile_limit = libc::rlimit {
        rlim_cur: 0,
        rlim_max: 0,
    };
    let result = unsafe { libc::getrlimit(libc::RLIMIT_NOFILE, &mut nofile_limit) };
    if result != 0 {
        return Err(std::io::Error::last_os_error());
    }

    nofile_limit.rlim_cur = nofile_limit.rlim_max;
    let result = unsafe { libc::setrlimit(libc::RLIMIT_NOFILE, &nofile_limit) };
    if result != 0 {
        Err(std::io::Error::last_os_error())
    } else {
        Ok(())
    }
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();

    if args.burn_spawn_blocking && args.burn_cpu_amount <= Duration::ZERO {
        eprintln!("burn_spawn_blocking requires burn_cpu_amount > 0");
        std::process::exit(1);
    }

    raise_nofile_rlimit()?;

    println!(
        "listening for TCP thread connections on {} ...",
        args.tcp_thread_listen_addr
    );
    let tcp_thread_listener = TcpListener::bind(args.tcp_thread_listen_addr)?;

    let thread_server_handle =
        thread::spawn(move || run_thread_server(tcp_thread_listener, args.burn_cpu_amount));

    let tokio_handle = thread::spawn(move || {
        run_tokio(
            &args.grpc_listen_addr,
            &args.tcp_tokio_listen_addr,
            args.disable_lifo_slot,
            args.burn_cpu_amount,
            args.burn_spawn_blocking,
        )
    });

    println!("waiting for server to exit (should never exit) ...");

    match thread_server_handle.join() {
        Ok(Ok(())) => {}
        Ok(Err(e)) => panic!("thread server returned error: {e:?}"),
        Err(e) => {
            panic!("thread server paniced: {e:?}")
        }
    }

    match tokio_handle.join() {
        Ok(Ok(())) => {}
        Ok(Err(e)) => panic!("tokio returned error: {e:?}"),
        Err(e) => {
            panic!("tokio paniced: {e:?}")
        }
    }

    Ok(())
}

/// Runs the Tokio event loop with TCP and gRPC echo implementations.
fn run_tokio(
    grpc_listen_addr: &str,
    tcp_tokio_listen_addr: &str,
    disable_lifo_slot: bool,
    burn_cpu_amount: Duration,
    burn_spawn_blocking: bool,
) -> Result<(), ErrorMessageOnly> {
    // Equivalent to tokio::runtime::Runtime::new()
    let mut builder = tokio::runtime::Builder::new_multi_thread();
    builder.enable_all();
    if disable_lifo_slot {
        println!("disabling tokio LIFO slot optimization ...");
        builder.disable_lifo_slot();
    }
    let runtime = builder.build()?;

    let rt_metrics = runtime.metrics();
    println!(
        "tokio runtime flavor={:?} num_workers={} std::thread::available_parallelism()={}",
        runtime.handle().runtime_flavor(),
        rt_metrics.num_workers(),
        std::thread::available_parallelism().unwrap(),
    );

    let grpc_listen_addr = grpc_listen_addr.to_string();
    let tcp_tokio_listen_addr = tcp_tokio_listen_addr.to_string();
    runtime.block_on(async move {
        tokio_main(
            grpc_listen_addr,
            tcp_tokio_listen_addr,
            burn_cpu_amount,
            burn_spawn_blocking,
        )
        .await
    })?;
    Ok(())
}

async fn tokio_main(
    grpc_listen_addr: String,
    tcp_tokio_listen_addr: String,
    burn_cpu_amount: Duration,
    burn_spawn_blocking: bool,
) -> Result<(), ErrorMessageOnly> {
    println!("listening for Tokio TCP on {tcp_tokio_listen_addr} ...");
    let tcp_tokio_parsed: std::net::SocketAddr = tcp_tokio_listen_addr.parse()?;
    let tcp_tokio_listener =
        tokio::net::TcpListener::bind((tcp_tokio_parsed.ip(), tcp_tokio_parsed.port())).await?;
    let tokio_tcp_echo_handle = tokio::spawn(tokio_echo_server(
        tcp_tokio_listener,
        burn_cpu_amount,
        burn_spawn_blocking,
    ));

    println!("listening for gRPC on {grpc_listen_addr} ...");
    let grpc_addr_parsed = grpc_listen_addr.parse()?;

    // create the grpc wrappers and listen
    let echo_service = EchoService::new(burn_cpu_amount, burn_spawn_blocking);
    let echo_server = EchoServer::new(echo_service);
    Server::builder()
        .add_service(echo_server)
        .serve(grpc_addr_parsed)
        .await?;

    match tokio_tcp_echo_handle.await {
        Ok(echo_result) => match echo_result {
            Ok(()) => Ok(()),
            Err(e) => ErrorMessageOnly::err(e),
        },
        Err(e) => ErrorMessageOnly::err(e),
    }
}

async fn tokio_echo_server(
    tcp_tokio_listener: tokio::net::TcpListener,
    burn_cpu_amount: Duration,
    burn_spawn_blocking: bool,
) -> Result<(), std::io::Error> {
    loop {
        let (connection, _) = tcp_tokio_listener.accept().await?;
        tokio::spawn(tokio_echo_connection_no_err(
            connection,
            burn_cpu_amount,
            burn_spawn_blocking,
        ));
    }
}

async fn tokio_echo_connection_no_err(
    connection: tokio::net::TcpStream,
    burn_cpu_amount: Duration,
    burn_spawn_blocking: bool,
) {
    match tokio_echo_connection(connection, burn_cpu_amount, burn_spawn_blocking).await {
        Ok(()) => {}
        Err(err) => {
            panic!("tokio echo connection failed: {err}");
        }
    }
}

async fn tokio_echo_connection(
    connection: tokio::net::TcpStream,
    burn_cpu_amount: Duration,
    burn_spawn_blocking: bool,
) -> Result<(), tokio::io::Error> {
    let (read_connection, mut write_connection) = connection.into_split();
    let mut reader = tokio::io::BufReader::with_capacity(TCP_INITIAL_BUFFER_BYTES, read_connection);
    let mut buf = String::new();
    loop {
        buf.clear();
        reader.read_line(&mut buf).await?;
        if buf.is_empty() {
            // connection closed: stop echoing
            break;
        }
        assert!(buf.ends_with('\n'));

        tokio_burn_cpu(burn_cpu_amount, burn_spawn_blocking).await;
        write_connection.write_all(buf.as_bytes()).await?;
    }

    Ok(())
}

#[derive(Debug)]
struct ErrorMessageOnly {
    message: String,
}

impl ErrorMessageOnly {
    fn err<E: std::error::Error>(e: E) -> Result<(), Self> {
        Err(Self {
            message: format!("{e}"),
        })
    }
}

impl std::fmt::Display for ErrorMessageOnly {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> Result<(), std::fmt::Error> {
        write!(formatter, "{}", self.message)
    }
}

impl std::error::Error for ErrorMessageOnly {}

impl From<std::io::Error> for ErrorMessageOnly {
    fn from(err: std::io::Error) -> Self {
        Self {
            message: err.to_string(),
        }
    }
}

impl From<std::net::AddrParseError> for ErrorMessageOnly {
    fn from(err: std::net::AddrParseError) -> Self {
        Self {
            message: err.to_string(),
        }
    }
}

impl From<tonic::transport::Error> for ErrorMessageOnly {
    fn from(err: tonic::transport::Error) -> Self {
        Self {
            message: err.to_string(),
        }
    }
}

fn run_thread_server(
    listener: TcpListener,
    burn_cpu_amount: Duration,
) -> Result<(), ErrorMessageOnly> {
    for connection_result in listener.incoming() {
        let connection = connection_result?;
        thread::spawn(move || run_thread_echo_no_errors(connection, burn_cpu_amount));
    }
    drop(listener);
    Ok(())
}

fn run_thread_echo_no_errors(connection: TcpStream, burn_cpu_amount: Duration) {
    run_thread_echo(connection, burn_cpu_amount).expect("echo thread must not return an error");
}

/// Uses user CPU time until amount time has passed. Does nothing if amount <= 0.
/// This simulates executing synchronous code that takes a while, e.g. compressing a large block
/// of data, or generating an asymmetric key pair.
fn burn_cpu(amount: Duration) {
    if amount > Duration::ZERO {
        let start = std::time::Instant::now();
        while start.elapsed() < amount {}
    }
}

fn run_thread_echo(
    mut connection: TcpStream,
    burn_cpu_amount: Duration,
) -> Result<(), Box<dyn std::error::Error>> {
    let reader_connection = connection.try_clone()?;
    let mut reader = BufReader::with_capacity(TCP_INITIAL_BUFFER_BYTES, reader_connection);
    let mut buf = String::new();
    loop {
        buf.clear();
        reader.read_line(&mut buf)?;
        if buf.is_empty() {
            // connection closed: stop echoing
            break;
        }
        assert!(buf.ends_with('\n'));

        burn_cpu(burn_cpu_amount);

        connection.write_all(buf.as_bytes())?;
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    #[test]
    fn test_go_parse_duration() {
        struct TestDefinition<'a> {
            input: &'a str,
            expected: Duration,
        }
        let tests = vec![
            TestDefinition {
                input: "0",
                expected: Duration::from_secs(0),
            },
            TestDefinition {
                input: "1s",
                expected: Duration::from_secs(1),
            },
            TestDefinition {
                input: "1000ms",
                expected: Duration::from_millis(1000),
            },
            TestDefinition {
                input: "42us",
                expected: Duration::from_micros(42),
            },
        ];

        for test in tests {
            let actual = super::parse_go_duration(test.input).unwrap();
            assert_eq!(actual, test.expected);
        }

        let invalid_inputs = vec![
            "1",
            "0xa6s",
            "1a5s",
            "1h",
            "-2s",
            "100sms",
            "5ns",
            "100us100ms",
            "100msus",
        ];
        for input in invalid_inputs {
            let err = super::parse_go_duration(input).unwrap_err();
            assert!(
                err.to_string().contains("invalid Go duration string \""),
                "unexpected err={err}",
            );
        }
    }
}
