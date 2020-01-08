`sflow-patcher` is a lightweight sFlow proxy that strips VXLAN headers from the sFlow raw packet records, keeping all other records and counters intact. All UDP packets that `sflow-patcher` failed to parse as sFlow datagrams are relayed as is.

The VXLAN header detection is dead-simple: `sflow-patcher` goes through the raw packet record headers and checks if the packet happens to be a UDP packet with destination port 4789.

# Building

Install Go 1.13, run `make`, and grab the binary from `out` directory in the repo.

# Usage

```
  sflow-patcher <upstream address:port> [flags]

Flags:
  -b, --bind string       address and port to bind on (default "0.0.0.0:5000")
  -s, --buffer-size int   input buffer size in bytes (default 1500)
  -d, --debug             enable debug logging
  -h, --help              help for sflow-patcher
  -w, --workers int       number of workers (default 10)
```

Input buffer shoud fit any received sFlow datagram. Workers number indicates how many packets will be processed in parallel (could be increased in case of packet drops).


## Example

`sflow-patcher -b 0.0.0.0:16789 -w 20 172.16.150.40:6000` will listen on UDP port 16789 and relay the processed packets to host 172.16.150.40, UDP port 6000.