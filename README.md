# dnscrypt-cake

> :information_source: Note that:
>
> The goal of this project is to provide another alternative that *"just works"* for not-so-technical users. Thus, users only need to set these values correctly: `uplinkInterface`, `downlinkInterface`, `maxDL`, and `maxUL`.

[CAKE (Common Applications Kept Enhanced)](https://www.bufferbloat.net/projects/codel/wiki/CakeTechnical/) is a comprehensive smart queue management that is available as a queue discipline (qdisc) for the Linux kernel. It is one of the best qdiscs designed to solve bufferbloat problems at the network edge.

According to the CAKE's [ROUND TRIP TIME PARAMETERS](https://man7.org/linux/man-pages/man8/tc-cake.8.html) man7 page, if there is a way to adjust the RTT dynamically in real-time, it should theoretically make CAKE able to give the best possible AQM results between latency and throughput.

`dnscrypt-cake` is an attempt to adjust CAKE's `rtt` parameter in real-time based on real latency per DNS request using a slightly modified version of [dnscrypt-proxy 2](https://github.com/DNSCrypt/dnscrypt-proxy). In addition to that, it will also adjust `bandwidth` intelligently while constantly monitoring your real RTT.

This is an adaptation of the [cake-autorate](https://github.com/lynxthecat/cake-autorate) project implemented in Go, but it's adjusting CAKE's `rtt` and `bandwidth` based on your every DNS request and what website you are visiting, not by only ping-ing to `1.1.1.1`, `8.8.8.8` and/or any other DNS servers.

This implementation is suitable for servers and networks where most of the users are actively sending DNS requests.

* * *

## What to expect

There are several things you can expect from using this implementation:
1. You only need to worry about setting up `uplinkInterface`, `downlinkInterface`, `maxDL`, and `maxUL` correctly.
2. It will manage `bandwidth` intelligently (do a speedtest using [Speedtest CLI](https://www.speedtest.net/apps/cli) or similar tools to see it in action).
3. It will manage `rtt` ranging from 30ms - 1000ms.
4. It will manage `split-gso` automatically.
5. Designed to be scalable. It is able to manage CAKE's `bandwidth` from 1 Mbit/s to 1 Gbit/s (or even more) in seconds.

> :information_source: Note
>
> Just set `maxDL` and `maxUL` based on whatever speed advertised by your ISP. No need to limit them to 90% or something like that. The code logic will try to handle that automatically.

* * *

## How it works

![Workflow](https://github.com/galpt/dnscrypt-cake/blob/main/img/dnscrypt-cake.jpg)

1. When a latency increase is detected, `dnscrypt-cake` will try to check if the DNS latency is in the range of 30ms - 1000ms or not.
If yes, then use that as CAKE's `rtt`, if not then use `rtt 30ms` if it's less than 100ms, and `rtt 1000ms` if it's more than 1000ms.
2. `dnscrypt-cake` will then reduce CAKE's `bandwidth` by 50%.
3. The `cake()` function will try to handle `bandwidth` and `rtt` automatically over time.

* * *

## How to compile the code

1. Download and install [The Go Programming Language](https://go.dev/).
2. Copy the files from `./dnscrypt-cake/cake-support` to `./dnscrypt-cake/dnscrypt/dnscrypt-proxy`.
3. Edit the `plugin_query_log.go` file and adjust these values:
   1. `uplinkInterface` and `downlinkInterface` to your network interface names.
   3. `maxDL` and `maxUL` to your maximum network bandwidth (in Kilobit/s format) advertised by your ISP.


4. Then, simply compile the code with the following commands:

```yaml
$ cd ./dnscrypt-cake/dnscrypt/dnscrypt-proxy
$ go mod tidy
$ go build
```

> :information_source: Note that:
> 1. You have to run the binary with `sudo` since it needs to change the linux qdisc, so it needs enough permissions to do that.
> 2. It's not recommended to change `cakeUplink` and `cakeDownlink` parameters in the `plugin_query_log.go` file as they are intended to only handle `bandwidth` and `rtt`. If you need to change CAKE's parameters, change them directly from the terminal.

* * *

## See `dnscrypt-cake` in action

We are testing `dnscrypt-cake` in our server here:

https://net.0ms.dev:7777/netstat

This server is being used as our testing environment as well as a speedtest server for both Ookla and LibreSpeed.

### Ookla

![Ookla](https://github.com/galpt/dnscrypt-cake/blob/main/img/ookla.png)

### LibreSpeed

![LibreSpeed](https://github.com/galpt/dnscrypt-cake/blob/main/img/librespeed.png)

* * *

## Credits

Although we are writing this guide to let people know about our implementation, it was made possible by using other things provided by the developers and/or companies mentioned in this guide.

All credits and copyrights go to the respective owners.
