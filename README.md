# dnscrypt-cake

[CAKE (Common Applications Kept Enhanced)](https://www.bufferbloat.net/projects/codel/wiki/CakeTechnical/) is a comprehensive smart queue management that is available as a queue discipline (qdisc) for the Linux kernel. It is one of the best qdiscs designed to solve bufferbloat problems at the network edge.

According to the CAKE's [ROUND TRIP TIME PARAMETERS](https://man7.org/linux/man-pages/man8/tc-cake.8.html) man7 page, if there is a way to adjust the RTT dynamically in real-time, it should theoretically make CAKE able to give the best possible AQM results between latency and throughput.

`dnscrypt-cake` is an attempt to adjust CAKE's `rtt` parameter in real-time based on real latency per DNS request using a slightly modified version of [dnscrypt-proxy 2](https://github.com/DNSCrypt/dnscrypt-proxy).

* * *

## How to compile the code

1. Download and install [The Go Programming Language](https://go.dev/).
2. Copy the files from `./dnscrypt-cake/cake-support` to `./dnscrypt-cake/dnscrypt/dnscrypt-proxy`.
3. Edit the `plugin_query_log.go` file and adjust these values:
   1. `uplinkInterface` and `downlinkInterface` to your network interface names.
   2. `maxDL` and `maxUL` to your maximum network bandwidth.
   3. If you prefer bandwidth in Mbps, then you need to adjust the `cakeUplink` and `cakeDownlink` bandwidth parameters too.


4. Then, simply compile the code with the following commands:

```yaml
$ cd ./dnscrypt-cake/dnscrypt/dnscrypt-proxy
$ go mod tidy
$ go build
```

* * *

# Credits

Although we are writing this guide to let people know about our implementation, it was made possible by using other things provided by the developers and/or companies mentioned in this guide.

All credits and copyrights go to the respective owners.