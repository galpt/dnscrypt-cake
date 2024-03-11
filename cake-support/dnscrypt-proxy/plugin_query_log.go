package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/jedisct1/dlog"
	"github.com/miekg/dns"

	"github.com/gin-gonic/gin"
	"github.com/spf13/afero"
)

type (
	Cake struct {
		RTTAverage          time.Duration `json:"rttAverage"`
		RTTAverageString    string        `json:"rttAverageString"`
		BwUpAverage         float64       `json:"bwUpAverage"`
		BwUpAverageString   string        `json:"bwUpAverageString"`
		BwDownAverage       float64       `json:"bwDownAverage"`
		BwDownAverageString string        `json:"bwDownAverageString"`
		BwUpMedian          float64       `json:"bwUpMedian"`
		BwUpMedianString    string        `json:"bwUpMedianString"`
		BwDownMedian        float64       `json:"bwDownMedian"`
		BwDownMedianString  string        `json:"bwDownMedianString"`
		DataTotal           string        `json:"dataTotal"`
		ExecTimeCAKE        string        `json:"execTimeCAKE"`
		ExecTimeAverageCAKE string        `json:"execTimeAverageCAKE"`
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
	maxUL float64 = 4000000
	maxDL float64 = 4000000
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
	Mbit float64 = 1000.00    // 1 Mbit
	Gbit float64 = 1000000.00 // 1 Gbit
	// ------
	B float64 = 0.70
	C float64 = 0.40
	// ------
	Megabyte      = 1 << 20
	Kilobyte      = 1 << 10
	timeoutTr     = 30 * time.Second
	hostPortGin   = "0.0.0.0:22222"
	cakeDataLimit = 10000000 // 10 million
	usrAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"
)

// do not touch these.
// should be maintained by the functions automatically.
var (
	bwUL   float64 = 2
	bwDL   float64 = 2
	bwUL90 float64 = 90
	bwDL90 float64 = 90

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

	cakeExecTime            time.Time
	cakeExecTimeArr         []float64
	cakeExecTimeAvgTotal    float64       = 0
	cakeExecTimeAvgDuration time.Duration = 0

	rttArr         []float64
	rttAvgTotal    float64       = 0
	rttAvgDuration time.Duration = 0

	bwUpArr        []float64
	bwUpAvgTotal   float64 = 0
	bwDownArr      []float64
	bwDownAvgTotal float64 = 0

	bwUpMedTotal   float64 = 0
	bwDownMedTotal float64 = 0

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

	h1Tr = &http.Transport{
		DisableKeepAlives:      false,
		DisableCompression:     false,
		ForceAttemptHTTP2:      false,
		TLSClientConfig:        tlsConf,
		TLSHandshakeTimeout:    timeoutTr,
		ResponseHeaderTimeout:  timeoutTr,
		IdleConnTimeout:        timeoutTr,
		ExpectContinueTimeout:  1 * time.Second,
		MaxIdleConns:           1000,     // Prevents resource exhaustion
		MaxIdleConnsPerHost:    100,      // Increases performance and prevents resource exhaustion
		MaxConnsPerHost:        0,        // 0 for no limit
		MaxResponseHeaderBytes: 64 << 10, // 64k
		WriteBufferSize:        64 << 10, // 64k
		ReadBufferSize:         64 << 10, // 64k
	}

	h1Client = &http.Client{
		Transport: h1Tr,
		Timeout:   timeoutTr,
	}

	osFS = afero.NewOsFs()
)

// fetch OISD Big blocklist every 1 hour
func oisdBigFetch() {

	for {

		req, err := http.NewRequest("GET", "https://big.oisd.nl/domainswild", nil)
		if err != nil {
			fmt.Println(" [req] ", err)
			return
		}
		req.Header.Set("User-Agent", usrAgent)

		getData, err := h1Client.Do(req)
		if err != nil {
			fmt.Println(" [getData] ", err)
			return
		}

		// delete 'oisd-big.txt' file
		osFS.RemoveAll("./oisd-big.txt")

		// create a new file
		createFile, err := osFS.Create("./oisd-big.txt")
		if err != nil {
			fmt.Println(" [createFile] ", err)

			return
		}

		// write response body to the newly created file
		writeFile, err := io.Copy(createFile, getData.Body)
		if err != nil {
			fmt.Println(" [writeFile] ", err)
			return
		}

		// print to let us know if blocklist has been downloaded and processed
		sizeinfo := fmt.Sprintf("'oisd-big.txt' has been processed (%v KB | %v MB).", (writeFile / Kilobyte), (writeFile / Megabyte))
		fmt.Println(sizeinfo)

		// close io
		if err := createFile.Close(); err != nil {
			fmt.Println(" [createFile.Close()] ", err)
			return
		}

		// close response body after being used
		getData.Body.Close()

		// sleep for 1 hour
		time.Sleep(1 * time.Hour)
	}
}

// cake functions
func cakeCheckArrays() {
	// when cakeDataLimit is reached,
	// remove the first data from the slices.
	if len(cakeDataJSON) >= cakeDataLimit {
		cakeDataJSON = cakeDataJSON[1:]
		rttArr = rttArr[1:]
		bwUpArr = bwUpArr[1:]
		bwDownArr = bwDownArr[1:]
		cakeExecTimeArr = cakeExecTimeArr[1:]
	}
}

func cakeAppendValues() {
	cakeDataJSON = append(cakeDataJSON, CakeData{RTT: newRTTus, BandwidthUpload: bwUL, BandwidthDownload: bwDL})
	rttArr = append(rttArr, float64(newRTTus))
	bwUpArr = append(bwUpArr, bwUL)
	bwDownArr = append(bwDownArr, bwDL)
}

func cakeMultiplyBandwidth() {

	// multiply the values by 2 if they're less than 90%.
	if bwUL < bwUL90 {
		bwUL *= 2
	}
	if bwDL < bwDL90 {
		bwDL *= 2
	}

	// limit current bandwidth values to 90% of maximum bandwidth specified.
	if bwUL >= bwUL90 {
		bwUL = bwUL90
	}
	if bwDL >= bwDL90 {
		bwDL = bwDL90
	}
}

func cakeNormalizeRTT() {
	// normalize RTT
	if newRTTus < (metroRTT / time.Microsecond) {
		newRTTus = (metroRTT / time.Microsecond)
	} else if newRTTus > (satelliteRTT / time.Microsecond) {
		newRTTus = (satelliteRTT / time.Microsecond)
	}
}

func cakeConvertRTTtoMicroseconds() {
	// convert to microseconds
	newRTTus = newRTT / time.Microsecond
}

func cakeAutoSplitGSO() {
	// automatically use "split-gso" when bandwidth is less than 50%.
	if bwUL < (float64(bwUL)*float64(0.5)) || bwDL < (float64(bwDL)*float64(0.5)) {
		autoSplitGSO = "split-gso"
	} else {
		autoSplitGSO = "no-split-gso"
	}
}

func cakeQdiscReconfigure() {

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
}

func cakeBufferbloatBandwidth() {
	// when a bufferbloat is detected, we should slow things down.
	bwUL = float64(bwUL) * float64(0.2)
	bwDL = float64(bwDL) * float64(0.2)
}

func cakeCalculateRTTandBandwidth() {
	rttAvgTotal = 0
	rttAvgDuration = 0
	bwUpAvgTotal = 0
	bwDownAvgTotal = 0
	cakeExecTimeAvgTotal = 0
	cakeExecTimeAvgDuration = 0

	for rttIdx := range rttArr {
		rttAvgTotal = float64(rttAvgTotal + rttArr[rttIdx])
		bwUpAvgTotal = float64(bwUpAvgTotal + bwUpArr[rttIdx])
		bwDownAvgTotal = float64(bwDownAvgTotal + bwDownArr[rttIdx])
	}

	rttAvgTotal = float64(rttAvgTotal) / float64(len(rttArr))
	rttAvgDuration = time.Duration(rttAvgTotal)
	newRTTus = rttAvgDuration
	bwUpAvgTotal = float64(bwUpAvgTotal) / float64(len(bwUpArr))
	bwDownAvgTotal = float64(bwDownAvgTotal) / float64(len(bwDownArr))

	if len(bwUpArr)%2 == 0 {
		bwUpMedTotal = ((bwUpArr[len(bwUpArr)-1] / 2) + ((bwUpArr[len(bwUpArr)-1]/2)+1)/2)
	} else {
		bwUpMedTotal = (bwUpArr[len(bwUpArr)-1] + 1) / 2
	}

	if len(bwDownArr)%2 == 0 {
		bwDownMedTotal = ((bwDownArr[len(bwDownArr)-1] / 2) + ((bwDownArr[len(bwDownArr)-1]/2)+1)/2)
	} else {
		bwDownMedTotal = (bwDownArr[len(bwDownArr)-1] + 1) / 2
	}

	// use median values as optimal bandwidth if more than 20% of maxUL/maxDL.
	if bwUpMedTotal > (float64(maxUL) * float64(0.2)) {
		bwUL = bwUpMedTotal
	}

	if bwUpMedTotal > (float64(maxDL) * float64(0.2)) {
		bwDL = bwDownMedTotal
	}
}

func cakeHandleAvgRTT() {
	if rttAvgDuration > newRTTus {
		newRTTus = rttAvgDuration
	}
}

func cakeHandleJSON() {
	cakeExecTimeArr = append(cakeExecTimeArr, float64(time.Since(cakeExecTime)))

	for execTimeIdx := range cakeExecTimeArr {
		cakeExecTimeAvgTotal = float64(cakeExecTimeAvgTotal + cakeExecTimeArr[execTimeIdx])
	}

	cakeExecTimeAvgTotal = float64(cakeExecTimeAvgTotal) / float64(len(cakeExecTimeArr))
	cakeExecTimeAvgDuration = time.Duration(cakeExecTimeAvgTotal)

	cakeJSON = Cake{RTTAverage: rttAvgDuration, RTTAverageString: fmt.Sprintf("%.2f ms | %.2f μs", (float64(rttAvgDuration) / float64(1000.00)), float64(rttAvgDuration)), BwUpAverage: bwUpAvgTotal, BwUpAverageString: fmt.Sprintf("%.2f kbit | %.2f Mbit", bwUpAvgTotal, (bwUpAvgTotal / Mbit)), BwDownAverage: bwDownAvgTotal, BwDownAverageString: fmt.Sprintf("%.2f kbit | %.2f Mbit", bwDownAvgTotal, (bwDownAvgTotal / Mbit)), BwUpMedian: bwUpMedTotal, BwUpMedianString: fmt.Sprintf("%.2f kbit | %.2f Mbit", bwUpMedTotal, (bwUpMedTotal / Mbit)), BwDownMedian: bwDownMedTotal, BwDownMedianString: fmt.Sprintf("%.2f kbit | %.2f Mbit", bwDownMedTotal, (bwDownMedTotal / Mbit)), DataTotal: fmt.Sprintf("%v of %v", len(cakeDataJSON), cakeDataLimit), ExecTimeCAKE: fmt.Sprintf("%.2f ms | %.2f μs", (float64(cakeExecTimeArr[len(cakeExecTimeArr)-1]) / float64(time.Millisecond)), (float64(cakeExecTimeArr[len(cakeExecTimeArr)-1]) / float64(time.Microsecond))), ExecTimeAverageCAKE: fmt.Sprintf("%.2f ms | %.2f μs", (float64(cakeExecTimeAvgDuration) / float64(time.Millisecond)), (float64(cakeExecTimeAvgDuration) / float64(time.Microsecond)))}
}

func bufferbloatTrue() {
	bufferbloatState = true
}

func bufferbloatFalse() {
	bufferbloatState = false
}

func cake() {

	// calculate bandwidth percentage
	bwUL90 = ((maxUL * 90) / 100)
	bwDL90 = ((maxDL * 90) / 100)

	// set last bandwidth values
	bwUL = maxUL
	bwDL = maxDL

	// infinite loop to change cake parameters in real-time
	for {

		// sleep for 1 millisecond
		time.Sleep(time.Millisecond)

		// counting exec time starts from here
		cakeExecTime = time.Now()

		// handle bufferbloat state
		if bufferbloatState {

			cakeBufferbloatBandwidth()
			cakeQdiscReconfigure()

			// then restore the bandwidth over time.
			// if the bandwidth ratio is 1:1,
			// then increase both values at the same time.
			if maxUL == maxDL {
				for bwUL < bwUL90 {

					cakeCheckArrays()
					cakeAppendValues()
					cakeMultiplyBandwidth()
					cakeConvertRTTtoMicroseconds()
					cakeNormalizeRTT()
					cakeAutoSplitGSO()
					cakeQdiscReconfigure()

				}

			} else {

				// if the bandwidth ratio isn't 1:1,
				// then handle them separately.
				for bwUL < bwUL90 || bwDL < bwDL90 {

					cakeCheckArrays()
					cakeAppendValues()
					cakeMultiplyBandwidth()
					cakeConvertRTTtoMicroseconds()
					cakeNormalizeRTT()
					cakeAutoSplitGSO()
					cakeQdiscReconfigure()

				}

			}

			bufferbloatFalse()
		}

		cakeCheckArrays()
		cakeAppendValues()
		cakeCalculateRTTandBandwidth()
		cakeConvertRTTtoMicroseconds()
		cakeNormalizeRTT()
		cakeAutoSplitGSO()
		cakeQdiscReconfigure()

		// keep increasing current bandwidth in there's no bufferbloat.
		for bwUL < bwUL90 || bwDL < bwDL90 {

			cakeCheckArrays()
			cakeAppendValues()
			cakeMultiplyBandwidth()
			cakeConvertRTTtoMicroseconds()
			cakeNormalizeRTT()
			cakeAutoSplitGSO()
			cakeQdiscReconfigure()

		}

		cakeHandleAvgRTT()
		cakeQdiscReconfigure()

		cakeHandleJSON()

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
		Addr:              fmt.Sprintf("%v", hostPortGin),
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

			// update bufferbloat status
			bufferbloatTrue()

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
