// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package speedtest

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDownload(t *testing.T) {
	// start up the speedtest server with a hardcoded port and address

	// Create a channel to signal the server to close and defer the signal
	// so that the server closes when the test ends.
	killServer := make(chan bool, 1)
	defer (func() { killServer <- true })()

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	listener := l.(*net.TCPListener)

	_, serverPort, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal("cannot get the port: ", err)
	}
	t.Log("port found:", serverPort)
	serverIP := "127.0.0.1:" + string(serverPort)

	type state struct {
		err error
	}

	stateChan := make(chan state, 2)

	go (func() {
		err := Serve(listener, 1, killServer, nil)
		stateChan <- state{err: err}
	})()

	go (func() {
		results, err := RunClient(DefaultDuration, serverIP)
		if err != nil {
			fmt.Println("client died")
			stateChan <- state{err: err}
			return
		}
		var intervalStart time.Duration
		for _, result := range results {
			t.Log(displayDownload(result, intervalStart))
			intervalStart += result.Interval
		}
		stateChan <- state{err: nil}
	})()

	testState := <-stateChan
	if testState.err != nil {
		t.Fatal(testState.err)
	}

}

func displayDownload(r Result, intervalStart time.Duration) string {
	var sb strings.Builder
	sb.WriteString("--------------------------------\n")
	if !r.Total {
		sb.WriteString(fmt.Sprintf("between  %.2f seconds and %.2f seconds:\n", intervalStart.Seconds(), (intervalStart.Seconds() + r.Interval.Seconds())))
		sb.WriteString(fmt.Sprintf("received %.4f Mb in %.2f second(s)\n", float64(r.Bytes)/1000000.0, r.Interval.Seconds()))
	} else {
		sb.WriteString("Total Speed\n")
		sb.WriteString(fmt.Sprintf("received %.4f Mb in %.3f second(s)\n", float64(r.Bytes)/1000000.0, r.Interval.Seconds()))
	}
	// convert bytes per second to megabits per second
	mbps := r.BytesPerSecond() * (8.0 / 1000000.0)
	sb.WriteString(fmt.Sprintf("download speed: %.4f Mbps\n", mbps))
	return sb.String()
}
