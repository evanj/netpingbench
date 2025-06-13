package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/evanj/hacks/trivialstats"
	"github.com/evanj/netpingbench/echopb"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const echoMessage = "ping"
const tcpInitialBufferBytes = 4096

type echoServer struct {
	echopb.UnimplementedEchoServer
}

func newEchoServer() *echoServer {
	return &echoServer{echopb.UnimplementedEchoServer{}}
}

func (s *echoServer) Echo(ctx context.Context, request *echopb.EchoRequest) (*echopb.EchoResponse, error) {
	resp := &echopb.EchoResponse{}
	resp.SetOutput(request.GetInput())
	return resp, nil
}

func main() {
	listenAddr := flag.String("listenAddr", "localhost",
		"listening address: use empty to listen on all devices")
	tcpPort := flag.Int("tcpPort", 8001, "port for TCP echo requests")
	otherTCPPort := flag.Int("otherTCPPort", 0,
		"additional port for TCP echo requests (use for tokio async)")
	tcpNewConnectionPerRequest := flag.Bool("tcpNewConnectionPerRequest", false,
		"use a new TCP connection for each request")
	tls := flag.Bool("tls", false, "generate a self-signed TLS certificate and test TLS")
	grpcPort := flag.Int("grpcPort", 8002, "port for gRPC echo requests")
	runDuration := flag.Duration("runDuration", 10*time.Second, "time to run the throughput test")
	remoteAddr := flag.String("remoteAddr", "",
		"IP/DNS name for remote endpoints. If empty: run servers only")
	throughputThreads := flag.Int("throughputThreads", 1, "threads to run the throughput test")
	throughputThreadsEnd := flag.Int("throughputThreadsEnd", 0,
		"If set: sweep [throughputThreads, throughputThreadsEnd], doubling each time")
	grpcChannels := flag.Int("grpcChannels", 1,
		"number of separate gRPC channels to use (max=throughputThreads)")
	grpcChannelsEnd := flag.Int("grpcChannelsEnd", 0,
		"If set: sweep [grpcChannels, grpcChannelsEnd] gRPC channels, doubling each time")
	flag.Parse()

	if *remoteAddr == "" {
		err := runServers(*listenAddr, *tcpPort, *grpcPort, *tls)
		if err != nil {
			panic(err)
		}
		return
	}
	if *grpcChannels < 1 {
		fmt.Fprintf(os.Stderr, "ERROR: --grpcChannels=%d; must be >= 1\n", *grpcChannels)
		os.Exit(1)
	}
	if *throughputThreads < 1 {
		fmt.Fprintf(os.Stderr, "ERROR: --throughputThreads=%d; must be >= 1\n", *throughputThreads)
		os.Exit(1)
	}
	if *throughputThreadsEnd != 0 && *throughputThreadsEnd < *throughputThreads {
		fmt.Fprintf(os.Stderr, "ERROR: --throughputThreadsEnd=%d; must be >= throughputThreads=%d\n",
			*throughputThreadsEnd, *throughputThreads)
		os.Exit(1)
	}
	if *throughputThreadsEnd == 0 {
		*throughputThreadsEnd = *throughputThreads
	}

	if *grpcChannelsEnd != 0 {
		if *grpcChannelsEnd < *grpcChannels {
			fmt.Fprintf(os.Stderr, "ERROR: --grpcChannelsEnd=%d; must be >= grpcChannels=%d\n",
				*grpcChannelsEnd, *grpcChannels)
			os.Exit(1)
		}
		if *grpcChannelsEnd > *throughputThreadsEnd {
			fmt.Fprintf(os.Stderr, "ERROR: --grpcChannelsEnd=%d; must be <= throughputThreadsEnd=%d\n",
				*grpcChannelsEnd, *throughputThreadsEnd)
			os.Exit(1)
		}
	}
	if *grpcChannelsEnd == 0 {
		*grpcChannelsEnd = *grpcChannels
	}

	// TCP test: need separate connections for each thread
	runTCPThroughputBenchmark(
		*remoteAddr, *tcpPort, *tls, *throughputThreads, *throughputThreadsEnd, *runDuration,
		"tcp", *tcpNewConnectionPerRequest)
	if *otherTCPPort != 0 {
		runTCPThroughputBenchmark(
			*remoteAddr, *otherTCPPort, *tls, *throughputThreads, *throughputThreadsEnd, *runDuration,
			"other_tcp", *tcpNewConnectionPerRequest)
	}

	runGRPCThroughputBenchmark(*remoteAddr, *grpcPort, *tls,
		*throughputThreads, *throughputThreadsEnd, *runDuration, *grpcChannels, *grpcChannelsEnd)
}

