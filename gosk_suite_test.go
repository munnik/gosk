package main_test

import (
	"testing"

	"github.com/fgrosse/zaptest"
	"github.com/munnik/gosk/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGosk(t *testing.T) {
	logger.SetLogger(zaptest.Logger(t))
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gosk Suite")
}
