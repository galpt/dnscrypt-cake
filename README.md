# dnscrypt-cake

> :information_source: Note that:
>
> The goal of this project is to provide another alternative that *"just works"* for not-so-technical users. Thus, users only need to set these values correctly: `uplinkInterface`, `downlinkInterface`, `maxDL`, and `maxUL`.

[CAKE (Common Applications Kept Enhanced)](https://www.bufferbloat.net/projects/codel/wiki/CakeTechnical/) is a comprehensive smart queue management that is available as a queue discipline (qdisc) for the Linux kernel. It is one of the best qdiscs designed to solve bufferbloat problems at the network edge.

According to the CAKE's [ROUND TRIP TIME PARAMETERS](https://man7.org/linux/man-pages/man8/tc-cake.8.html) man7 page, if there is a way to adjust the RTT dynamically in real-time, it should theoretically make CAKE able to give the best possible AQM results between latency and throughput.

`dnscrypt-cake` is an attempt to adjust CAKE's `rtt` parameter in real-time based on real latency per DNS request using a slightly modified version of [dnscrypt-proxy 2](https://github.com/DNSCrypt/dnscrypt-proxy). In addition to that, it will also adjust `bandwidth` intelligently while constantly monitoring your real RTT.

Cloudflare said that [almost everything on the Internet starts with a DNS request](https://developers.cloudflare.com/1.1.1.1/privacy/public-dns-resolver/#:~:text=Nearly%20everything%20on%20the%20Internet%20starts%20with%20a%20DNS%20request), so that's why we made this implementation.

Some of the possible ways we can measure the user's real RTT are by using the following methods:
1. a transparent proxy (some kind of HTTP CONNECT proxy or something like that).
2. a DNS proxy server (this is probably the easiest way these days).

We think that adjusting CAKE's `rtt` and `bandwidth` using a DNS server running on the user's machine is a good way to improve the user's network performance as early as possible before the other Internet assets can be loaded.

This is an adaptation of the [cake-autorate](https://github.com/lynxthecat/cake-autorate) project implemented in Go, but it's adjusting CAKE's `rtt` and `bandwidth` based on your every DNS request and what website you are visiting, not by only ping-ing to `1.1.1.1`, `8.8.8.8` and/or any other DNS servers.

This implementation is suitable for servers and networks where most of the users are actively sending DNS requests.

* * *

## What to expect

There are several things you can expect from using this implementation:
1. You only need to worry about setting up `uplinkInterface`, `downlinkInterface`, `maxDL`, and `maxUL` correctly.
2. It will manage `bandwidth` intelligently (do a speedtest via `speedtest-cli` or similar tools to see it in action).
3. It will manage `rtt` ranging from 100ms - 1000ms.

> :information_source: Note
>
> Just set `maxDL` and `maxUL` based on whatever speed advertised by your ISP. No need to limit them to 90% or something like that. The code logic will try to handle that automatically.

* * *

## How it works

![Workflow](https://github.com/galpt/dnscrypt-cake/blob/main/img/dnscrypt-cake.jpg)

1. When a latency increase is detected, `dnscrypt-cake` will try to check if the DNS latency is in the range of 100ms - 1000ms or not.
If yes, then use that as CAKE's `rtt`, if not then use `rtt 100ms` if it's less than 100ms, and `rtt 1000ms` if it's more than 1000ms.
2. `dnscrypt-cake` will then reduce CAKE's `bandwidth` to 5% of the specified `maxDL` and `maxUL`.
3. The `cakeBwIncrease()` function will try to increase bandwidth over time, but it can be slow in some situations when the DNS latency varies a lot.
4. The `cakeBwRecovery()` function will help CAKE recover bandwidth faster to maintain high throughput while trying to get latency under control.
5. The `cakeBwNormalize()` function will check CAKE's `bandwidth` every 3 seconds. If CAKE failed to recover bandwidth to the specified `maxDL` and `maxUL` during that period, this function will normalize bandwidth to the specified `maxDL` and `maxUL`.
6. The `cakeBwMax()` function will check in real-time to make sure bandwidth stays at 90% from the specified `maxDL` and `maxUL`.
7. The `cakeResetRTT()` function will check the RTT every 2 seconds and reset CAKE's `rtt` to either 100ms or 300ms.

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
