// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program speedtest provides the speedtest command. The reason to keep it separate from
// the normal tailscale cli is because it is not yet ready to go in the tailscale binary.
// It will be included in the tailscale cli after it has been added to tailscaled.
// Example usage for client command: go run cmd/speedtest -host 127.0.0.1:8080 -t 5s
// This will connect to the server on 127.0.0.1:8080 and start a 5 second speedtestd.
// Example usage for server command: go run cmd/speedtest -s -max-conns 1 -host :8080
// This will start a speedtest server on port 8080 and allow only 1 concurrent connection
// at a time.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"tailscale.com/net/speedtest"

	"github.com/peterbourgon/ff/v2/ffcli"
)

// Runs the speedtest command as a commandline program
func main() {
	args := os.Args[1:]
	if err := speedtestCmd.Parse(args); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}

	err := speedtestCmd.Run(context.Background())
	if errors.Is(err, flag.ErrHelp) {
		fmt.Println(speedtestCmd.ShortUsage)
		os.Exit(2)
	}
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
}

// speedtestCmd is the root command. It runs either the server and client depending on the
// flags passed to it.
var speedtestCmd = &ffcli.Command{
	Name:       "speedtest",
	ShortUsage: "speedtest [-s] [-max-conns <max connections>] [-t <test duration>]",
	ShortHelp:  "Run a speed test",
	FlagSet: (func() *flag.FlagSet {
		fs := flag.NewFlagSet("speedtest", flag.ExitOnError)
		fs.StringVar(&speedtestArgs.host, "host", ":0", "host:port pair to connect to or listen on")
		fs.IntVar(&speedtestArgs.maxConnections, "max-conns", 1, "max number of concurrent connections allowed")
		fs.DurationVar(&speedtestArgs.testDuration, "t", speedtest.DefaultDuration, "duration of the speed test")
		fs.BoolVar(&speedtestArgs.runServer, "s", false, "run a speedtest server")
		return fs
	})(),
	Exec: runSpeedtest,
}

var speedtestArgs struct {
	port           int
	maxConnections int
	host           string
	testDuration   time.Duration
	runServer      bool
}

func runSpeedtest(ctx context.Context, args []string) error {
	if speedtestArgs.runServer {
		listener, err := net.Listen("tcp", speedtestArgs.host)
		if err != nil {
			return err
		}
		tcpListener := listener.(*net.TCPListener)

		// If the user provides a 0 port, a random available port will be chosen,
		// so we need to identify which one was chosen, to display to the user.
		port := tcpListener.Addr().(*net.TCPAddr).Port
		fmt.Println("listening on port", port)

		resultsChan := make(chan []speedtest.Result, speedtestArgs.maxConnections)

		// this goroutine would end when the commandline program ends
		go func() {
			for results := range resultsChan {
				fmt.Println("Results:")
				var d time.Duration
				for _, result := range results {
					fmt.Print(displayUpload(result, d))
					d += result.Interval
				}
			}
		}()

		return speedtest.Serve(tcpListener, speedtestArgs.maxConnections, nil, resultsChan)
	}

	if speedtestArgs.host == "" {
		return errors.New("both host and port must be given")
	}

	// Ensure the duration is within the allowed range
	if speedtestArgs.testDuration < speedtest.MinDuration || speedtestArgs.testDuration > speedtest.MaxDuration {
		return errors.New(fmt.Sprintf("test duration must be within %.0fs and %.0fs.\n", speedtest.MinDuration.Seconds(), speedtest.MaxDuration.Seconds()))
	}

	fmt.Printf("Starting a test with %s\n", speedtestArgs.host)
	results, err := speedtest.RunClient(speedtestArgs.testDuration, speedtestArgs.host)
	if err != nil {
		return err
	}

	fmt.Println("Results:")
	var d time.Duration
	for _, result := range results {
		fmt.Print(displayDownload(result, d))
		d += result.Interval
	}
	return nil
}

// Returns a nicely formatted string to use when displaying the speeds in each result.
func displayDownload(r speedtest.Result, intervalStart time.Duration) string {
	var sb strings.Builder
	sb.WriteString("--------------------------------\n")
	if !r.Total {
		sb.WriteString(fmt.Sprintf("between  %.2f seconds and %.2f seconds:\n", intervalStart.Seconds(), (intervalStart.Seconds() + r.Interval.Seconds())))
		sb.WriteString(fmt.Sprintf("received %.4f Mb in %.2f second(s)\n", float64(r.Bytes)/1000000.0, r.Interval.Seconds()))
	} else {
		sb.WriteString("Total Speed\n")
		sb.WriteString(fmt.Sprintf("received %.4f Mb in %.3f second(s)\n", float64(r.Bytes)/1000000.0, r.Interval.Seconds()))
	}
	mbps := r.BytesPerSecond() * (8.0 / 1000000.0)
	sb.WriteString(fmt.Sprintf("download speed: %.4f Mbps\n", mbps))
	return sb.String()
}

func displayUpload(r speedtest.Result, intervalStart time.Duration) string {
	var sb strings.Builder
	sb.WriteString("--------------------------------\n")
	if !r.Total {
		sb.WriteString(fmt.Sprintf("between  %.2f seconds and %.2f seconds:\n", intervalStart.Seconds(), (intervalStart.Seconds() + r.Interval.Seconds())))
		sb.WriteString(fmt.Sprintf("sent %.4f Mb in %.2f second(s)\n", float64(r.Bytes)/1000000.0, r.Interval.Seconds()))
	} else {
		sb.WriteString("Total Speed\n")
		sb.WriteString(fmt.Sprintf("sent %.4f Mb in %.3f second(s)\n", float64(r.Bytes)/1000000.0, r.Interval.Seconds()))
	}
	mbps := r.BytesPerSecond() * (8.0 / 1000000.0)
	sb.WriteString(fmt.Sprintf("upload speed: %.4f Mbps\n", mbps))
	return sb.String()
}
