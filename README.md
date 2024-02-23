# Prometheus metrics for AI-on-the-edge devices

[![Latest release](https://img.shields.io/github/v/release/hansmi/prometheus-ai-on-the-edge-device-exporter)][releases]
[![CI workflow](https://github.com/hansmi/prometheus-ai-on-the-edge-device-exporter/actions/workflows/ci.yaml/badge.svg)](https://github.com/hansmi/prometheus-ai-on-the-edge-device-exporter/actions/workflows/ci.yaml)
[![Go reference](https://pkg.go.dev/badge/github.com/hansmi/prometheus-ai-on-the-edge-device-exporter.svg)](https://pkg.go.dev/github.com/hansmi/prometheus-ai-on-the-edge-device-exporter)

A Prometheus metrics exporter for [AI-on-the-edge devices][aiontheedge], an
artificial intelligence-based system to digitize analog measurements by meters
(water, gas, power, etc.) running on ESP32 microcontrollers.


## Usage

```shell
docker run --rm -p 8081:8081 ghcr.io/hansmi/prometheus-ai-on-the-edge-device-exporter:latest
```


## Scrape config

```yaml
scrape_configs:

# Metrics of exporter itself
- job_name: ai-on-the-edge-device-exporter
  static_configs:
    - targets: ["exporter:8081"]

- job_name: ai-on-the-edge-device
  metrics_path: /probe
  static_configs:
    - targets:
      - http://water
      - http://gas
  relabel_configs:
    - source_labels: [__address__]
      target_label: __param_target
    - source_labels: [__param_target]
      target_label: instance
    - target_label: __address__
      replacement: "exporter:8081"
```


## Installation

[Pre-built binaries][releases]:

* Binary archives (`.tar.gz`)
* Debian/Ubuntu (`.deb`)
* RHEL/Fedora (`.rpm`)
* Microsoft Windows (`.zip`)

Docker images via GitHub's container registry:

```shell
docker pull ghcr.io/hansmi/prometheus-ai-on-the-edge-device-exporter
```

With the source being available it's also possible to produce custom builds
directly using [Go][golang] or [GoReleaser][goreleaser].


[aiontheedge]: https://github.com/jomjol/AI-on-the-edge-device
[golang]: https://golang.org/
[goreleaser]: https://goreleaser.com/
[releases]: https://github.com/hansmi/prometheus-ai-on-the-edge-device-exporter/releases/latest

<!-- vim: set sw=2 sts=2 et : -->
