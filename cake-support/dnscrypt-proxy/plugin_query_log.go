package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/jedisct1/dlog"
	"github.com/miekg/dns"

	"github.com/gin-gonic/gin"
)

type (
	Cake struct {
		RTTAverage          time.Duration `json:"rttAverage"`
		RTTAverageString    string        `json:"rttAverageString"`
		BwUpAverage         float64       `json:"bwUpAverage"`
		BwUpAverageString   string        `json:"bwUpAverageString"`
		BwDownAverage       float64       `json:"bwDownAverage"`
		BwDownAverageString string        `json:"bwDownAverageString"`
		DataTotal           string        `json:"dataTotal"`
	}

	CakeData struct {
		RTT               time.Duration `json:"rtt"`
		BandwidthUpload   float64       `json:"bandwidthUpload"`
		BandwidthDownload float64       `json:"bandwidthDownload"`
	}
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
	// ------
	B float64 = 0.7
	C float64 = 0.4

	timeoutTr     = 30 * time.Second
	hostPortGin   = "22222"
	cakeDataLimit = 100000000 // 100 million
)

// do not touch these.
// should be maintained by the functions automatically.
var (
	bwUL   float64 = 2
	bwDL   float64 = 2
	bwUL90 float64 = 90
	bwDL90 float64 = 90

	lastBwUL                   float64 = 90
	lastBwDL                   float64 = 90
	lastBufferbloatTimestamp   time.Time
	lastBufferbloatTimeElapsed time.Duration = 10 // this will be in seconds
	kUL                        float64       = 1
	kDL                        float64       = 1

	// default to 100ms rtt.
	// in Go, "time.Duration" defaults to nanoseconds.
	newRTT   time.Duration = 100000000 // this is in nanoseconds
	oldRTT   time.Duration = 100000000 // this is in nanoseconds
	newRTTus time.Duration = 100000    // this will be in microseconds

	// decide whether split-gso should be used or not.
	autoSplitGSO = "split-gso"

	// bufferbloat state
	bufferbloatState = false

	cakeJSON     Cake
	cakeDataJSON []CakeData

	rttArr         []float64
	rttAvgTotal    float64       = 0
	rttAvgDuration time.Duration = 0

	bwUpArr        []float64
	bwUpAvgTotal   float64 = 0
	bwDownArr      []float64
	bwDownAvgTotal float64 = 0

	mem         runtime.MemStats
	HeapAlloc   string
	SysMem      string
	Frees       string
	NumGCMem    string
	timeElapsed string
	latestLog   string

	CertFilePath = "/etc/letsencrypt/live/net.0ms.dev/fullchain.pem"
	KeyFilePath  = "/etc/letsencrypt/live/net.0ms.dev/privkey.pem"

	tlsConf = &tls.Config{
		InsecureSkipVerify: true,
	}
)

