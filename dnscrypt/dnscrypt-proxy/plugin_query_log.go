package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/jedisct1/dlog"
	"github.com/miekg/dns"
)

const (
	// adjust "minUL" and "minDL" based on the minimum speed
	// where you're sure there won't be bufferbloats.
	// ------
	// adjust "maxUL" and "maxDL" based on the maximum speed
	// where you're sure there won't be bufferbloats.
	// usually they can be 90% of the total advertised speed by ISP.
	// ------
	// 2 mbit is small enough for cellular networks.
	// 1000 mbit for most servers.
	// values are in mbit.
	minUL = 1000
	minDL = 1000
	maxUL = 4000
	maxDL = 4000
)

var (
	// adjust them according to your network interface names
	uplinkInterface   = "enp3s0"
	downlinkInterface = "ifb4enp3s0"

	// try to adjust cake shaper automatically based on rtt.
	// values are in mbit.
	bwUL = 2
	bwDL = 2

	// do not touch.
	// default to 100ms rtt.
	newRTT time.Duration = 100000000 // this is in nanoseconds
	oldRTT time.Duration = 100000000 // this is in nanoseconds
)

// function for adjusting cake
func cake(minUL int, minDL int, newRTT time.Duration, oldRTT time.Duration) {

	// infinite loop to change cake parameters in real-time
	for {

		// convert to microseconds
		newRTT = newRTT / time.Microsecond
		oldRTT = oldRTT / time.Microsecond

		// check if the real RTT increases (unstable) or not.
		// if the "newRTT" does increase compared to the "oldRTT",
		// then reduce cake's bandwidth by n-percent.
		// after reducing the bandwidth, keep increasing the bandwidth
		// until it detects an RTT increase from the "newRTT" again,
		// then repeat the cycle from the start.
		if newRTT > oldRTT {

			// reduce current bandwidth by 10%
			bwUL = bwUL - ((bwUL * 10) / 100)
			bwDL = bwDL - ((bwDL * 10) / 100)

			// if the divided bandwidth is less than minUL/minDL,
			// set them to minUL & minDL instead.
			if bwUL < minUL {
				bwUL = minUL
			}
			if bwDL < minDL {
				bwDL = minDL
			}

			// if bwUL/bwDL is more than maxUL/maxDL,
			// set them to maxUL & maxDL instead.
			if bwUL > maxUL {
				bwUL = maxUL
			}
			if bwDL > maxDL {
				bwDL = maxDL
			}
		}

		// update cake settings based on real world data.
		// adjust the parameters other than RTT and Bandwidth according to your needs.
		// ------
		// set uplink
		cakeUplink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", uplinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTT), "bandwidth", fmt.Sprintf("%dmbit", bwUL))
		output, err := cakeUplink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return
		}
		// set downlink
		cakeDownlink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", downlinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTT), "bandwidth", fmt.Sprintf("%dmbit", bwDL))
		output, err = cakeDownlink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return
		}

		// keep increasing bandwidth until it detects an RTT increase again.
		bwUL++
		bwDL++

	}
}

type PluginQueryLog struct {
	logger        io.Writer
	format        string
	ignoredQtypes []string
}

func (plugin *PluginQueryLog) Name() string {
	return "query_log"
}

func (plugin *PluginQueryLog) Description() string {
	return "Log DNS queries."
}

func (plugin *PluginQueryLog) Init(proxy *Proxy) error {
	plugin.logger = Logger(proxy.logMaxSize, proxy.logMaxAge, proxy.logMaxBackups, proxy.queryLogFile)
	plugin.format = proxy.queryLogFormat
	plugin.ignoredQtypes = proxy.queryLogIgnoredQtypes

	return nil
}

func (plugin *PluginQueryLog) Drop() error {
	return nil
}

func (plugin *PluginQueryLog) Reload() error {
	return nil
}

