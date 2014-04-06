package hashing_test

import (
	"testing"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestHashing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hashing Suite")
}
