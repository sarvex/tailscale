// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package speedtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// RunClient dials the given address and starts a speedtest.
// It returns any errors that come up in the tests.
// It returns an error if the given test type isn't either download or upload.
// If there are no errors in the test, it returns a slice of results.
func RunClient(duration time.Duration, host string) ([]Result, error) {
	conn, err := net.Dial("tcp", host)
	if err != nil {
		return nil, err
	}

	config := testConfig{TestDuration: duration, Version: version}

	defer conn.Close()
	encoder := json.NewEncoder(conn)

	if err = encoder.Encode(config); err != nil {
		return nil, err
	}
	var response testConfigResponse
	decoder := json.NewDecoder(conn)
	if err = decoder.Decode(&response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	return runTestC(conn, config)
}

// runTestC handles the entire download speed test.
// It has a loop that breaks if the connection recieves an IO error or if the server sends a header
// with the "end" type. It reads the headers and data coming from the server and records the number of bytes recieved in each interval in a result slice.
func runTestC(conn net.Conn, config testConfig) ([]Result, error) {
	bufferData := make([]byte, blockSize)

	sum := 0
	totalSum := 0

	var lastCalculated time.Time
	var downloadBegin time.Time
	var currentTime time.Time
	var results []Result

	for {
		n, err := io.ReadFull(conn, bufferData)
		if downloadBegin.IsZero() {
			downloadBegin = time.Now()
			lastCalculated = time.Now()
		}

		currentTime = time.Now()
		sum = sum + n
		if err != nil {
			//worst case scenario: the server closes the connection and the client quits
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				// none/some bytes read, then EOF
				break
			} else {
				return nil, fmt.Errorf("unexpected error has occured: %w", err)
			}
		}

		// checks if the current time is more or equal to the lastCalculated time plus the increment
		if currentTime.After(lastCalculated.Add(time.Second * time.Duration(increment))) {
			interval := currentTime.Sub(lastCalculated)
			if interval > minInterval {
				results = append(results, Result{Bytes: sum, Interval: interval, Total: false})
			}
			lastCalculated = currentTime
			totalSum += sum
			sum = 0
		}

	}

	// get last segment
	interval := currentTime.Sub(lastCalculated)
	if interval > minInterval {
		results = append(results, Result{Bytes: sum, Interval: interval, Total: false})
	}

	// get total
	totalSum += sum
	interval = currentTime.Sub(downloadBegin)
	if interval > minInterval {
		results = append(results, Result{Bytes: totalSum, Interval: interval, Total: true})
	}

	return results, nil

}