// cake function
func cake() {

	// calculate bandwidth percentage
	bwUL90 = ((maxUL * 90) / 100)
	bwDL90 = ((maxDL * 90) / 100)

	// set last bandwidth values
	bwUL = maxUL
	bwDL = maxDL

	// infinite loop to change cake parameters in real-time
	for {

		time.Sleep(10 * time.Millisecond)

		// save newRTT to newRTTus
		newRTTus = newRTT

		// handle bufferbloat state
		if bufferbloatState {

			// cubic function.
			// see https://learn-sys.github.io/cn/slides/r0/week12-1.pdf for the details.
			// ------
			// this is T
			lastBufferbloatTimeElapsed = time.Since(lastBufferbloatTimestamp) / time.Duration(float64(time.Second))

			// this is K
			kUL = math.Cbrt((lastBwUL * (1 - B) / C))
			kDL = math.Cbrt((lastBwDL * (1 - B) / C))

			// check T
			if lastBufferbloatTimeElapsed <= 0 {

				bwUL = B * lastBwUL
				bwDL = B * lastBwDL
			} else if lastBufferbloatTimeElapsed > 0 {

				bwUL = C*math.Pow((float64(lastBufferbloatTimeElapsed)-kUL), 3) + lastBwUL
				bwDL = C*math.Pow((float64(lastBufferbloatTimeElapsed)-kDL), 3) + lastBwDL
			}

			bufferbloatState = false
		}

		if len(cakeDataJSON) >= cakeDataLimit {
			cakeDataJSON = nil
			rttArr = nil
			bwUpArr = nil
			bwDownArr = nil
		}

		cakeDataJSON = append(cakeDataJSON, CakeData{RTT: newRTTus, BandwidthUpload: bwUL, BandwidthDownload: bwDL})
		rttArr = append(rttArr, float64(newRTTus))
		bwUpArr = append(bwUpArr, bwUL)
		bwDownArr = append(bwDownArr, bwDL)

		rttAvgTotal = 0
		rttAvgDuration = 0
		for rttIdx := range rttArr {
			rttAvgTotal = float64(rttAvgTotal + rttArr[rttIdx])
		}
		rttAvgTotal = float64(rttAvgTotal) / float64(len(rttArr))
		rttAvgDuration = time.Duration(rttAvgTotal)
		newRTTus = rttAvgDuration

		bwUpAvgTotal = 0
		bwDownAvgTotal = 0
		for bwUpIdx := range bwUpArr {
			bwUpAvgTotal = float64(bwUpAvgTotal + bwUpArr[bwUpIdx])
		}
		for bwDownIdx := range bwDownArr {
			bwDownAvgTotal = float64(bwDownAvgTotal + bwDownArr[bwDownIdx])
		}
		bwUpAvgTotal = float64(bwUpAvgTotal) / float64(len(bwUpArr))
		bwDownAvgTotal = float64(bwDownAvgTotal) / float64(len(bwDownArr))
		bwUL = bwUpAvgTotal
		bwDL = bwDownAvgTotal

		// update last bandwidth values
		lastBwUL = bwUL
		lastBwDL = bwDL

		cakeJSON = Cake{RTTAverage: rttAvgDuration, RTTAverageString: fmt.Sprintf("%.2f ms | %.2f μs", float64(newRTTus/time.Millisecond), float64(newRTTus/time.Microsecond)), BwUpAverage: bwUpAvgTotal, BwUpAverageString: fmt.Sprintf("%.2f kbit | %.2f Mbit", bwUL, (bwUL / Mbit)), BwDownAverage: bwDownAvgTotal, BwDownAverageString: fmt.Sprintf("%.2f kbit | %.2f Mbit", bwDL, (bwDL / Mbit)), DataTotal: fmt.Sprintf("%v of %v", len(cakeDataJSON), cakeDataLimit)}

		// normalize RTT
		if newRTTus < metroRTT {
			newRTTus = metroRTT
		} else if newRTTus > satelliteRTT {
			newRTTus = satelliteRTT
		}

		// convert to microseconds
		newRTTus = newRTTus / time.Microsecond

		// use autoSplitGSO
		if bwUL < (100*Mbit) || bwDL < (100*Mbit) {
			autoSplitGSO = "split-gso"
		} else if bwUL > (100*Mbit) || bwDL > (100*Mbit) {
			autoSplitGSO = "no-split-gso"
		}

		// set uplink
		cakeUplink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", uplinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTTus), "bandwidth", fmt.Sprintf("%fkbit", bwUL), fmt.Sprintf("%v", autoSplitGSO))
		output, err := cakeUplink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return
		}
		// set downlink
		cakeDownlink := exec.Command("tc", "qdisc", "replace", "dev", fmt.Sprintf("%v", downlinkInterface), "root", "cake", "rtt", fmt.Sprintf("%dus", newRTTus), "bandwidth", fmt.Sprintf("%fkbit", bwDL), fmt.Sprintf("%v", autoSplitGSO))
		output, err = cakeDownlink.Output()

		if err != nil {
			fmt.Println(err.Error() + ": " + string(output))
			return
		}

		// prevent bandwidth too low
		bwUL = bwUL + (bwUL / 2)
		bwDL = bwDL + (bwDL / 2)

		// automatically limit max bandwidth to 90%
		if bwUL > bwUL90 {
			bwUL = bwUL90
		}
		if bwDL > bwDL90 {
			bwDL = bwDL90
		}

	}
}

func cakeServer() {

	duration := time.Now()

	// Use Gin as the HTTP router
	gin.SetMode(gin.ReleaseMode)
	recover := gin.New()
	recover.Use(gin.Recovery())
	ginroute := recover

	// Custom NotFound handler
	ginroute.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, fmt.Sprintln("[404] NOT FOUND"))
	})

	// Print homepage
	ginroute.GET("/", func(c *gin.Context) {
		runtime.ReadMemStats(&mem)
		NumGCMem = fmt.Sprintf("%v", mem.NumGC)
		timeElapsed = fmt.Sprintf("%v", time.Since(duration))

		latestLog = fmt.Sprintf("\n •===========================• \n • [SERVER STATUS] \n • Last Modified: %v \n • Completed GC Cycles: %v \n • Time Elapsed: %v \n •===========================• \n\n", time.Now().UTC().Format(time.RFC850), NumGCMem, timeElapsed)

		c.String(http.StatusOK, fmt.Sprintf("%v", latestLog))
	})

	// metrics for cake
	ginroute.GET("/cake", func(c *gin.Context) {
		c.IndentedJSON(http.StatusOK, cakeJSON)
	})

	tlsConf = &tls.Config{
		InsecureSkipVerify: true,
		// Certificates:       []tls.Certificate{serverTLSCert},
	}

	// HTTP proxy server Gin
	httpserverGin := &http.Server{
		Addr:              fmt.Sprintf(":%v", hostPortGin),
		Handler:           ginroute,
		TLSConfig:         tlsConf,
		MaxHeaderBytes:    64 << 10, // 64k
		ReadTimeout:       timeoutTr,
		ReadHeaderTimeout: timeoutTr,
		WriteTimeout:      timeoutTr,
		IdleTimeout:       timeoutTr,
	}
	httpserverGin.SetKeepAlivesEnabled(true)

	notifyGin := fmt.Sprintf("check cake metrics on %v", fmt.Sprintf(":%v", hostPortGin))

	fmt.Println()
	fmt.Println(notifyGin)
	fmt.Println()
	// httpserverGin.ListenAndServe()
	httpserverGin.ListenAndServeTLS(CertFilePath, KeyFilePath)

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

			// update timestamp
			lastBufferbloatTimestamp = time.Now()

			// update bufferbloat status
			bufferbloatState = true

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
