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

	// ------
	// you should adjust these values
	// ------
	// adjust them according to your network interface names
	uplinkInterface   = "enp3s0"
	downlinkInterface = "ifb4enp3s0"
	// ------
	// adjust "maxUL" and "maxDL" based on the maximum speed
	// advertised by your ISP (in Kilobit/s format).
	// 1 Mbit = 1000 kbit.
	maxUL = 3000000
	maxDL = 3000000
	// ------

	// do not touch these.
	// these are in nanoseconds.
	// 1 ms = 1000000 ns.
	metroRTT     time.Duration = 10000000
	internetRTT  time.Duration = 100000000
	oceanicRTT   time.Duration = 300000000
	satelliteRTT time.Duration = 1000000000
)

// do not touch these.
// should be maintained by the functions automatically.
var (

	// values are in mbit
	bwUL   = 2
	bwDL   = 2
	bwUL5  = 5
	bwDL5  = 5
	bwUL10 = 10
	bwDL10 = 10
	bwUL30 = 30
	bwDL30 = 30
	bwUL50 = 50
	bwDL50 = 50
	bwUL70 = 70
	bwDL70 = 70
	bwUL90 = 90
	bwDL90 = 90

	// default to 100ms rtt
	newRTT time.Duration = 100000000 // this is in nanoseconds
	oldRTT time.Duration = 100000000 // this is in nanoseconds
)

// functions for adjusting cake
func cakeBwIncrease() {

	// infinite loop to change cake parameters in real-time
	for {

		// set uplink
		cakeUplink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", uplinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTT), "bandwidth", fmt.Sprintf("%dkbit", bwUL))
		output, err := cakeUplink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return
		}
		// set downlink
		cakeDownlink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", downlinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTT), "bandwidth", fmt.Sprintf("%dkbit", bwDL))
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

func cakeBwRecovery() {

	// calculate bandwidth percentage
	bwUL5 = ((maxUL * 5) / 100)
	bwDL5 = ((maxDL * 5) / 100)
	bwUL10 = ((maxUL * 10) / 100)
	bwDL10 = ((maxDL * 10) / 100)
	bwUL30 = ((maxUL * 30) / 100)
	bwDL30 = ((maxDL * 30) / 100)
	bwUL50 = ((maxUL * 50) / 100)
	bwDL50 = ((maxDL * 50) / 100)
	bwUL70 = ((maxUL * 70) / 100)
	bwDL70 = ((maxDL * 70) / 100)
	bwUL90 = ((maxUL * 90) / 100)
	bwDL90 = ((maxDL * 90) / 100)

	for {

		// check bandwidthh every second
		// ------
		// fast recovery uplink & downlink
		time.Sleep(1 * time.Second)
		if bwUL < bwUL10 {
			bwUL = bwUL10
		} else if bwUL > bwUL10 && bwUL < bwUL30 {
			bwUL = bwUL30
		} else if bwUL > bwUL30 && bwUL < bwUL50 {
			bwUL = bwUL50
		} else if bwUL > bwUL50 && bwUL < bwUL70 {
			bwUL = bwUL70
		} else if bwUL > bwUL70 && bwUL < bwUL90 {
			bwUL = bwUL90
		}

		if bwDL < bwDL10 {
			bwDL = bwDL10
		} else if bwDL > bwDL10 && bwDL < bwDL30 {
			bwDL = bwDL30
		} else if bwDL > bwDL30 && bwDL < bwDL50 {
			bwDL = bwDL50
		} else if bwDL > bwDL50 && bwDL < bwDL70 {
			bwDL = bwDL70
		} else if bwDL > bwDL70 && bwDL < bwDL90 {
			bwDL = bwDL90
		}

	}
}

func cakeBwNormalize() {

	for {

		// in some situations, when DNS latency varies a lot,
		// it's possible for the bandwidth logic to fail to recover
		// the bandwidth to the specified maxDL/maxUL.
		// because of that, we want to normalize cake's bandwidth
		// to maxDL/maxUL if it takes longer than 4 seconds to recover.
		// it should work well with cakeBwRecovery()
		time.Sleep(4 * time.Second)
		if bwUL < bwUL90 {
			bwUL = bwUL90
		}
		if bwDL < bwDL90 {
			bwDL = bwDL90
		}

	}
}

func cakeBwMax() {

	for {

		// automatically limit max bandwidth to 90%
		if bwUL > bwUL90 {
			bwUL = bwUL90
		}
		if bwDL > bwDL90 {
			bwDL = bwDL90
		}

	}
}

func cakeResetRTT() {

	for {

		// in some situations, when DNS latency varies a lot,
		// it's possible that RTT varies a lot too.
		// this function will reset cake's rtt
		// back to either "internetRTT" or "oceanicRTT"
		// every 5 seconds.
		time.Sleep(5 * time.Second)
		if newRTT < internetRTT {
			newRTT = internetRTT
		}
		if newRTT > oceanicRTT {
			newRTT = oceanicRTT
		}

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

		// normalize RTT
		if newRTT < metroRTT {
			newRTT = metroRTT
		}
		if newRTT > satelliteRTT {
			newRTT = satelliteRTT
		}

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

			// reduce current bandwidth to 5%
			bwUL = bwUL5
			bwDL = bwDL5
		}

		// update cake settings based on real world data.
		// ------
		// set uplink
		cakeUplink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", uplinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTT), "bandwidth", fmt.Sprintf("%dkbit", bwUL))
		output, err := cakeUplink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return errors.New("Failed setting up cakeUplink")
		}
		// set downlink
		cakeDownlink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", downlinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTT), "bandwidth", fmt.Sprintf("%dkbit", bwDL))
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
