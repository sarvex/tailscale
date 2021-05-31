// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package speedtest

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Serve starts up the server on a given host and port pair. It starts to listen for
// connections and handles each one in a goroutine. Because it runs in an infinite loop,
// this function only returns if any of the speedtests return with errors, or if a bool is sent
// to the killSignal channel.
func Serve(l *net.TCPListener, maxConnections int, killSignal chan bool, resultsChan chan []Result) error {
	defer l.Close()

	numConnections := 0
	testStateChan := make(chan TestState, maxConnections)
	connChan := make(chan *net.TCPConn, maxConnections)

	// This goroutine runs in an infinite loop and returns if there is an error with the listener
	// or if it is closed. The listener is closed when StartServer returns via a defer statement.
	go (func() {
		for {
			conn, err := l.AcceptTCP()
			if err != nil {
				// The AcceptTCP will return an error if the listener is closed.
				return
			}
			if numConnections >= maxConnections {
				conn.Close()
				continue
			}
			connChan <- conn
		}
	})()

	for {
		select {
		case <-killSignal:
			return nil
		case conn := <-connChan:
			// Handle the connection in a goroutine.
			go handleConnection(conn, testStateChan)
			numConnections++
		case state := <-testStateChan:
			if state.failed {
				return state.err
			} else {
				// send the results to the results channel to be displayed
				if resultsChan != nil {
					resultsChan <- state.results
				}
			}
			numConnections--
		}
	}
}

// handleConnection handles the initial exchange between the server and the client.
// It reads the testconfig message into a TestConfig struct. If any errors occur with
// the testconfig (specifically, if there is a version mismatch), it will return those
// errors to the client with a TestConfigResponse. After the exchange, it will start
// the speed test.
func handleConnection(conn *net.TCPConn, testStateChan chan TestState) {
	defer conn.Close()
	var config TestConfig

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)
	err := decoder.Decode(&config)

	// Both return and encode errors that were thrown before the test has started
	if err != nil {
		encoder.Encode(TestConfigResponse{Error: err.Error()})
		testStateChan <- TestState{failed: true, err: err}
		return
	}
	if config.Version != version {
		err = fmt.Errorf("version mismatch! Server is version %d, client is version %d", version, config.Version)
		encoder.Encode(TestConfigResponse{Error: err.Error()})
		testStateChan <- TestState{failed: true, err: err}
		return
	}

	// Start the test
	encoder.Encode(TestConfigResponse{Error: ""})
	results, err := runTestS(conn, config)

	if err != nil {
		testStateChan <- TestState{failed: true, err: err, results: nil}
		return
	}
	testStateChan <- TestState{failed: false, err: nil, results: results}
}

// runTestS runs the server side of the speed test. For the given amount of time,
// it sends randomly generated data in 32 kilobyte blocks. When time's up the function returns
// and the connection is closed. This function returns an error if the write fails, as well as a
// slice of results that contains the result of the test.
func runTestS(conn *net.TCPConn, config TestConfig) ([]Result, error) {
	conn.SetWriteBuffer(LenBufData)

	BufData := make([]byte, LenBufData)
	sum := 0
	totalSum := 0

	var lastCalculated time.Time
	var currentTime time.Time
	var startTime time.Time
	var results []Result

	for {
		// Randomize data
		_, err := rand.Read(BufData)
		if err != nil {
			continue
		}

		n, err := conn.Write(BufData)
		if err != nil {
			// If the write failed, there is most likely something wrong with the connection.
			return nil, fmt.Errorf("server: connection closed unexpectedly: %w", err)
		}
		if startTime.IsZero() {
			startTime = time.Now()
			lastCalculated = time.Now()
		}

		currentTime = time.Now()
		sum = sum + n
		if currentTime.After(lastCalculated.Add(time.Second * time.Duration(increment))) {
			interval := currentTime.Sub(lastCalculated)
			result := getResult(sum, interval, false)
			if result != nil {
				results = append(results, *result)
			}
			lastCalculated = lastCalculated.Add(time.Duration(increment) * time.Second)
			totalSum += sum
			sum = 0
		}

		if time.Since(startTime) > config.TestDuration {
			break
		}

	}
	result := getResult(totalSum, currentTime.Sub(startTime), true)
	if result != nil {
		results = append(results, *result)
	}
	return results, nil

}
