package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/evanj/netpingbench/echopb"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
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
	resp := &echopb.EchoResponse{
		Output: request.Input,
	}
	return resp, nil
}

func main() {
	listenAddr := flag.String("listenAddr", "localhost",
		"listening address: use empty to listen on all devices")
	tcpPort := flag.Int("tcpPort", 8001, "port for TCP echo requests")
	otherTCPPort := flag.Int("otherTCPPort", 0, "additional port for TCP echo requests (use for tokio async)")
	grpcPort := flag.Int("grpcPort", 8002, "port for gRPC echo requests")
	runDuration := flag.Duration("runDuration", 10*time.Second, "time to run the throughput test")
	remoteAddr := flag.String("remoteAddr", "",
		"IP/DNS name for remote endpoints. If empty: run servers only")
	throughputThreads := flag.Int("throughputThreads", 1, "threads to run the throughput test")
	throughputThreadsEnd := flag.Int("throughputThreadsEnd", 0,
		"if specified, sweep [throughputThreads, throughputThreadsEnd], doubling each time")
	grpcChannels := flag.Int("grpcChannels", 1, "number of separate gRPC channels to use (max=throughputThreads)")
	flag.Parse()

	if *remoteAddr == "" {
		runServers(*listenAddr, *tcpPort, *grpcPort)
		return
	}
	if *grpcChannels < 1 {
		fmt.Fprintf(os.Stderr, "ERROR: --grpcChannels=%d; must be >= 1", *grpcChannels)
	}
	if *throughputThreads < 1 {
		fmt.Fprintf(os.Stderr, "ERROR: --throughputThreads=%d; must be >= 1", *throughputThreads)
	}
	if *throughputThreadsEnd != 0 && *throughputThreadsEnd < *throughputThreads {
		fmt.Fprintf(os.Stderr, "ERROR: --throughputThreadsEnd=%d; must be >= throughputThreads=%d",
			*throughputThreadsEnd, *throughputThreads)
	}

	if *throughputThreadsEnd == 0 {
		*throughputThreadsEnd = *throughputThreads
	}

	// TCP test: need separate connections for each thread
	runTCPThroughputBenchmark(*remoteAddr, *tcpPort, *throughputThreads, *throughputThreadsEnd, *runDuration, "tcp")
	if *otherTCPPort != 0 {
		runTCPThroughputBenchmark(*remoteAddr, *otherTCPPort, *throughputThreads, *throughputThreadsEnd, *runDuration, "other_tcp")
	}

	runGRPCThroughputBenchmark(*remoteAddr, *grpcPort, *throughputThreads, *throughputThreadsEnd, *runDuration, *grpcChannels)
}

func runTCPThroughputBenchmark(addr string, port int, numClientsStart int, numClientsEnd int, runDuration time.Duration, label string) error {
	// TCP test: need separate connections for each thread
	var clients []echoClient
	for i := 0; i < numClientsEnd; i++ {
		client, err := newTCPEchoClient(addr, port)
		if err != nil {
			return err
		}
		clients = append(clients, client)
	}
	return runThroughputSweep(clients, numClientsStart, numClientsEnd, runDuration, label)
}

func runGRPCThroughputBenchmark(addr string, port int, numClientsStart int, numClientsEnd int, runDuration time.Duration, grpcChannels int) error {
	// gRPC test: share a number of channels
	var channels []echoClient
	for i := 0; i < grpcChannels; i++ {
		channel, err := newGRPCEchoClient(addr, port)
		if err != nil {
			panic(err)
		}
		channels = append(channels, channel)
	}
	// distribute the channels across threads
	var clients []echoClient
	for i := 0; i < numClientsEnd; i++ {
		clients = append(clients, channels[i%grpcChannels])
	}
	grpcLabel := fmt.Sprintf("grpc-channels=%d", grpcChannels)
	return runThroughputSweep(clients, numClientsStart, numClientsEnd, runDuration, grpcLabel)
}

