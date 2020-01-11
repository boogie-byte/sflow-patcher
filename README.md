`sflow-patcher` is a lightweight sFlow proxy that strips VXLAN headers from the sFlow raw packet records, keeping all other records and counters intact. All UDP packets that `sflow-patcher` failed to parse as sFlow datagrams are relayed as is. UDP source addresses and ports are preserved in order to provide compatibility with sFlow analyzers thar rely on source IP instead of agendId value.

The VXLAN header detection is dead-simple: `sflow-patcher` goes through the raw packet record headers and checks if the packet happens to be a UDP packet with destination port 4789.

# Building

Install Go >=1.13, run `make`, and grab the binary from `out` directory in the repo. `sflow-patcher` uses pcap for emitting raw frames, so make sure you have `libpcap` headers installed.

# Usage

```
Usage:
  sflow-patcher [flags]

Flags:
  -b, --bind string       address and port to bind on (default "0.0.0.0:5000")
  -s, --buffer-size int   input buffer size in bytes (default 1500)
  -d, --debug             enable debug logging
  -m, --dst-mac string    destination MAC address
  -h, --help              help for sflow-patcher
  -i, --out-if string     outgoing interface
  -u, --upstream string   upstream address:port
  -w, --workers int       number of workers (default 10)
```

`dst-mac`, `out-if` and `upstream` parameters are mandatory. `buffer-size` shoud fit any received sFlow datagram. `workers` indicates how many packets will be processed in parallel (could be increased in case of packet drops).


## Example

`sflow-patcher -b 0.0.0.0:16789 -i eth0 -m 00:11:22:33:44:55 -u 172.16.150.40:6000` will listen on UDP port 16789 and relay the processed packets to host 172.16.150.40, UDP port 6000 sending ethernet frames to MAC address 00:11:22:33:44:55 from interface eth0.