func runTCPThroughputBenchmark(
	addr string, port int, tlsEnabled bool, numClientsStart int, numClientsEnd int,
	runDuration time.Duration, label string, tcpNewConnections bool,
) error {
	// TCP test: need separate connections for each thread
	var clients []echoClient
	for i := 0; i < numClientsEnd; i++ {
		client, err := newTCPEchoClient(addr, port, tlsEnabled, tcpNewConnections)
		if err != nil {
			return err
		}
		clients = append(clients, client)
	}
	return runThroughputSweep(clients, numClientsStart, numClientsEnd, runDuration, label, nil)
}

func runGRPCThroughputBenchmark(
	addr string, port int, tlsEnabled bool, numClientsStart int, numClientsEnd int,
	runDuration time.Duration, grpcChannelsStart int, grpcChannelsEnd int,
) error {
	// gRPC test: share a number of channels
	var channels []echoClient
	for i := 0; i < grpcChannelsEnd; i++ {
		channel, err := newGRPCEchoClient(addr, port, tlsEnabled)
		if err != nil {
			panic(err)
		}
		channels = append(channels, channel)
	}
	// distribute the channels across threads
	for grpcChannels := grpcChannelsStart; grpcChannels <= grpcChannelsEnd; grpcChannels *= 2 {
		var clients []echoClient
		for i := 0; i < numClientsEnd; i++ {
			clients = append(clients, channels[i%grpcChannels])
		}
		grpcChannelsAttr := []slog.Attr{slog.Int("grpc_channels", grpcChannels)}

		// numClientsStart must be >= grpcChannels: otherwise we are using fewer channels
		numClientsStartThisRun := numClientsStart
		if grpcChannels > numClientsStartThisRun {
			numClientsStartThisRun = grpcChannels
		}
		err := runThroughputSweep(
			clients, numClientsStartThisRun, numClientsEnd, runDuration, "grpc", grpcChannelsAttr,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

type clientStats struct {
	requests            int
	latencyDistribution *trivialstats.Distribution
}

func runThroughputClient(client echoClient, requestsChan chan<- clientStats, exit *atomic.Bool) {
	ctx := context.Background()
	requests := 0
	latencyDistribution := trivialstats.NewDistribution()
	for !exit.Load() {
		start := time.Now()
		err := client.Echo(ctx, echoMessage)
		requestLatency := time.Since(start)
		if err != nil {
			panic(err)
		}
		requests += 1
		latencyDistribution.Add(int64(requestLatency))
	}
	stats := clientStats{
		requests:            requests,
		latencyDistribution: latencyDistribution,
	}
	requestsChan <- stats
}

func runThroughputSweep(
	clients []echoClient, numClientsStart int, numClientsEnd int, runDuration time.Duration,
	label string, extraAttrs []slog.Attr,
) error {

	if !(numClientsStart <= numClientsEnd) {
		return fmt.Errorf("numClientsStart=%d must be <= numClientsEnd=%d", numClientsStart, numClientsEnd)
	}
	if numClientsStart < 1 {
		return fmt.Errorf("numClientsStart=%d must be >= 1", numClientsStart)
	}
	if len(clients) != numClientsEnd {
		return fmt.Errorf("len(clients)=%d must equal numClientsEnd=%d", len(clients), numClientsEnd)
	}

	for numClients := numClientsStart; numClients <= numClientsEnd; numClients *= 2 {
		err := runThroughputBenchmark(clients[0:numClients], runDuration, label, extraAttrs)
		if err != nil {
			return err
		}
	}
	return nil
}

func runThroughputBenchmark(
	clients []echoClient, runDuration time.Duration, label string, extraAttrs []slog.Attr,
) error {

	startUsage := unix.Rusage{}
	endUsage := unix.Rusage{}

	requestsChan := make(chan clientStats)
	exit := &atomic.Bool{}
	for _, client := range clients {
		go runThroughputClient(client, requestsChan, exit)
	}

	err1 := unix.Getrusage(unix.RUSAGE_SELF, &startUsage)
	time.Sleep(runDuration)
	err2 := unix.Getrusage(unix.RUSAGE_SELF, &endUsage)
	exit.Store(true)

	err := errors.Join(err1, err2)
	if err != nil {
		return err
	}

	total := clientStats{
		requests:            0,
		latencyDistribution: trivialstats.NewDistribution(),
	}
	for i := 0; i < len(clients); i++ {
		clientStats := <-requestsChan
		total.requests += clientStats.requests
		total.latencyDistribution.Merge(clientStats.latencyDistribution)
	}

	clientCPU := time.Duration(endUsage.Utime.Nano() + endUsage.Stime.Nano() -
		startUsage.Utime.Nano() - startUsage.Stime.Nano())

	latencyStats := total.latencyDistribution.Stats()
	slogAttrs := []slog.Attr{
		slog.String("label", label),
		slog.Int("threads", len(clients)),
		slog.Float64("requests_per_sec", float64(total.requests)/runDuration.Seconds()),
		slog.Duration("client_cpu_time", clientCPU),
		slog.Float64("client_cpu_avg_cores", clientCPU.Seconds()/runDuration.Seconds()),
		slog.Duration("latency_p50", time.Duration(latencyStats.P50)),
		slog.Duration("latency_p95", time.Duration(latencyStats.P95)),
		slog.Duration("latency_p99", time.Duration(latencyStats.P99)),
	}
	slogAttrs = append(slogAttrs, extraAttrs...)
	ctx := context.Background()
	slog.LogAttrs(ctx, slog.LevelInfo, "throughput result", slogAttrs...)
	return nil
}

func newTCPListener(addr string, port int) (net.Listener, error) {
	return net.Listen("tcp", addr+":"+strconv.Itoa(port))
}

func runServers(listenAddr string, tcpPort int, grpcPort int, tlsEnabled bool) error {
	slog.Info("starting TCP and gRPC servers ...",
		slog.String("listenAddr", listenAddr),
		slog.Int("grpcPort", grpcPort),
		slog.Int("tcpPort", tcpPort),
		slog.Bool("tlsEnabled", tlsEnabled),
	)

	tcpListener, err := newTCPListener(listenAddr, tcpPort)
	if err != nil {
		return err
	}
	grpcListener, err := newTCPListener(listenAddr, grpcPort)
	if err != nil {
		return err
	}

	var tlsConfig *tls.Config
	if tlsEnabled {
		slog.Info("generating self-signed TLS certificate ...")
		cert, err := newSelfSignedCertificate(nil)
		if err != nil {
			return err
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		tcpListener = tls.NewListener(tcpListener, tlsConfig)
	}

	startTCPEchoListener(tcpListener)
	startGRPCEchoServer(grpcListener, tlsConfig)

	slog.Info("blocking forever to let servers run ...")
	<-make(chan struct{})
	return nil
}

type echoClient interface {
	Echo(ctx context.Context, message string) error
}

type tcpEchoClient struct {
	conn                    net.Conn
	buf                     []byte
	addrWithPort            string
	newConnectionPerRequest bool
}

func newTCPEchoClient(addr string, port int, tlsEnabled bool, tcpNewConnectionPerRequest bool) (*tcpEchoClient, error) {
	addrWithPort := addr + ":" + strconv.Itoa(port)
	var conn net.Conn
	var err error
	if tlsEnabled {
		conn, err = tls.Dial("tcp", addrWithPort, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = net.Dial("tcp", addrWithPort)
	}
	if err != nil {
		return nil, err
	}
	return &tcpEchoClient{conn, make([]byte, 0, tcpInitialBufferBytes), addrWithPort, tcpNewConnectionPerRequest}, nil
}

func (c *tcpEchoClient) Echo(ctx context.Context, message string) error {
	c.buf = append(c.buf[:0], message...)
	c.buf = append(c.buf, '\n')
	n, err := c.conn.Write(c.buf)
	if err != nil {
		return err
	}
	if n != len(message)+1 {
		// Should be impossible: Write must return an error if it returns a short write
		// but this does test that we created the buffer correctly
		panic(fmt.Sprintf("tcp echo: must write len(message)+1=%d ; wrote %d", len(message)+1, n))
	}
	n, err = io.ReadFull(c.conn, c.buf)
	if err != nil {
		return err
	}
	if n != len(message)+1 {
		panic(fmt.Sprintf("tcp echo: expected to read %d bytes in reply; read %d",
			len(message)+1, n))
	}

	if c.newConnectionPerRequest {
		err = c.conn.Close()
		if err != nil {
			return err
		}
		conn, err := net.Dial("tcp", c.addrWithPort)
		if err != nil {
			return err
		}
		c.conn = conn
	}
	return nil
}

type grpcEchoClient struct {
	client echopb.EchoClient
}

func newGRPCEchoClient(addr string, port int, tlsEnabled bool) (*grpcEchoClient, error) {
	var transportCredentials credentials.TransportCredentials
	if tlsEnabled {
		transportCredentials = newInsecureTrustAnyCert()
	} else {
		transportCredentials = insecure.NewCredentials()
	}
	conn, err := grpc.NewClient(addr+":"+strconv.Itoa(port), grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		return nil, err
	}
	return &grpcEchoClient{echopb.NewEchoClient(conn)}, nil
}

func (c *grpcEchoClient) Echo(ctx context.Context, message string) error {
	req := &echopb.EchoRequest{}
	req.SetInput(message)
	_, err := c.client.Echo(ctx, req)
	return err
}

func startTCPEchoListener(listener net.Listener) error {
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				slog.Error("failed accepting connection", slog.String("error", err.Error()))
				continue
			}

			go handleTCPEchoConnection(conn)
		}
	}()

	return nil
}

func handleTCPEchoConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		line = append(line, '\n')
		_, err := conn.Write(line)
		if err != nil {
			slog.Error("failed to write to connection", slog.String("error", err.Error()))
			return
		}
	}
	err := scanner.Err()
	if err != nil {
		slog.Error("failed reading from connection", slog.String("error", err.Error()))
	}
	err = conn.Close()
	if err != nil {
		slog.Error("failed closing connection", slog.String("error", err.Error()))
	}
}

