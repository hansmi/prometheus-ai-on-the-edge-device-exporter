package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hansmi/prometheus-ai-on-the-edge-device-exporter/internal/testutil"
)

func newTestCollector(t *testing.T, handler http.Handler) *collector {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)

	return newCollector(ctx, testutil.MustParseURL(t, ts.URL))
}

func TestCollector(t *testing.T) {
	c := newTestCollector(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			io.WriteString(w, `
{
  "main": {
    "value": "9.876",
    "error": "no error",
    "timestamp": "2000-01-02T10:11:12+1300"
  },
  "second": {
    "value": "",
    "raw": "00013.501",
    "pre": "13.564",
    "error": "Neg. Rate - Read:  - Raw: 00013.501 - Pre: 13.564 ",
    "rate": "",
    "timestamp": "2000-01-02T20:21:22+0100"
  }
}
`)
		case "/sysinfo":
			io.WriteString(w, `
[
  {
    "firmware": "v15.3.0",
    "buildtime": "2023-07-22 09:42",
    "gitbranch": "HEAD",
    "gittag": "v15.3.0",
    "gitrevision": "3fbff0a",
    "html": "Release: v15.3.0 (Commit: 3fbff0a)",
    "cputemp": "50",
    "hostname": "device123",
    "IPv4": "192.0.2.1",
    "freeHeapMem": "10623"
  }
]
`)
		case "/rssi":
			io.WriteString(w, "-90")

		default:
			http.Error(w, "", http.StatusNotFound)
		}
	}))

	testutil.CollectAndCompare(t, c, `
# HELP ai_on_the_edge_device_cpu_temperature_celsius CPU temperature in degrees celsius.
# TYPE ai_on_the_edge_device_cpu_temperature_celsius gauge
ai_on_the_edge_device_cpu_temperature_celsius 50

# HELP ai_on_the_edge_device_firmware_info Firmware metadata.
# TYPE ai_on_the_edge_device_firmware_info gauge
ai_on_the_edge_device_firmware_info{gitrevision="3fbff0a",gittag="v15.3.0",version="v15.3.0"} 1

# HELP ai_on_the_edge_device_flow_error_info Error encountered during digitization.
# TYPE ai_on_the_edge_device_flow_error_info gauge
ai_on_the_edge_device_flow_error_info{message="",name="main"} 1
ai_on_the_edge_device_flow_error_info{message="Neg. Rate - Read:  - Raw: 00013.501 - Pre: 13.564",name="second"} 1

# HELP ai_on_the_edge_device_flow_success Whether digitization was successful.
# TYPE ai_on_the_edge_device_flow_success gauge
ai_on_the_edge_device_flow_success{name="main"} 1
ai_on_the_edge_device_flow_success{name="second"} 0

# HELP ai_on_the_edge_device_flow_timestamp_seconds Timestamp of the most recent digitization.
# TYPE ai_on_the_edge_device_flow_timestamp_seconds gauge
ai_on_the_edge_device_flow_timestamp_seconds{name="main"} 946761072
ai_on_the_edge_device_flow_timestamp_seconds{name="second"} 946840882

# HELP ai_on_the_edge_device_flow_value Most recent value.
# TYPE ai_on_the_edge_device_flow_value counter
ai_on_the_edge_device_flow_value{name="main"} 9.876

# HELP ai_on_the_edge_device_memory_heap_free_bytes Bytes available on the heap.
# TYPE ai_on_the_edge_device_memory_heap_free_bytes gauge
ai_on_the_edge_device_memory_heap_free_bytes 10623

# HELP ai_on_the_edge_device_network_info Network metadata.
# TYPE ai_on_the_edge_device_network_info gauge
ai_on_the_edge_device_network_info{hostname="device123",ipv4="192.0.2.1"} 1

# HELP ai_on_the_edge_device_rssi_dbm WiFi signal strength in dBm.
# TYPE ai_on_the_edge_device_rssi_dbm gauge
ai_on_the_edge_device_rssi_dbm -90
`)
}

func TestCollectSysinfo(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name:    "empty",
			handler: func(w http.ResponseWriter, r *http.Request) {},
			wantErr: errEmptyResponse,
		},
		{
			name: "request error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "", http.StatusTeapot)
			},
			wantErr: errRequestFailed,
		},
		{
			name: "bad number",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `[{"cputemp": "hello", "freeHeapMem": "world"}]`)
			},
			wantErr: cmpopts.AnyError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestCollector(t, tc.handler)

			err := c.collectSysinfo(context.Background(), testutil.DiscardMetrics(t))

			if diff := cmp.Diff(tc.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("Error diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCollectRssi(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name:    "empty",
			handler: func(w http.ResponseWriter, r *http.Request) {},
			wantErr: io.EOF,
		},
		{
			name: "request error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "", http.StatusTeapot)
			},
			wantErr: errRequestFailed,
		},
		{
			name: "bad number",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `hello world`)
			},
			wantErr: strconv.ErrSyntax,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestCollector(t, tc.handler)

			err := c.collectRssi(context.Background(), testutil.DiscardMetrics(t))

			if diff := cmp.Diff(tc.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("Error diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCollectFlows(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name:    "empty",
			handler: func(w http.ResponseWriter, r *http.Request) {},
			wantErr: errEmptyResponse,
		},
		{
			name: "request error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "", http.StatusTeapot)
			},
			wantErr: errRequestFailed,
		},
		{
			name: "empty data",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{ "foo": {} }`)
			},
		},
		{
			name: "bad value",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{ "foo": { "value": "hello" } }`)
			},
			wantErr: cmpopts.AnyError,
		},
		{
			name: "bad timestamp",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{ "foo": { "timestamp": "hello" } }`)
			},
			wantErr: cmpopts.AnyError,
		},
		{
			name: "negative rate",
			handler: func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `
{
  "main": {
    "value": "",
    "raw": "00013.501",
    "pre": "13.564",
    "error": "Neg. Rate - Read:  - Raw: 00013.501 - Pre: 13.564 ",
    "rate": "",
    "timestamp": "2000-01-02T20:21:22+0100"
  }
}
`)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestCollector(t, tc.handler)

			err := c.collectFlows(context.Background(), testutil.DiscardMetrics(t))

			if diff := cmp.Diff(tc.wantErr, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("Error diff (-want +got):\n%s", diff)
			}
		})
	}
}
