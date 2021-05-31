// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Program Speedtest provides the speedtest command. The reason to keep it seperate from
// the normal tailscale cli is because it is not yet ready to go in the tailscale binary.
package main

// Example usage for client command: go run cmd/speedtest/speedtest.go client --d --host 127.0.0.1 --port 8080 --inc 1 --seconds 5
// This will connect to the server (if one is running) on 127.0.0.1:8080 and start a 5 second speedtest that gathers data in 1 second
// increments.
// Example usage for server command: go run cmd/speedtest/speedtest.go server --max-conns 1 --port 8080 --localhost
// This will start a speedtest server on 127.0.0.1:8080 and allow only 1 concurrent connection at a time.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"tailscale.com/net/speedtest"

	"github.com/peterbourgon/ff/v2/ffcli"
)

// Runs the speedtest command as a commandline program
func main() {
	args := os.Args[1:]
	if err := speedtestCmd.Parse(args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err := speedtestCmd.Run(context.Background())
	if err != nil {
		if err != flag.ErrHelp {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

// Speedtest command that contains the server and client sub commands.
var speedtestCmd = &ffcli.Command{
	Name:       "speedtest",
	ShortUsage: "speedtest {server|client} ...",
	ShortHelp:  "Run a speed test",
	Subcommands: []*ffcli.Command{
		speedtestServerCmd,
		speedtestClientCmd,
	},
	Exec: func(context.Context, []string) error {
		return errors.New("subcommand required; run 'tailscale speedtest -h' for details")
	},
}

// speedtestServerCmd takes necessary info like the port to
// listen on and then passes them to the StartServer function in the speedtest package.
// if the localhost flag is given, the server will use 127.0.0.1, otherwise the server will
// use the tailscale ip address
var speedtestServerCmd = &ffcli.Command{
	Name:       "server",
	ShortUsage: "speedtest server [--port <port>] [--max-conns <max connections>]",
	ShortHelp:  "Start a speed test server",
	Exec:       runServer,
	FlagSet: (func() *flag.FlagSet {
		fs := flag.NewFlagSet("server", flag.ExitOnError)
		fs.IntVar(&serverArgs.port, "port", 0, "port to listen on")
		fs.IntVar(&serverArgs.maxConnections, "max-conns", 1, "max number of concurrent connections allowed")
		return fs
	})(),
}

// speedtestClientCmd takes info like the type of test to run, message size, test time, and the host and port
// of the speedtest server and passes them to the StartClient function in the speedtest package.
var speedtestClientCmd = &ffcli.Command{
	Name:       "client",
	ShortUsage: "speedtest client --host <host> --port <port> [--seconds <number of seconds>]",
	ShortHelp:  "Start a speed test client and connect to a speed test server",
	Exec:       runClient,
	FlagSet: (func() *flag.FlagSet {
		fs := flag.NewFlagSet("client", flag.ExitOnError)
		fs.StringVar(&clientArgs.host, "host", "", "The ip address for the speedtest server being used")
		fs.StringVar(&clientArgs.port, "port", "", "The port of the speedtest server being used")
		fs.IntVar(&clientArgs.seconds, "seconds", speedtest.MinNumSeconds, "The duration of the speed test in seconds")
		return fs
	})(),
}

var serverArgs struct {
	port           int
	maxConnections int
}

// runServer takes the port from the serverArgs variable, finds the tailscale ip if needed, then passes them
// to speedtest.GetListener. The listener is then passed to speedtest.StartServer. No channel is passed to
// StartServer, because to kill the server all the user has to do is do Ctrl+c.
func runServer(ctx context.Context, args []string) error {

	portString := strconv.Itoa(serverArgs.port)
	listener, err := net.Listen("tcp", ":"+portString)
	if err != nil {
		return err
	}
	tcpListener := listener.(*net.TCPListener)

	// If the user provides a 0 port, a random available port will be chosen,
	// so we need to identify which one was chosen, to display to the user.
	port := tcpListener.Addr().(*net.TCPAddr).Port
	fmt.Println("listening on port", port)

	resultsChan := make(chan []speedtest.Result, serverArgs.maxConnections)

	// this goroutine would end when the commandline program ends
	go (func() {
		for {
			results := <-resultsChan
			fmt.Println(len(results))
			fmt.Println("Results:")
			var intervalStart time.Duration
			for _, result := range results {
				fmt.Print(displayUpload(result, intervalStart))
				intervalStart += result.Interval
			}
		}
	})()

	return speedtest.Serve(tcpListener, serverArgs.maxConnections, nil, resultsChan)
}

var clientArgs struct {
	seconds int
	host    string
	port    string
}

// runClient checks that the given parameters are within the allowed range. It also checks
// that both the host and port of the server are given. It passes the parameters to the
// startClient function in the speedtest package. It then prints the results that are returned.
func runClient(ctx context.Context, args []string) error {
	if clientArgs.host == "" || clientArgs.port == "" {
		return errors.New("both host and port must be given")
	}
	var testDuration time.Duration
	// configure the time
	if clientArgs.seconds < speedtest.MinNumSeconds || clientArgs.seconds > speedtest.MaxNumSeconds {
		testDuration = time.Duration(speedtest.MinNumSeconds) * time.Second
	} else {
		testDuration = time.Duration(clientArgs.seconds) * time.Second
	}

	fmt.Printf("Starting a test with %s:%s\n", clientArgs.host, clientArgs.port)
	results, err := speedtest.RunClient(testDuration, clientArgs.host, clientArgs.port)
	if err != nil {
		return err
	}

	fmt.Println("Results:")
	var intervalStart time.Duration
	for _, result := range results {
		fmt.Print(displayDownload(result, intervalStart))
		intervalStart += result.Interval
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
