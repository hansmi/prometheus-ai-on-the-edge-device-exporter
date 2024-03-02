package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
)

const metricNamePrefix = "ai_on_the_edge_device_"

var errRequestFailed = errors.New("request failed")
var errEmptyResponse = errors.New("empty response")

var descriptors = struct {
	error *prometheus.Desc

	firmwareInfo *prometheus.Desc
	netInfo      *prometheus.Desc
	cpuTemp      *prometheus.Desc
	rssi         *prometheus.Desc
	memHeapFree  *prometheus.Desc

	flowValue     *prometheus.Desc
	flowSuccess   *prometheus.Desc
	flowErrorInfo *prometheus.Desc
	flowTimestamp *prometheus.Desc
}{
	error: prometheus.NewDesc(metricNamePrefix+"error",
		"Metrics collection failed.",
		nil, nil),

	firmwareInfo: prometheus.NewDesc(metricNamePrefix+"firmware_info",
		"Firmware metadata.",
		[]string{"version", "gittag", "gitrevision"}, nil),
	netInfo: prometheus.NewDesc(metricNamePrefix+"network_info",
		"Network metadata.",
		[]string{"hostname", "ipv4"}, nil),
	rssi: prometheus.NewDesc(metricNamePrefix+"rssi_dbm",
		"WiFi signal strength in dBm.",
		nil, nil),
	cpuTemp: prometheus.NewDesc(metricNamePrefix+"cpu_temperature_celsius",
		"CPU temperature in degrees celsius.",
		nil, nil),
	memHeapFree: prometheus.NewDesc(metricNamePrefix+"memory_heap_free_bytes",
		"Bytes available on the heap.",
		nil, nil),

	flowValue: prometheus.NewDesc(metricNamePrefix+"flow_value",
		"Most recent value.",
		[]string{"name"}, nil),
	flowSuccess: prometheus.NewDesc(metricNamePrefix+"flow_success",
		"Whether digitization was successful.",
		[]string{"name"}, nil),
	flowErrorInfo: prometheus.NewDesc(metricNamePrefix+"flow_error_info",
		"Error encountered during digitization.",
		[]string{"name", "message"}, nil),
	flowTimestamp: prometheus.NewDesc(metricNamePrefix+"flow_timestamp_seconds",
		"Timestamp of the most recent digitization.",
		[]string{"name"}, nil),
}

var client = func() *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 10 * time.Second,
		}).DialContext,
		DisableCompression:    true,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   1,
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   10 * time.Second,

		// The HTTP server is single-threaded.
		MaxConnsPerHost: 1,
	}

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}()

func collectJsonNumber(ch chan<- prometheus.Metric, name string,
	desc *prometheus.Desc, valueType prometheus.ValueType, raw json.RawMessage,
	labelValues ...string,
) error {
	if len(raw) == 0 {
		// Message is unset
		return nil
	}

	var s string

	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	if s == "" {
		// Number is missing
		return nil
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	ch <- prometheus.MustNewConstMetric(desc, valueType, value, labelValues...)

	return nil
}

type collector struct {
	ctx    context.Context
	target *url.URL
}

func newCollector(ctx context.Context, target *url.URL) *collector {
	return &collector{ctx, target}
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descriptors.firmwareInfo
	ch <- descriptors.netInfo
	ch <- descriptors.rssi
	ch <- descriptors.cpuTemp
	ch <- descriptors.memHeapFree
	ch <- descriptors.flowValue
	ch <- descriptors.flowSuccess
	ch <- descriptors.flowErrorInfo
	ch <- descriptors.flowTimestamp
}

func (c *collector) newRequest(ctx context.Context, path string) (*http.Request, error) {
	u := c.target.JoinPath(path)

	return http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
}

func (c *collector) doRequest(ctx context.Context, path string) (*http.Response, error) {
	if req, err := c.newRequest(ctx, path); err != nil {
		return nil, err
	} else if resp, err := client.Do(req); err != nil {
		return nil, err
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %q: %v", errRequestFailed, req.URL.String(), resp.Status)
	} else {
		return resp, nil
	}
}

func (c *collector) doJsonRequest(ctx context.Context, path string, v any) error {
	resp, err := c.doRequest(ctx, path)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.ContentLength == 0 {
		return fmt.Errorf("%q: %w", resp.Request.URL.String(), errEmptyResponse)
	}

	dec := json.NewDecoder(resp.Body)

	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("JSON decoder: %w", err)
	}

	return nil
}

