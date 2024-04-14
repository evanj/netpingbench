# netpingbench: Rough TCP and gRPC benchmark in Go and Rust

An attempt to compare "out of the box" performance for small network request/response using TCP and gRPC in Go and Rust, on a "reasonable" sized server (8 cores, 16 hyperthreads). I wanted a some rough understanding of what performance we should expect on modern systems in the cloud. This is a very synthetic benchmark, since the application does nothing. It really is testing how efficiently these runtimes use the operating system, since nearly all the time is spent doing network I/O.

## Results Summary

* Max TCP throughput: 1250k reqs/sec using Rust async tokio; 1140k reqs/sec using Go; 1000k reqs/sec using Rust threads.
* Max gRPC throughput: 390k reqs/sec using Go, or 355k reqs/sec using Rust Tonic. Neither implementation uses all the available CPU, although they get close. 
* Both gRPC implementations were unable to use 10)% need many connections to 
*TL;DR*: Max of about 1M reqs/sec using raw TCP, 500k reqs/sec using Go+gRPC, and 350k reqs/sec with  To scale gRPC you will need to use lots of connections, like at least 1 for every 4 CPU cores. Rust is about the same as Go in this benchmark, although Rust+Tonic gRPC is a bit slower than Go+gRPC.

Rust+Tonic gRPC is quite a bit less efficient than Go+gRPC at parallelizing work. I only got about half the throughput I got with Go + gRPC.


## Benchmark details

The benchmark is a Go client that starts many Goroutines making "echo" requests. Each one waits for the response then immediately sends the next request (a closed loop load generator). I did not measure latency at all, since my goal was only to get a rough sense of the "best" throughput. A careful benchmark should absolutely measure latency!

The servers ran on a Google Cloud c3d-highcpu-16 machine (AMD EPYC Genoa 4th Generation, 8 cores, 16 hyperthreads, 32 GiB RAM, 20 Gbps network), and the client running on a c3d-highcpu-60 (30 cores, 60 hyperthreads). The goal is the server should be the bottleneck. During the gRPC benchmarks in particular the CPU on the client according to top sometimes spiked up to around 4000% (40/60 VCPUs).

* Go TCP: max 1014k reqs/sec with 512 client threads.
* Go gRPC: max 390k reqs/sec with 8192 client threads and 256 connections. As a rough rule of thumb: for best throughput you need many  connections, but too many connections and throughput will decrease again. With one connection it only gets up to about 110k reqs/sec and doesn't use that much CPU.
* Rust TCP threads: max of 1050k reqs/sec with 1024 client threads.
* Rust tonic gRPC: 355k reqs/sec with 8192 client threads and 256 connections. With 1 connection, it only can use about ~4 CPU cores, and gets a max of 100k reqs/sec. 


## Benchmark results Google C3D AMD 

CPU: AMD EPYC Genoa 4th Generation EPYC 9B14
https://cloud.google.com/compute/docs/cpu-platforms



## GCP Network Performance


### Go raw results