func runThroughputClient(client echoClient, requestsChan chan<- int, exit *atomic.Bool) {
	ctx := context.Background()
	requests := 0
	for !exit.Load() {
		err := client.Echo(ctx, echoMessage)
		if err != nil {
			panic(err)
		}
		requests += 1
	}
	requestsChan <- requests
}

func runThroughputSweep(clients []echoClient, numClientsStart int, numClientsEnd int, runDuration time.Duration, label string) error {
	if !(numClientsStart <= numClientsEnd) {
		return fmt.Errorf("numClientsStart=%d must be <= numClientsEnd=%d", numClientsStart, numClientsEnd)
	}
	if numClientsStart < 1 {
		return fmt.Errorf("numClientsStart=%d must be >= 1", numClientsStart)
	}
	if len(clients) != numClientsEnd {
		return fmt.Errorf("len(clients)=%d must equal numClientsEnd=%d", len(clients), numClientsEnd)
	}

	for numClients := 1; numClients <= numClientsEnd; numClients *= 2 {
		err := runThroughputBenchmark(clients[0:numClients], runDuration, label)
		if err != nil {
			return err
		}
	}
	return nil
}

func runThroughputBenchmark(clients []echoClient, runDuration time.Duration, label string) error {
	startUsage := unix.Rusage{}
	endUsage := unix.Rusage{}

	requestsChan := make(chan int)
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

	total := 0
	for i := 0; i < len(clients); i++ {
		total += <-requestsChan
	}

	clientCPU := time.Duration(endUsage.Utime.Nano() + endUsage.Stime.Nano() -
		startUsage.Utime.Nano() - startUsage.Stime.Nano())
	slog.Info("throughput result",
		slog.String("label", label),
		slog.Int("threads", len(clients)),
		slog.Float64("requests_per_sec", float64(total)/runDuration.Seconds()),
		slog.Duration("client_cpu_ns", clientCPU),
		slog.Float64("client_avg_cpu_cores", clientCPU.Seconds()/runDuration.Seconds()),
	)
	return nil
}

func runServers(listenAddr string, tcpPort int, grpcPort int) {
	slog.Info("starting TCP and gRPC servers ...",
		slog.String("listenAddr", listenAddr),
		slog.Int("grpcPort", grpcPort),
		slog.Int("tcpPort", tcpPort))

	err := startTCPEchoListener(listenAddr, tcpPort)
	if err != nil {
		panic(err)
	}

	err = startGRPCEchoServer(listenAddr, grpcPort)
	if err != nil {
		panic(err)
	}

	slog.Info("blocking forever to let servers run ...")
	<-make(chan struct{})
}

type echoClient interface {
	Echo(ctx context.Context, message string) error
}

type tcpEchoClient struct {
	conn net.Conn
	buf  []byte
}

func newTCPEchoClient(addr string, port int) (*tcpEchoClient, error) {
	conn, err := net.Dial("tcp", addr+":"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	return &tcpEchoClient{conn, make([]byte, 0, tcpInitialBufferBytes)}, nil
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
	return nil
}

type grpcEchoClient struct {
	client echopb.EchoClient
}

func newGRPCEchoClient(addr string, port int) (*grpcEchoClient, error) {
	conn, err := grpc.NewClient(addr+":"+strconv.Itoa(port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &grpcEchoClient{echopb.NewEchoClient(conn)}, nil
}

func (c *grpcEchoClient) Echo(ctx context.Context, message string) error {
	_, err := c.client.Echo(ctx, &echopb.EchoRequest{Input: message})
	return err
}

func startTCPEchoListener(addr string, port int) error {
	lis, err := net.Listen("tcp", addr+":"+strconv.Itoa(port))
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := lis.Accept()
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

func startGRPCEchoServer(addr string, port int) error {
	lis, err := net.Listen("tcp", addr+":"+strconv.Itoa(port))
	if err != nil {
		panic(err)
	}

	s := grpc.NewServer()
	echopb.RegisterEchoServer(s, newEchoServer())

	go func() {
		err := s.Serve(lis)
		if err != nil {
			slog.Error("failed serving gRPC", slog.String("error", err.Error()))
			panic(err)
		}
	}()
	return nil
}