type sysinfoData struct {
	Firmware    string          `json:"firmware"`
	GitTag      string          `json:"gittag"`
	GitRevision string          `json:"gitrevision"`
	CpuTemp     json.RawMessage `json:"cputemp"`
	Hostname    string          `json:"hostname"`
	IPv4        string          `json:"ipv4"`
	FreeHeapMem json.RawMessage `json:"freeHeapMem"`
}

func (c *collector) collectSysinfo(ctx context.Context, ch chan<- prometheus.Metric) error {
	var payload []sysinfoData

	if err := c.doJsonRequest(ctx, "/sysinfo", &payload); err != nil {
		return err
	}

	if len(payload) < 1 {
		return errors.New("sysinfo missing from response")
	}

	data := payload[0]

	ch <- prometheus.MustNewConstMetric(descriptors.firmwareInfo, prometheus.GaugeValue, 1,
		data.Firmware, data.GitTag, data.GitRevision)
	ch <- prometheus.MustNewConstMetric(descriptors.netInfo, prometheus.GaugeValue, 1,
		data.Hostname, data.IPv4)

	if err := collectJsonNumber(ch, "cputemp", descriptors.cpuTemp, prometheus.GaugeValue, data.CpuTemp); err != nil {
		return err
	}

	if err := collectJsonNumber(ch, "freeHeapMem", descriptors.memHeapFree, prometheus.GaugeValue, data.FreeHeapMem); err != nil {
		return err
	}

	return nil
}

func (c *collector) collectRssi(ctx context.Context, ch chan<- prometheus.Metric) error {
	resp, err := c.doRequest(ctx, "/rssi")
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	var value float64

	if n, err := fmt.Fscanf(resp.Body, "%f\n", &value); err != nil {
		return fmt.Errorf("RSSI: %w", err)
	} else if n < 1 {
		return errors.New("RSSI value missing")
	} else {
		ch <- prometheus.MustNewConstMetric(descriptors.rssi, prometheus.GaugeValue, value)
	}

	return nil
}

type flowData struct {
	Value     json.RawMessage `json:"value"`
	Error     string          `json:"error"`
	Timestamp string          `json:"timestamp"`
}

func (d flowData) collect(ch chan<- prometheus.Metric, name string) error {
	if err := collectJsonNumber(ch, "value", descriptors.flowValue, prometheus.CounterValue, d.Value, name); err != nil {
		return err
	}

	var successValue float64

	errMsg := strings.TrimSpace(d.Error)

	switch errMsg {
	case "", "no error":
		successValue = 1
		errMsg = ""
	}

	ch <- prometheus.MustNewConstMetric(descriptors.flowSuccess, prometheus.GaugeValue, successValue, name)
	ch <- prometheus.MustNewConstMetric(descriptors.flowErrorInfo, prometheus.GaugeValue, 1, name, errMsg)

	if d.Timestamp != "" {
		ts, err := time.Parse("2006-01-02T15:04:05-0700", d.Timestamp)
		if err != nil {
			return err
		}

		ch <- prometheus.MustNewConstMetric(descriptors.flowTimestamp, prometheus.GaugeValue, float64(ts.Unix()), name)
	}

	return nil
}

func (c *collector) collectFlows(ctx context.Context, ch chan<- prometheus.Metric) error {
	var payload map[string]flowData

	if err := c.doJsonRequest(ctx, "/json", &payload); err != nil {
		return err
	}

	for name, data := range payload {
		if err := data.collect(ch, name); err != nil {
			return fmt.Errorf("flow %q: %w", name, err)
		}
	}

	return nil
}

func (c *collector) collect(ch chan<- prometheus.Metric) error {
	g, ctx := errgroup.WithContext(c.ctx)
	g.SetLimit(runtime.GOMAXPROCS(0))

	for _, fn := range []func(context.Context, chan<- prometheus.Metric) error{
		c.collectSysinfo,
		c.collectRssi,
		c.collectFlows,
	} {
		fn := fn
		g.Go(func() error { return fn(ctx, ch) })
	}

	return g.Wait()
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	if err := c.collect(ch); err != nil {
		ch <- prometheus.NewInvalidMetric(descriptors.error, err)
	}
}