func startGRPCEchoServer(listener net.Listener, tlsConfig *tls.Config) error {
	var serverOptions []grpc.ServerOption
	if tlsConfig != nil {
		serverOptions = []grpc.ServerOption{grpc.Creds(credentials.NewTLS(tlsConfig))}
	}
	s := grpc.NewServer(serverOptions...)
	echopb.RegisterEchoServer(s, newEchoServer())

	go func() {
		err := s.Serve(listener)
		if err != nil {
			slog.Error("failed serving gRPC", slog.String("error", err.Error()))
			panic(err)
		}
	}()
	return nil
}

// newSelfSignedCertificates generates an ECDSA P256 key and a self-signed certificate.
// The hosts list must contain all DNS names or IP addresses used to connect to this server.
// Based on generate_cert:
// https://github.com/golang/go/blob/master/src/crypto/tls/generate_cert.go
func newSelfSignedCertificate(hosts []string) (tls.Certificate, error) {
	if len(hosts) == 0 {
		hosts = []string{"localhost", "127.0.0.1"}
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	notBefore := time.Now().Add(-5 * time.Minute).UTC()
	const selfSignedValidYears = 1
	notAfter := notBefore.AddDate(selfSignedValidYears, 0, 0).UTC()

	template := x509.Certificate{
		// must be unique to avoid errors when serial/issuer is reused with different keys
		SerialNumber: new(big.Int).SetInt64(getRandomSerial()),
		Subject: pkix.Name{
			Organization: []string{"Example Inc"},
			// does not seem to be required, but makes it more similar to "real" keys
			CommonName: hosts[0],
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEMBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEMBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	return tls.X509KeyPair(certPEMBlock, keyPEMBlock)
}

// getRandomSerial returns a random 64-bit serial number for a TLS certificate. The serial number
// is used by some system to cache keys, so it must change when we change the key. With 63 bits
// the collision probability is unlikely.
func getRandomSerial() int64 {
	var id int64 = 0
	// do not permit an id of zero
	for id == 0 {
		err := binary.Read(rand.Reader, binary.LittleEndian, &id)
		if err != nil {
			panic("binary.Read failed: " + err.Error())
		}
	}

	// clear the top bit to force it to be positive
	id &= ^(math.MinInt64)
	return id
}

// newInsecureTrustAnyCert returns gRPC credentials which uses TLS but trusts any certificate.
func newInsecureTrustAnyCert() credentials.TransportCredentials {
	return insecureTrustAnyCertTC{}
}

type insecureTrustAnyCertTC struct{}

type insecureTrustAnyCertAuthInfo struct {
	credentials.CommonAuthInfo
}

func (insecureTrustAnyCertAuthInfo) AuthType() string {
	return "insecure_tls"
}

func (insecureTrustAnyCertTC) ClientHandshake(
	ctx context.Context, authorityHeader string, rawConn net.Conn,
) (net.Conn, credentials.AuthInfo, error) {
	// Modified from grpc-go's TLS implementation:
	// https://github.com/grpc/grpc-go/blob/master/credentials/tls.go

	// serverName, _, err := net.SplitHostPort(authorityHeader)
	// if err != nil {
	//      // If the authority had no host port or if the authority cannot be parsed, use it as-is.
	//      serverName = authorityHeader
	// }
	// cfg := &tls.Config{
	//      ServerName: serverName,
	// }
	cfg := &tls.Config{
		InsecureSkipVerify: true,
		// HTTP/2 requires ALPN to negotiate "h2"; gRPC requires this also
		NextProtos: []string{"h2"},
	}

	conn := tls.Client(rawConn, cfg)
	errChannel := make(chan error, 1)
	go func() {
		errChannel <- conn.Handshake()
		close(errChannel)
	}()
	select {
	case err := <-errChannel:
		if err != nil {
			conn.Close()
			return nil, nil, err
		}
	case <-ctx.Done():
		conn.Close()
		return nil, nil, ctx.Err()
	}

	authInfo := insecureTrustAnyCertAuthInfo{
		CommonAuthInfo: credentials.CommonAuthInfo{
			// This actually provides Privacy but no Integrity, but that is not an option
			SecurityLevel: credentials.NoSecurity,
		},
	}

	return conn, authInfo, nil
}

func (insecureTrustAnyCertTC) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	panic("ServerHandshake not implemented: only for use with clients")
}

func (insecureTrustAnyCertTC) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "insecureTLS"}
}

func (insecureTrustAnyCertTC) Clone() credentials.TransportCredentials {
	return insecureTrustAnyCertTC{}
}

func (insecureTrustAnyCertTC) OverrideServerName(serverName string) error {
	return fmt.Errorf("insecureTrustAnyCertTC does not support OverrideServerName (serverName=%s)",
		serverName)
}