func (plugin *PluginQueryLog) Eval(pluginsState *PluginsState, msg *dns.Msg) error {
	var clientIPStr string
	switch pluginsState.clientProto {
	case "udp":
		clientIPStr = (*pluginsState.clientAddr).(*net.UDPAddr).IP.String()
	case "tcp", "local_doh":
		clientIPStr = (*pluginsState.clientAddr).(*net.TCPAddr).IP.String()
	default:
		// Ignore internal flow.
		return nil
	}
	question := msg.Question[0]
	qType, ok := dns.TypeToString[question.Qtype]
	if !ok {
		qType = string(qType)
	}
	if len(plugin.ignoredQtypes) > 0 {
		for _, ignoredQtype := range plugin.ignoredQtypes {
			if strings.EqualFold(ignoredQtype, qType) {
				return nil
			}
		}
	}
	qName := pluginsState.qName

	if pluginsState.cacheHit {
		pluginsState.serverName = "-"
	} else {
		switch pluginsState.returnCode {
		case PluginsReturnCodeSynth, PluginsReturnCodeCloak, PluginsReturnCodeParseError:
			pluginsState.serverName = "-"
		}
	}
	returnCode, ok := PluginsReturnCodeToString[pluginsState.returnCode]
	if !ok {
		returnCode = string(returnCode)
	}

	var requestDuration time.Duration
	if !pluginsState.requestStart.IsZero() && !pluginsState.requestEnd.IsZero() {
		requestDuration = pluginsState.requestEnd.Sub(pluginsState.requestStart)

	}
	var line string
	if plugin.format == "tsv" {
		now := time.Now()
		year, month, day := now.Date()
		hour, minute, second := now.Clock()
		tsStr := fmt.Sprintf("[%d-%02d-%02d %02d:%02d:%02d]", year, int(month), day, hour, minute, second)
		line = fmt.Sprintf(
			"%s\t%s\t%s\t%s\t%s\t%dms\t%s\n",
			tsStr,
			clientIPStr,
			StringQuote(qName),
			qType,
			returnCode,
			requestDuration/time.Millisecond,
			StringQuote(pluginsState.serverName),
		)

		// save DNS latency as the new RTT for cake
		newRTT = requestDuration

		// convert to microseconds
		newRTT = newRTT / time.Microsecond
		oldRTT = oldRTT / time.Microsecond

		// check if the real RTT increases (unstable) or not.
		// if the "newRTT" does increase compared to the "oldRTT",
		// then reduce cake's bandwidth by n-percent.
		// after reducing the bandwidth, keep increasing the bandwidth
		// until it detects an RTT increase from the "newRTT" again,
		// then repeat the cycle from the start.
		if newRTT > oldRTT {

			// reduce current bandwidth by 10%
			bwUL = bwUL - ((bwUL * 10) / 100)
			bwDL = bwDL - ((bwDL * 10) / 100)

			// if the divided bandwidth is less than minUL/minDL,
			// set them to minUL & minDL instead.
			if bwUL < minUL {
				bwUL = minUL
			}
			if bwDL < minDL {
				bwDL = minDL
			}

			// if bwUL/bwDL is more than maxUL/maxDL,
			// set them to maxUL & maxDL instead.
			if bwUL > maxUL {
				bwUL = maxUL
			}
			if bwDL > maxDL {
				bwDL = maxDL
			}
		}

		// update cake settings based on real world data.
		// adjust the parameters other than RTT and Bandwidth according to your needs.
		// ------
		// set uplink
		cakeUplink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", uplinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTT), "bandwidth", fmt.Sprintf("%dmbit", bwUL))
		output, err := cakeUplink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return errors.New("Failed setting up cakeUplink")
		}
		// set downlink
		cakeDownlink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", downlinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTT), "bandwidth", fmt.Sprintf("%dmbit", bwDL))
		output, err = cakeDownlink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return errors.New("Failed setting up cakeDownlink")
		}

		// update oldRTT
		oldRTT = newRTT

	} else if plugin.format == "ltsv" {
		cached := 0
		if pluginsState.cacheHit {
			cached = 1
		}
		line = fmt.Sprintf("time:%d\thost:%s\tmessage:%s\ttype:%s\treturn:%s\tcached:%d\tduration:%d\tserver:%s\n",
			time.Now().Unix(), clientIPStr, StringQuote(qName), qType, returnCode, cached, requestDuration/time.Millisecond, StringQuote(pluginsState.serverName))
	} else {
		dlog.Fatalf("Unexpected log format: [%s]", plugin.format)
	}
	if plugin.logger == nil {
		return errors.New("Log file not initialized")
	}
	_, _ = plugin.logger.Write([]byte(line))

	return nil
}
