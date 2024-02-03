# dnscrypt-cake

[CAKE (Common Applications Kept Enhanced)](https://www.bufferbloat.net/projects/codel/wiki/CakeTechnical/) is a comprehensive smart queue management that is available as a queue discipline (qdisc) for the Linux kernel. It is one of the best qdiscs designed to solve bufferbloat problems at the network edge.

According to the CAKE's [ROUND TRIP TIME PARAMETERS](https://man7.org/linux/man-pages/man8/tc-cake.8.html) man7 page, if there is a way to adjust the RTT dynamically in real-time, it should theoretically make CAKE able to give the best possible AQM results between latency and throughput.

`dnscrypt-cake` is an attempt to adjust CAKE's `rtt` parameter in real-time based on real latency per DNS request using a slightly modified version of [dnscrypt-proxy 2](https://github.com/DNSCrypt/dnscrypt-proxy). In addition to that, it will also adjust `bandwidth` based on the minimum value you think your network will be bufferbloat-free, and try to increase it continuously from there while constantly monitoring your real RTT.

This is an adaptation of the [cake-autorate](https://github.com/lynxthecat/cake-autorate) project implemented in Go, but this is potentially a better implementation since it's adjusting CAKE's `rtt` and `bandwidth` based on your every DNS request and what website you are visiting, not by only ping-ing to `1.1.1.1`, `8.8.8.8` and/or any other DNS servers.

This implementation is suitable for servers and networks where most of the users are actively sending DNS requests.

* * *

## How to compile the code

1. Download and install [The Go Programming Language](https://go.dev/).
2. Copy the files from `./dnscrypt-cake/cake-support` to `./dnscrypt-cake/dnscrypt/dnscrypt-proxy`.
3. Edit the `plugin_query_log.go` file and adjust these values:
   1. `uplinkInterface` and `downlinkInterface` to your network interface names.
   2. `minDL` and `minUL` to your minimum network bandwidth where you think there won't be any bufferbloat.
   3. `maxDL` and `maxUL` to your maximum network bandwidth advertised by your ISP (recommended to limit them to 90%).


4. Then, simply compile the code with the following commands:

```yaml
$ cd ./dnscrypt-cake/dnscrypt/dnscrypt-proxy
$ go mod tidy
$ go build
```

* * *

# See `dnscrypt-cake` in action

We are testing `dnscrypt-cake` in our server here:

https://net.0ms.dev:7777/netstat

This server is being used as our testing environment as well as a speedtest server for both Ookla and LibreSpeed.

### Ookla

![Ookla](https://github.com/galpt/dnscrypt-cake/blob/main/img/ookla.png)

### LibreSpeed

![LibreSpeed](https://github.com/galpt/dnscrypt-cake/blob/main/img/librespeed.png)

* * *

# Credits

Although we are writing this guide to let people know about our implementation, it was made possible by using other things provided by the developers and/or companies mentioned in this guide.

All credits and copyrights go to the respective owners.