```
for i in 1 2 4 8 16 32 64 128 256 512 1024 2048 4096; do
    ./gnetpingbench --remoteAddr=10.128.0.47 --throughputThreads=$i || break;
done

2024/04/14 13:49:12 INFO throughput result label=tcp threads=1 requests_per_sec=19569.1
2024/04/14 13:49:22 INFO throughput result label="grpc 1 channels" threads=1 requests_per_sec=8813.5
2024/04/14 13:49:32 INFO throughput result label=tcp threads=2 requests_per_sec=38510.7
2024/04/14 13:49:42 INFO throughput result label="grpc 1 channels" threads=2 requests_per_sec=16344.1
2024/04/14 13:49:52 INFO throughput result label=tcp threads=4 requests_per_sec=68612
2024/04/14 13:50:02 INFO throughput result label="grpc 1 channels" threads=4 requests_per_sec=27515.6
2024/04/14 13:50:12 INFO throughput result label=tcp threads=8 requests_per_sec=114286.3
2024/04/14 13:50:22 INFO throughput result label="grpc 1 channels" threads=8 requests_per_sec=42749.2
2024/04/14 13:50:32 INFO throughput result label=tcp threads=16 requests_per_sec=176936.6
2024/04/14 13:50:42 INFO throughput result label="grpc 1 channels" threads=16 requests_per_sec=62003.1
2024/04/14 13:50:52 INFO throughput result label=tcp threads=32 requests_per_sec=212155.7
2024/04/14 13:51:02 INFO throughput result label="grpc 1 channels" threads=32 requests_per_sec=75519.9
2024/04/14 13:51:12 INFO throughput result label=tcp threads=64 requests_per_sec=235789.5
2024/04/14 13:51:22 INFO throughput result label="grpc 1 channels" threads=64 requests_per_sec=79815.8
2024/04/14 13:51:32 INFO throughput result label=tcp threads=128 requests_per_sec=377435.9
2024/04/14 13:51:42 INFO throughput result label="grpc 1 channels" threads=128 requests_per_sec=93686.6
2024/04/14 13:51:52 INFO throughput result label=tcp threads=256 requests_per_sec=615539.2
2024/04/14 13:52:02 INFO throughput result label="grpc 1 channels" threads=256 requests_per_sec=103264.5
2024/04/14 13:52:12 INFO throughput result label=tcp threads=512 requests_per_sec=899317.9
2024/04/14 13:52:22 INFO throughput result label="grpc 1 channels" threads=512 requests_per_sec=109860.7
2024/04/14 13:53:25 INFO throughput result label=tcp threads=1024 requests_per_sec=1.0918367e+06
2024/04/14 13:53:35 INFO throughput result label="grpc 1 channels" threads=1024 requests_per_sec=110733.8
2024/04/14 13:53:46 INFO throughput result label=tcp threads=2048 requests_per_sec=1.155089e+06
2024/04/14 13:53:56 INFO throughput result label="grpc 1 channels" threads=2048 requests_per_sec=112870.7
2024/04/14 13:54:13 INFO throughput result label=tcp threads=4096 requests_per_sec=1.0966329e+06
2024/04/14 13:54:23 INFO throughput result label="grpc 1 channels" threads=4096 requests_per_sec=118336.9


for i in 1 2 4 8 16 32 64 128 256 512 1024 2048 4096; do
    ./gnetpingbench --remoteAddr=10.128.0.47 --throughputThreads=$i --grpcChannels=8 || break;
done

2024/04/14 17:50:25 INFO throughput result label=tcp threads=1 requests_per_sec=19121
2024/04/14 17:50:35 INFO throughput result label="grpc 8 channels" threads=1 requests_per_sec=8846.6
2024/04/14 17:50:45 INFO throughput result label=tcp threads=2 requests_per_sec=34193.9
2024/04/14 17:50:55 INFO throughput result label="grpc 8 channels" threads=2 requests_per_sec=16696.2
2024/04/14 17:51:05 INFO throughput result label=tcp threads=4 requests_per_sec=67589.5
2024/04/14 17:51:15 INFO throughput result label="grpc 8 channels" threads=4 requests_per_sec=28461.3
2024/04/14 17:51:25 INFO throughput result label=tcp threads=8 requests_per_sec=112384.2
2024/04/14 17:51:35 INFO throughput result label="grpc 8 channels" threads=8 requests_per_sec=39648.6
2024/04/14 17:51:45 INFO throughput result label=tcp threads=16 requests_per_sec=171059.3
2024/04/14 17:51:55 INFO throughput result label="grpc 8 channels" threads=16 requests_per_sec=54913.3
2024/04/14 17:52:05 INFO throughput result label=tcp threads=32 requests_per_sec=215026.6
2024/04/14 17:52:15 INFO throughput result label="grpc 8 channels" threads=32 requests_per_sec=78783.2
2024/04/14 17:52:25 INFO throughput result label=tcp threads=64 requests_per_sec=235585.2
2024/04/14 17:52:35 INFO throughput result label="grpc 8 channels" threads=64 requests_per_sec=104680.1
2024/04/14 17:52:45 INFO throughput result label=tcp threads=128 requests_per_sec=369443
2024/04/14 17:52:55 INFO throughput result label="grpc 8 channels" threads=128 requests_per_sec=126664.2
2024/04/14 17:53:05 INFO throughput result label=tcp threads=256 requests_per_sec=598797.1
2024/04/14 17:53:15 INFO throughput result label="grpc 8 channels" threads=256 requests_per_sec=152847.2
2024/04/14 17:53:25 INFO throughput result label=tcp threads=512 requests_per_sec=865977.2
2024/04/14 17:53:35 INFO throughput result label="grpc 8 channels" threads=512 requests_per_sec=185576.2
2024/04/14 17:53:45 INFO throughput result label=tcp threads=1024 requests_per_sec=1.075643e+06
2024/04/14 17:53:55 INFO throughput result label="grpc 8 channels" threads=1024 requests_per_sec=233233.1
2024/04/14 17:54:06 INFO throughput result label=tcp threads=2048 requests_per_sec=1.1432359e+06
2024/04/14 17:54:16 INFO throughput result label="grpc 8 channels" threads=2048 requests_per_sec=281601.8
2024/04/14 17:54:26 INFO throughput result label=tcp threads=4096 requests_per_sec=1.0944455e+06
2024/04/14 17:54:36 INFO throughput result label="grpc 8 channels" threads=4096 requests_per_sec=293408
```

### Rust raw results

