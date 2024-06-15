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
struct EchoService;

impl EchoService {
    const fn new() -> Self {
        Self {}
    }
}

#[tonic::async_trait]
impl Echo for EchoService {
    async fn echo(&self, request: Request<EchoRequest>) -> Result<Response<EchoResponse>, Status> {
        let r = request.into_inner();
        Ok(tonic::Response::new(EchoResponse { output: r.input }))
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

    raise_nofile_rlimit()?;

    println!(
        "listening for TCP thread connections on {} ...",
        args.tcp_thread_listen_addr
    );
    let tcp_thread_listener = TcpListener::bind(args.tcp_thread_listen_addr)?;

    let thread_server_handle = thread::spawn(move || run_thread_server(tcp_thread_listener));

    let tokio_handle =
        thread::spawn(move || run_tokio(&args.grpc_listen_addr, &args.tcp_tokio_listen_addr));

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
fn run_tokio(grpc_listen_addr: &str, tcp_tokio_listen_addr: &str) -> Result<(), ErrorMessageOnly> {
    let runtime = tokio::runtime::Runtime::new()?;
    let grpc_listen_addr = grpc_listen_addr.to_string();
    let tcp_tokio_listen_addr = tcp_tokio_listen_addr.to_string();
    runtime.block_on(async move { tokio_main(grpc_listen_addr, tcp_tokio_listen_addr).await })?;
    Ok(())
}

async fn tokio_main(
    grpc_listen_addr: String,
    tcp_tokio_listen_addr: String,
) -> Result<(), ErrorMessageOnly> {
    println!("listening for Tokio TCP on {tcp_tokio_listen_addr} ...");
    let tcp_tokio_parsed: std::net::SocketAddr = tcp_tokio_listen_addr.parse()?;
    let tcp_tokio_listener =
        tokio::net::TcpListener::bind((tcp_tokio_parsed.ip(), tcp_tokio_parsed.port())).await?;
    let tokio_tcp_echo_handle = tokio::spawn(tokio_echo_server(tcp_tokio_listener));

    println!("listening for gRPC on {grpc_listen_addr} ...");
    let grpc_addr_parsed = grpc_listen_addr.parse()?;

    // create the grpc wrappers and start listening
    let echo_service = EchoService::new();
    let echo_server = EchoServer::new(echo_service);
    Server::builder()
        .add_service(echo_server)
        .serve(grpc_addr_parsed)
        .await?;

    match tokio_tcp_echo_handle.await {
        Ok(_) => Ok(()),
        Err(e) => ErrorMessageOnly::err(e),
    }
}

async fn tokio_echo_server(
    tcp_tokio_listener: tokio::net::TcpListener,
) -> Result<(), std::io::Error> {
    loop {
        let (connection, _) = tcp_tokio_listener.accept().await?;
        tokio::spawn(tokio_echo_connection_no_err(connection));
    }
}

async fn tokio_echo_connection_no_err(connection: tokio::net::TcpStream) {
    match tokio_echo_connection(connection).await {
        Ok(()) => {}
        Err(err) => {
            panic!("tokio echo connection failed: {err}");
        }
    }
}

async fn tokio_echo_connection(connection: tokio::net::TcpStream) -> Result<(), tokio::io::Error> {
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

fn run_thread_server(listener: TcpListener) -> Result<(), ErrorMessageOnly> {
    for connection_result in listener.incoming() {
        let connection = connection_result?;
        thread::spawn(move || run_thread_echo_no_errors(connection));
    }
    drop(listener);
    Ok(())
}

fn run_thread_echo_no_errors(connection: TcpStream) {
    run_thread_echo(connection).expect("echo thread must not return an error");
}

fn run_thread_echo(mut connection: TcpStream) -> Result<(), Box<dyn std::error::Error>> {
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

        connection.write_all(buf.as_bytes())?;
    }

    Ok(())
}
