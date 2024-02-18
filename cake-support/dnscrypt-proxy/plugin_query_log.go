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
	maxUL = 4000000
	maxDL = 4000000
	// ------

	// do not touch these.
	// these are in nanoseconds.
	// 1 ms = 1000000 ns.
	// 1 ms = 1000 us.
	metroRTT     time.Duration = 10000000
	regionalRTT  time.Duration = 30000000
	internetRTT  time.Duration = 100000000
	oceanicRTT   time.Duration = 300000000
	satelliteRTT time.Duration = 1000000000
	// ------
	Mbit = 1000    // 1 Mbit
	Gbit = 1000000 // 1 Gbit
)

// do not touch these.
// should be maintained by the functions automatically.
var (
	bwUL   = 2
	bwDL   = 2
	bwUL1  = 1
	bwDL1  = 1
	bwUL90 = 90
	bwDL90 = 90

	// default to 100ms rtt.
	// in Go, "time.Duration" defaults to nanoseconds.
	newRTT   time.Duration = 100000000 // this is in nanoseconds
	oldRTT   time.Duration = 100000000 // this is in nanoseconds
	newRTTus time.Duration = 100000    // this will be in microseconds

	// decide whether split-gso should be used or not.
	autoSplitGSO = "split-gso"

	// bufferbloat state
	bufferbloatState      = false
	bufferbloatStateCount = 0
)

// functions for adjusting cake
func cake() {

	// calculate bandwidth percentage
	bwUL1 = ((maxUL * 1) / 100)
	bwDL1 = ((maxDL * 1) / 100)
	bwUL90 = ((maxUL * 90) / 100)
	bwDL90 = ((maxDL * 90) / 100)

	// infinite loop to change cake parameters in real-time
	for {

		time.Sleep(100 * time.Millisecond)

		// handle bufferbloat state
		if bufferbloatState {

			// a check for connections slower than 1 Mbit/s
			// (i.e. data cellular ISPs with Fair Usage Policy).
			if bwUL1 < Mbit || bwDL1 < Mbit {
				bwUL = bwUL1
				bwDL = bwDL1
			} else if bwUL1 > Mbit || bwDL1 > Mbit {
				bwUL = Mbit
				bwDL = Mbit
			}

			bufferbloatStateCount++
			if bufferbloatStateCount >= 5 {
				bufferbloatStateCount = 0
				bufferbloatState = false
			}
		} else if !bufferbloatState {
			// fast recovery uplink & downlink
			bwUL = maxUL
			bwDL = maxDL
		}

		// automatically limit max bandwidth to 90%
		if bwUL > bwUL90 {
			bwUL = bwUL90
		}
		if bwDL > bwDL90 {
			bwDL = bwDL90
		}

		// use autoSplitGSO
		if bwUL < Gbit || bwDL < Gbit {
			autoSplitGSO = "split-gso"
		} else if bwUL > Gbit || bwDL > Gbit {
			autoSplitGSO = "no-split-gso"
		}

		// set uplink
		cakeUplink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", uplinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTTus), "bandwidth", fmt.Sprintf("%dkbit", bwUL), fmt.Sprintf("%v", autoSplitGSO))
		output, err := cakeUplink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return
		}
		// set downlink
		cakeDownlink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", downlinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTTus), "bandwidth", fmt.Sprintf("%dkbit", bwDL), fmt.Sprintf("%v", autoSplitGSO))
		output, err = cakeDownlink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return
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

		// check if the real RTT increases (unstable) or not.
		if newRTT > oldRTT {

			// a check for connections slower than 1 Mbit/s
			// (i.e. data cellular ISPs with Fair Usage Policy).
			if bwUL1 < Mbit || bwDL1 < Mbit {
				bwUL = bwUL1
				bwDL = bwDL1
			} else if bwUL1 > Mbit || bwDL1 > Mbit {
				bwUL = Mbit
				bwDL = Mbit
			}

			// use autoSplitGSO
			if bwUL < Gbit || bwDL < Gbit {
				autoSplitGSO = "split-gso"
			} else if bwUL > Gbit || bwDL > Gbit {
				autoSplitGSO = "no-split-gso"
			}

			bufferbloatState = true

		}

		// normalize RTT
		if newRTT < internetRTT {
			newRTT = internetRTT
		}
		if newRTT > satelliteRTT {
			newRTT = satelliteRTT
		}

		// convert to microseconds
		newRTTus = newRTT / time.Microsecond

		// update cake settings based on real world data.
		// ------
		// set uplink
		cakeUplink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", uplinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTTus), "bandwidth", fmt.Sprintf("%dkbit", bwUL), fmt.Sprintf("%v", autoSplitGSO))
		output, err := cakeUplink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return errors.New("Failed setting up cakeUplink")
		}
		// set downlink
		cakeDownlink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", downlinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTTus), "bandwidth", fmt.Sprintf("%dkbit", bwDL), fmt.Sprintf("%v", autoSplitGSO))
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
