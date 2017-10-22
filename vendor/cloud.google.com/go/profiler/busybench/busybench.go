// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"cloud.google.com/go/profiler"
	"compress/gzip"
	"flag"
	"log"
	"math/rand"
	"time"
)

var service = flag.String("service", "", "service name")

const duration = time.Minute * 10

// busywork continuously generates 1MiB of random data and compresses it
// throwing away the result.
func busywork() {
	ticker := time.NewTicker(duration)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			return
		default:
			busyworkOnce()
		}
	}
}

func busyworkOnce() {
	data := make([]byte, 1024*1024)
	rand.Read(data)

	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(data); err != nil {
		log.Printf("Failed to write to gzip stream: %v", err)
		return
	}
	if err := gz.Flush(); err != nil {
		log.Printf("Failed to flush to gzip stream: %v", err)
		return
	}
	if err := gz.Close(); err != nil {
		log.Printf("Failed to close gzip stream: %v", err)
	}
	// Throw away the result.
}

func main() {
	flag.Parse()

	if *service == "" {
		log.Print("Service name must be configured using --service flag.")
	} else if err := profiler.Start(profiler.Config{Service: *service, DebugLogging: true}); err != nil {
		log.Printf("Failed to start the profiler: %v", err)
	} else {
		busywork()
	}

	log.Printf("busybench finished profiling.")
	select {}
}
