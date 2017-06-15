/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

/*
Package grpclog defines logging for grpc.
*/
package grpclog // import "google.golang.org/grpc/grpclog"

import (
	"log"
	"os"
)

// Use golang's standard logger by default.
// Access is not mutex-protected: do not modify except in init()
// functions.
var logger Logger = log.New(os.Stderr, "", log.LstdFlags)

// Logger mimics golang's standard Logger as an interface.
type Logger interface {
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Fatalln(args ...interface{})
	Print(args ...interface{})
	Printf(format string, args ...interface{})
	Println(args ...interface{})
}

// SetLogger sets the logger that is used in grpc. Call only from
// init() functions.
func SetLogger(l Logger) {
	logger = l
}

// Fatal is equivalent to Print() followed by a call to os.Exit() with a non-zero exit code.
func Fatal(args ...interface{}) {
	logger.Fatal(args...)
}

// Fatalf is equivalent to Printf() followed by a call to os.Exit() with a non-zero exit code.
func Fatalf(format string, args ...interface{}) {
	logger.Fatalf(format, args...)
}

// Fatalln is equivalent to Println() followed by a call to os.Exit()) with a non-zero exit code.
func Fatalln(args ...interface{}) {
	logger.Fatalln(args...)
}

// Print prints to the logger. Arguments are handled in the manner of fmt.Print.
func Print(args ...interface{}) {
	logger.Print(args...)
}

// Printf prints to the logger. Arguments are handled in the manner of fmt.Printf.
func Printf(format string, args ...interface{}) {
	logger.Printf(format, args...)
}

// Println prints to the logger. Arguments are handled in the manner of fmt.Println.
func Println(args ...interface{}) {
	logger.Println(args...)
}
