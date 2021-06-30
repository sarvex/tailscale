// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package speedtest contains both server and client code for
// running speedtests between tailscale nodes.
package speedtest

import (
	"time"
)

const (
	lenBufData      = 32000           // Size of the block of randomly generated data to send.
	MinDuration     = 5 * time.Second // Default time for a test
	DefaultDuration = MinDuration
	MaxDuration     = 30 * time.Second
	version         = 1
	increment       = 1.0 // increment to display results for, in seconds
)

// This is the initial message sent to the server, that contains information on how to
// conduct the test.
type TestConfig struct {
	Version      int           `json:"version"`
	TestDuration time.Duration `json:"time"`
}

// This is the response to the TestConfig message. It contains an error that the server
// has with the TestConfig.
type TestConfigResponse struct {
	Error string `json:"error,omitempty"`
}

// This represents the Result of a speedtest within a specific interval
type Result struct {
	Bytes    int           // number of bytes sent/received during the interval
	Interval time.Duration // duration of the interval
	Total    bool          // if true, this result struct represents the entire test, rather than a segment of the test.
}

func (r Result) BytesPerSecond() float64 {
	return float64(r.Bytes) / r.Interval.Seconds()
}

func (r Result) BitsPerSecond() float64 {
	return float64(r.Bytes) * 8.0 / r.Interval.Seconds()
}

// getResult returns a pointer to a result struct created using the parameters,
// only if the interval is greater than 0.01 seconds.
func getResult(sum int, interval time.Duration, total bool) *Result {
	//return early if it's not worth displaying the data
	if interval.Seconds() < 0.01 {
		return nil
	}
	r := &Result{}
	r.Bytes = sum
	r.Interval = interval
	r.Total = total
	return r
}

// TestState is used by the server when checking the result of a test.
type TestState struct {
	failed  bool
	err     error
	results []Result
}
