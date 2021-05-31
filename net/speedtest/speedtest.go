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
	LenBufData    = 32000 // Size of the block of randomly generated data to send.
	MinNumSeconds = 5     // Default time for a test
	MaxNumSeconds = 30
	version       = 1
	increment     = 1.0 // increment to display results for, in seconds
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
	Bytes    int
	Interval time.Duration
	Total    bool
}

func (r Result) BytesPerSecond() float64 {
	return float64(r.Bytes) / r.Interval.Seconds()
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