```
for i in 1 2 4 8 16 32 64 128 256 512 1024 2048 4096; do
    ./gnetpingbench --remoteAddr=10.128.0.47 --tcpPort=8003 --grpcPort=8005 --throughputThreads=$i || break;
done

2024/04/14 17:55:57 INFO throughput result label=tcp threads=1 requests_per_sec=19982.8
2024/04/14 17:56:07 INFO throughput result label="grpc 1 channels" threads=1 requests_per_sec=10062
2024/04/14 17:56:17 INFO throughput result label=tcp threads=2 requests_per_sec=39273.1
2024/04/14 17:56:27 INFO throughput result label="grpc 1 channels" threads=2 requests_per_sec=18441.1
2024/04/14 17:56:37 INFO throughput result label=tcp threads=4 requests_per_sec=76308
2024/04/14 17:56:47 INFO throughput result label="grpc 1 channels" threads=4 requests_per_sec=30119
2024/04/14 17:56:57 INFO throughput result label=tcp threads=8 requests_per_sec=134181.2
2024/04/14 17:57:07 INFO throughput result label="grpc 1 channels" threads=8 requests_per_sec=44577.7
2024/04/14 17:57:17 INFO throughput result label=tcp threads=16 requests_per_sec=208260.7
2024/04/14 17:57:27 INFO throughput result label="grpc 1 channels" threads=16 requests_per_sec=49931.1
2024/04/14 17:57:37 INFO throughput result label=tcp threads=32 requests_per_sec=220663.1
2024/04/14 17:57:47 INFO throughput result label="grpc 1 channels" threads=32 requests_per_sec=53277.6
2024/04/14 17:57:57 INFO throughput result label=tcp threads=64 requests_per_sec=259949.2
2024/04/14 17:58:07 INFO throughput result label="grpc 1 channels" threads=64 requests_per_sec=58128.5
2024/04/14 17:58:17 INFO throughput result label=tcp threads=128 requests_per_sec=457983
2024/04/14 17:58:27 INFO throughput result label="grpc 1 channels" threads=128 requests_per_sec=66741.7
2024/04/14 17:58:37 INFO throughput result label=tcp threads=256 requests_per_sec=682304.5
2024/04/14 17:58:47 INFO throughput result label="grpc 1 channels" threads=256 requests_per_sec=71255.1
2024/04/14 17:58:57 INFO throughput result label=tcp threads=512 requests_per_sec=901469.4
2024/04/14 17:59:07 INFO throughput result label="grpc 1 channels" threads=512 requests_per_sec=76416.2
2024/04/14 17:59:17 INFO throughput result label=tcp threads=1024 requests_per_sec=1.0714881e+06
2024/04/14 17:59:27 INFO throughput result label="grpc 1 channels" threads=1024 requests_per_sec=84882.7
2024/04/14 17:59:38 INFO throughput result label=tcp threads=2048 requests_per_sec=1.0564692e+06
2024/04/14 17:59:48 INFO throughput result label="grpc 1 channels" threads=2048 requests_per_sec=93904.5
2024/04/14 17:59:58 INFO throughput result label=tcp threads=4096 requests_per_sec=891568.6
2024/04/14 18:00:08 INFO throughput result label="grpc 1 channels" threads=4096 requests_per_sec=98463.8

for i in 1 2 4 8 16 32 64 128 256 512 1024 2048 4096; do
    ./gnetpingbench --remoteAddr=10.128.0.47 --tcpPort=8003 --grpcPort=8005 --throughputThreads=$i --grpcChannels=8 || break;
done

2024/04/14 18:02:12 INFO throughput result label=tcp threads=1 requests_per_sec=19691.2
2024/04/14 18:02:22 INFO throughput result label="grpc 8 channels" threads=1 requests_per_sec=9952.5
2024/04/14 18:02:32 INFO throughput result label=tcp threads=2 requests_per_sec=39732.1
2024/04/14 18:02:42 INFO throughput result label="grpc 8 channels" threads=2 requests_per_sec=17145.4
2024/04/14 18:02:52 INFO throughput result label=tcp threads=4 requests_per_sec=75322.5
2024/04/14 18:03:02 INFO throughput result label="grpc 8 channels" threads=4 requests_per_sec=31416.9
2024/04/14 18:03:12 INFO throughput result label=tcp threads=8 requests_per_sec=134834
2024/04/14 18:03:22 INFO throughput result label="grpc 8 channels" threads=8 requests_per_sec=41940.7
2024/04/14 18:03:32 INFO throughput result label=tcp threads=16 requests_per_sec=209589.5
2024/04/14 18:03:42 INFO throughput result label="grpc 8 channels" threads=16 requests_per_sec=49542
2024/04/14 18:03:52 INFO throughput result label=tcp threads=32 requests_per_sec=217500.4
2024/04/14 18:04:02 INFO throughput result label="grpc 8 channels" threads=32 requests_per_sec=73816
2024/04/14 18:04:12 INFO throughput result label=tcp threads=64 requests_per_sec=260424.5
2024/04/14 18:04:22 INFO throughput result label="grpc 8 channels" threads=64 requests_per_sec=106211.6
2024/04/14 18:04:32 INFO throughput result label=tcp threads=128 requests_per_sec=456189.2
2024/04/14 18:04:42 INFO throughput result label="grpc 8 channels" threads=128 requests_per_sec=126605.4
2024/04/14 18:04:52 INFO throughput result label=tcp threads=256 requests_per_sec=693924.3
2024/04/14 18:05:02 INFO throughput result label="grpc 8 channels" threads=256 requests_per_sec=146060
2024/04/14 18:05:12 INFO throughput result label=tcp threads=512 requests_per_sec=901033.6
2024/04/14 18:05:22 INFO throughput result label="grpc 8 channels" threads=512 requests_per_sec=175518
2024/04/14 18:05:32 INFO throughput result label=tcp threads=1024 requests_per_sec=1.0725903e+06
2024/04/14 18:05:42 INFO throughput result label="grpc 8 channels" threads=1024 requests_per_sec=209806.8
2024/04/14 18:05:53 INFO throughput result label=tcp threads=2048 requests_per_sec=1.062728e+06
2024/04/14 18:06:03 INFO throughput result label="grpc 8 channels" threads=2048 requests_per_sec=250168.1
2024/04/14 18:06:13 INFO throughput result label=tcp threads=4096 requests_per_sec=884765.9
2024/04/14 18:06:23 INFO throughput result label="grpc 8 channels" threads=4096 requests_per_sec=265547.6


tokio tcp

for i in 1 2 4 8 16 32 64 128 256 512 1024 2048 4096; do
    ./gnetpingbench --remoteAddr=10.128.0.47 --tcpPort=8004 --grpcPort=8005 --throughputThreads=$i || break;
done

2024/04/14 18:43:12 INFO throughput result label=tcp threads=1 requests_per_sec=19985.3
2024/04/14 18:43:32 INFO throughput result label=tcp threads=2 requests_per_sec=35898.8
2024/04/14 18:43:52 INFO throughput result label=tcp threads=4 requests_per_sec=72056.2
2024/04/14 18:44:13 INFO throughput result label=tcp threads=8 requests_per_sec=122628.1
2024/04/14 18:44:33 INFO throughput result label=tcp threads=16 requests_per_sec=195200.2
2024/04/14 18:44:53 INFO throughput result label=tcp threads=32 requests_per_sec=220461.4
2024/04/14 18:45:13 INFO throughput result label=tcp threads=64 requests_per_sec=249885.5
2024/04/14 18:45:33 INFO throughput result label=tcp threads=128 requests_per_sec=407273.4
2024/04/14 18:45:53 INFO throughput result label=tcp threads=256 requests_per_sec=571039.8
2024/04/14 18:46:13 INFO throughput result label=tcp threads=512 requests_per_sec=786409.1
2024/04/14 18:46:33 INFO throughput result label=tcp threads=1024 requests_per_sec=1.0337303e+06
2024/04/14 18:46:53 INFO throughput result label=tcp threads=2048 requests_per_sec=1.236689e+06
2024/04/14 18:47:14 INFO throughput result label=tcp threads=4096 requests_per_sec=1.2192193e+06
```

### iperf raw results

Using iperf: 1 TCP connection gets close to the full 20 Gbps bandwidth (iperf reports 18.8 Gbits/sec). Using 2 or more TCP connections reliably hits the bandwidth maximum. This is different than (my previous bencmarks where I had to use 6 TCP connections to get the full bandwidth)[https://www.evanjones.ca/read-write-buffer-size.html]. With 4 TCP connections, the server uses about ~25% of the 4 VCPUs: 1% user space, 11% system, 13% software interrupts.


c3d-highcpu-16: 16 vCPUs (8 cores); 32 GiB RAM; network bandwitdth: up to 20Gbps = 
6.5.0-1017-gcp Ubuntu 22.04.4 LTS

https://cloud.google.com/compute/docs/general-purpose-machines#c3d-highcpu

https://cloud.google.com/compute/docs/network-bandwidth "Per-VM maximum egress bandwidth is generally 2 Gbps per vCPU" 


## Related links

* [gRPC's benchmarking guide](https://grpc.io/docs/guides/benchmarking/) links to a [dashboard with automated test results](https://grafana-dot-grpc-testing.appspot.com/). As of 2024-04-13 for gRPC Go, they show an 8 core server getting 178k requests/sec and a 32 core server getting ~420k requests/sec.
