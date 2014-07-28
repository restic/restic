package khepri_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)

func TestHashing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hashing Suite")
}
