package main

import (
	"math"
	"reflect"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestParsePercentage(t *testing.T) {
	testCases := []struct {
		input       string
		output      float64
		expectError bool
	}{
		{"0%", 0.0, false},
		{"1%", 1.0, false},
		{"100%", 100.0, false},
		{"123%", 123.0, false},
		{"123.456%", 123.456, false},
		{"0.742%", 0.742, false},
		{"-100%", -100.0, false},
		{" 1%", 0.0, true},
		{"1 %", 0.0, true},
		{"1% ", 0.0, true},
	}
	for _, testCase := range testCases {
		output, err := parsePercentage(testCase.input)

		if testCase.expectError {
			rtest.Assert(t, err != nil, "Expected error for case %s", testCase.input)
			rtest.Assert(t, output == 0.0, "Expected output to be 0.0, got %s", output)
		} else {
			rtest.Assert(t, err == nil, "Expected no error for case %s", testCase.input)
			rtest.Assert(t, math.Abs(testCase.output-output) < 0.00001, "Expected %f, got %f",
				testCase.output, output)
		}
	}
}

func TestStringToIntSlice(t *testing.T) {
	testCases := []struct {
		input       string
		output      []uint
		expectError bool
	}{
		{"3/5", []uint{3, 5}, false},
		{"1/100", []uint{1, 100}, false},
		{"abc", nil, true},
		{"1/a", nil, true},
		{"/", nil, true},
	}
	for _, testCase := range testCases {
		output, err := stringToIntSlice(testCase.input)

		if testCase.expectError {
			rtest.Assert(t, err != nil, "Expected error for case %s", testCase.input)
			rtest.Assert(t, output == nil, "Expected output to be nil, got %s", output)
		} else {
			rtest.Assert(t, err == nil, "Expected no error for case %s", testCase.input)
			rtest.Assert(t, len(output) == 2, "Invalid output length for case %s", testCase.input)
			rtest.Assert(t, reflect.DeepEqual(output, testCase.output), "Expected %f, got %f",
				testCase.output, output)
		}
	}
}

func TestSelectPacksByBucket(t *testing.T) {
	var testPacks = make(map[restic.ID]int64)
	for i := 1; i <= 10; i++ {
		id := restic.NewRandomID()
		// ensure relevant part of generated id is reproducable
		id[0] = byte(i)
		testPacks[id] = 0
	}

	selectedPacks := selectPacksByBucket(testPacks, 0, 10)
	rtest.Assert(t, len(selectedPacks) == 0, "Expected 0 selected packs")

	for i := uint(1); i <= 5; i++ {
		selectedPacks = selectPacksByBucket(testPacks, i, 5)
		rtest.Assert(t, len(selectedPacks) == 2, "Expected 2 selected packs")
	}

	selectedPacks = selectPacksByBucket(testPacks, 1, 1)
	rtest.Assert(t, len(selectedPacks) == 10, "Expected 10 selected packs")
	for testPack := range testPacks {
		_, ok := selectedPacks[testPack]
		rtest.Assert(t, ok, "Expected input and output to be equal")
	}
}

func TestSelectRandomPacksByPercentage(t *testing.T) {
	var testPacks = make(map[restic.ID]int64)
	for i := 1; i <= 10; i++ {
		testPacks[restic.NewRandomID()] = 0
	}

	selectedPacks := selectRandomPacksByPercentage(testPacks, 0.0)
	rtest.Assert(t, len(selectedPacks) == 1, "Expected 1 selected packs")

	selectedPacks = selectRandomPacksByPercentage(testPacks, 10.0)
	rtest.Assert(t, len(selectedPacks) == 1, "Expected 1 selected pack")
	for pack := range selectedPacks {
		_, ok := testPacks[pack]
		rtest.Assert(t, ok, "Unexpected selection")
	}

	selectedPacks = selectRandomPacksByPercentage(testPacks, 50.0)
	rtest.Assert(t, len(selectedPacks) == 5, "Expected 5 selected packs")
	for pack := range selectedPacks {
		_, ok := testPacks[pack]
		rtest.Assert(t, ok, "Unexpected item in selection")
	}

	selectedPacks = selectRandomPacksByPercentage(testPacks, 100.0)
	rtest.Assert(t, len(selectedPacks) == 10, "Expected 10 selected packs")
	for testPack := range testPacks {
		_, ok := selectedPacks[testPack]
		rtest.Assert(t, ok, "Expected input and output to be equal")
	}
}

func TestSelectNoRandomPacksByPercentage(t *testing.T) {
	// that the a repository without pack files works
	var testPacks = make(map[restic.ID]int64)
	selectedPacks := selectRandomPacksByPercentage(testPacks, 10.0)
	rtest.Assert(t, len(selectedPacks) == 0, "Expected 0 selected packs")
}

func TestSelectRandomPacksByFileSize(t *testing.T) {
	var testPacks = make(map[restic.ID]int64)
	for i := 1; i <= 10; i++ {
		id := restic.NewRandomID()
		// ensure unique ids
		id[0] = byte(i)
		testPacks[id] = 0
	}

	selectedPacks := selectRandomPacksByFileSize(testPacks, 10, 500)
	rtest.Assert(t, len(selectedPacks) == 1, "Expected 1 selected packs")

	selectedPacks = selectRandomPacksByFileSize(testPacks, 10240, 51200)
	rtest.Assert(t, len(selectedPacks) == 2, "Expected 2 selected packs")
	for pack := range selectedPacks {
		_, ok := testPacks[pack]
		rtest.Assert(t, ok, "Unexpected selection")
	}

	selectedPacks = selectRandomPacksByFileSize(testPacks, 500, 500)
	rtest.Assert(t, len(selectedPacks) == 10, "Expected 10 selected packs")
	for pack := range selectedPacks {
		_, ok := testPacks[pack]
		rtest.Assert(t, ok, "Unexpected item in selection")
	}
}

func TestSelectNoRandomPacksByFileSize(t *testing.T) {
	// that the a repository without pack files works
	var testPacks = make(map[restic.ID]int64)
	selectedPacks := selectRandomPacksByFileSize(testPacks, 10, 500)
	rtest.Assert(t, len(selectedPacks) == 0, "Expected 0 selected packs")
}
