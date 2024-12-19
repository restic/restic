//go:build windows
// +build windows

package restic

import (
	"math/rand"
	"os"
	"strconv"
	"sync"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

var (
	testFileName = "TestingAds.txt"
	testFilePath string
	adsFileName  = ":AdsName"
	testData     = "This is the main data stream."
	testDataAds  = "This is an alternate data stream "
	goWG         sync.WaitGroup
	dataSize     int
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func TestAdsFile(t *testing.T) {
	// create a temp test file

	for i := 0; i < 5; i++ {
		dataSize = 10000000 * i
		testData = testData + randStringBytesRmndr(dataSize)
		testDataAds = testDataAds + randStringBytesRmndr(dataSize)
		//Testing with multiple ads streams in sequence.
		testAdsForCount(i, t)
	}

}

func testAdsForCount(adsTestCount int, t *testing.T) {
	makeTestFile(adsTestCount)
	defer os.Remove(testFilePath)

	success, streams, errGA := GetADStreamNames(testFilePath)

	rtest.Assert(t, success, "GetADStreamNames status. error: %v", errGA)
	rtest.Assert(t, len(streams) == adsTestCount, "Stream found: %v", streams)

	adsCount := len(streams)

	goWG.Add(1)

	go ReadMain(t)

	goWG.Add(adsCount)
	for i := 0; i < adsCount; i++ {
		//Writing ADS to the file concurrently
		go ReadAds(i, t)
	}
	goWG.Wait()
	os.Remove(testFilePath)
}

func ReadMain(t *testing.T) {
	defer goWG.Done()
	data, errR := os.ReadFile(testFilePath)
	rtest.OK(t, errR)
	dataString := string(data)
	rtest.Assert(t, dataString == testData, "Data read: %v", len(dataString))
}

func ReadAds(i int, t *testing.T) {
	defer goWG.Done()
	dataAds, errAds := os.ReadFile(testFilePath + adsFileName + strconv.Itoa(i))
	rtest.OK(t, errAds)

	rtest.Assert(t, errAds == nil, "GetADStreamNames status. error: %v", errAds)
	dataStringAds := string(dataAds)
	rtest.Assert(t, dataStringAds == testDataAds+strconv.Itoa(i)+".\n", "Ads Data read: %v", len(dataStringAds))
}

func makeTestFile(adsCount int) error {
	f, err := os.CreateTemp("", testFileName)
	if err != nil {
		panic(err)
	}
	testFilePath = f.Name()

	defer f.Close()
	if adsCount == 0 || adsCount == 1 {
		goWG.Add(1)
		//Writing main file
		go WriteMain(err, f)
	}

	goWG.Add(adsCount)
	for i := 0; i < adsCount; i++ {
		//Writing ADS to the file concurrently while main file also gets written
		go WriteADS(i)
		if i == 1 {
			//Testing some cases where the main file writing may start after the ads streams writing has started.
			//These cases are tested when adsCount > 1. In this case we start writing the main file after starting to write ads.
			goWG.Add(1)
			go WriteMain(err, f)
		}
	}
	goWG.Wait()
	return nil
}

func WriteMain(err error, f *os.File) (bool, error) {
	defer goWG.Done()

	_, err1 := f.Write([]byte(testData))
	if err1 != nil {
		return true, err
	}
	return false, err
}

func WriteADS(i int) (bool, error) {
	defer goWG.Done()
	a, err := os.Create(testFilePath + adsFileName + strconv.Itoa(i))
	if err != nil {
		return true, err
	}
	defer a.Close()

	_, err = a.Write([]byte(testDataAds + strconv.Itoa(i) + ".\n"))
	if err != nil {
		return true, err
	}
	return false, nil
}

func randStringBytesRmndr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}

func TestTrimAds(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{input: "d:\\test.txt:stream1:$DATA", output: "d:\\test.txt"},
		{input: "test.txt:stream1:$DATA", output: "test.txt"},
		{input: "test.txt", output: "test.txt"},
		{input: "\\abc\\test.txt:stream1:$DATA", output: "\\abc\\test.txt"},
		{input: "\\abc\\", output: "\\abc\\"},
		{input: "\\", output: "\\"},
	}

	for _, test := range tests {

		t.Run("", func(t *testing.T) {
			output := TrimAds(test.input)
			rtest.Equals(t, test.output, output)
		})
	}
}

func TestIsAds(t *testing.T) {
	tests := []struct {
		input  string
		result bool
	}{
		{input: "d:\\test.txt:stream1:$DATA", result: true},
		{input: "test.txt:stream1:$DATA", result: true},
		{input: "test.txt", result: false},
		{input: "\\abc\\test.txt:stream1:$DATA", result: true},
		{input: "\\abc\\", result: false},
		{input: "\\", result: false},
	}

	for _, test := range tests {

		t.Run("", func(t *testing.T) {
			output := IsAds(test.input)
			rtest.Equals(t, test.result, output)
		})
	}
}
