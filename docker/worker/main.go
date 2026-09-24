// Command worker is a small background agent:
// it logs the container's load average and available memory on an interval, reading /proc directly.
// Stdlib only -- no external module, so the image needs no dependency download, just the Go toolchain.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// readLoadAvg returns the "1m 5m 15m" fields of /proc/loadavg.
// opens /proc/loadavg, a text file the Linux kernel itself maintains, and pulls out the "how busy is this system" numbers.
func readLoadAvg() (string, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return "", fmt.Errorf("unexpected /proc/loadavg format: %q", data)
	}
	return strings.Join(fields[:3], " "), nil
}

// opens /proc/meminfo, finds the line starting with MemAvailable:, and converts it to megabytes.
func readMemAvailableMB() (int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("unexpected /proc/meminfo line: %q", line)
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		return kb / 1024, nil
	}
	return 0, fmt.Errorf("MemAvailable not found in /proc/meminfo")
}

func main() {
	healthcheck := flag.Bool("healthcheck", false, "do a real /proc read and exit 0/1, instead of running the loop")
	flag.Parse()

	// HEALTHCHECK's CMD: a fresh read, not cached state from the loop below --
	// scratch has no shell, so this flag stands in for a wget/curl-based check.
	// If -healthcheck was passed, it does one load-average read, and immediately exits.
	if *healthcheck {
		if _, err := readLoadAvg(); err != nil {
			log.Printf("healthcheck failed: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

    // Reads how often to check (WORKER_INTERVAL, default 30s)
	interval := 30 * time.Second
	if v := os.Getenv("WORKER_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		} else {
			log.Printf("invalid WORKER_INTERVAL %q, using default %s", v, interval)
		}
	}

    // Registers itself to be notified if the OS sends it a SIGTERM/SIGINT signal
	// SIGTERM: STOPSIGNAL in the Dockerfile, and how Compose/Kubernetes ask a container to shut down.
	// Handling it here (instead of just dying) is what stop_grace_period / terminationGracePeriodSeconds are giving time for.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log.Printf("worker started, logging load/memory every %s", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

    // This program is written to runs forever and never crash even on a bad read, doing one of two things each time around:
    //  - If a shutdown signal arrived → log it and return (exit the program, cleanly)
    //  - If the timer ticked → read load/memory and log them, then loop again
	for {
		select {
		case <-ctx.Done():
			log.Println("received shutdown signal, exiting")
			return
		case <-ticker.C:
			load, err := readLoadAvg()
			if err != nil {
				log.Printf("read load average: %v", err)
				continue    // <-- doesn't crash, just logs and tries again next tick
			}
			memMB, err := readMemAvailableMB()
			if err != nil {
				log.Printf("read available memory: %v", err)
				continue    // <-- doesn't crash, just logs and tries again next tick
			}
			log.Printf("load avg (1m 5m 15m): %s -- available memory: %d MB", load, memMB)
		}
	}
}
