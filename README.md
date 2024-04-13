# netpingbench

An attempt to compare "out of the box" performance for small network request/response using TCP and gRPC in Go and Rust. The goal is to get some rough understanding of what performance we should expect on modern systems in the cloud.

## Benchmark results Google C3D AMD 

CPU: AMD EPYC Genoa 4th Generation EPYC 9B14
https://cloud.google.com/compute/docs/cpu-platforms



## GCP Network Performance

Using iperf: 1 TCP connection gets close to the full 20 Gbps bandwidth (iperf reports 18.8 Gbits/sec). Using 2 or more TCP connections reliably hits the bandwidth maximum. This is different than (my previous bencmarks where I had to use 6 TCP connections to get the full bandwidth)[https://www.evanjones.ca/read-write-buffer-size.html]. With 4 TCP connections, the server uses about ~25% of the 4 VCPUs: 1% user space, 11% system, 13% software interrupts.


c3d-highcpu-16: 16 vCPUs (8 cores); 32 GiB RAM; network bandwitdth: up to 20Gbps = 
6.5.0-1017-gcp Ubuntu 22.04.4 LTS

https://cloud.google.com/compute/docs/general-purpose-machines#c3d-highcpu

https://cloud.google.com/compute/docs/network-bandwidth "Per-VM maximum egress bandwidth is generally 2 Gbps per vCPU" 


iperf -s

### iperf output

iperf -t 30 -c 10.128.0.47 -P 4



## Related links

* [gRPC's benchmarking guide](https://grpc.io/docs/guides/benchmarking/) links to a [dashboard with automated test results](https://grafana-dot-grpc-testing.appspot.com/). As of 2024-04-13 for gRPC Go, they show an 8 core server getting 178k requests/sec and a 32 core server getting ~420k requests/sec.

for i in 1 2 4 6 8 16 32; do ./gnetpingbench --remoteAddr=10.128.0.47 --throughputThreads=$i || break; done

sudo apt update && sudo apt upgrade && sudo apt install screen iperf


gcloud compute ssh --zone "us-central1-a" "instance-1" --project "networkping"

gcloud compute ssh --zone "us-central1-a" "instance-1" --project "networkping"


gcloud compute instances create instance-1 \
    --project=networkping \
    --zone=us-central1-a \
    --machine-type=c3d-highcpu-16 \
    --network-interface=network-tier=PREMIUM,nic-type=GVNIC,stack-type=IPV4_ONLY,subnet=default \
    --metadata=enable-oslogin=true \
    --no-restart-on-failure \
    --maintenance-policy=TERMINATE \
    --provisioning-model=SPOT \
    --instance-termination-action=STOP \
    --no-service-account \
    --no-scopes \
    --create-disk=auto-delete=yes,boot=yes,device-name=instance-1,image=projects/ubuntu-os-cloud/global/images/ubuntu-minimal-2204-jammy-v20240318,mode=rw,size=10,type=projects/networkping/zones/us-central1-a/diskTypes/pd-balanced \
    --no-shielded-secure-boot \
    --shielded-vtpm \
    --shielded-integrity-monitoring \
    --labels=goog-ec-src=vm_add-gcloud \
    --reservation-affinity=any



gcloud compute instances create instance-2 \
    --project=networkping \
    --zone=us-central1-a \
    --machine-type=c3d-highcpu-16 \
    --network-interface=network-tier=PREMIUM,nic-type=GVNIC,stack-type=IPV4_ONLY,subnet=default \
    --metadata=enable-oslogin=true \
    --no-restart-on-failure \
    --maintenance-policy=TERMINATE \
    --provisioning-model=SPOT \
    --instance-termination-action=STOP \
    --no-service-account \
    --no-scopes \
    --create-disk=auto-delete=yes,boot=yes,device-name=instance-2,image=projects/ubuntu-os-cloud/global/images/ubuntu-minimal-2204-jammy-v20240318,mode=rw,size=10,type=projects/networkping/zones/us-central1-a/diskTypes/pd-balanced \
    --no-shielded-secure-boot \
    --shielded-vtpm \
    --shielded-integrity-monitoring \
    --labels=goog-ec-src=vm_add-gcloud \
    --reservation-affinity=any