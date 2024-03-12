package feature_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/restic/restic/internal/feature"
	rtest "github.com/restic/restic/internal/test"
)

var (
	alpha      = feature.FlagName("alpha-feature")
	beta       = feature.FlagName("beta-feature")
	stable     = feature.FlagName("stable-feature")
	deprecated = feature.FlagName("deprecated-feature")
)

var testFlags = map[feature.FlagName]feature.FlagDesc{
	alpha: {
		Type:        feature.Alpha,
		Description: "alpha",
	},
	beta: {
		Type:        feature.Beta,
		Description: "beta",
	},
	stable: {
		Type:        feature.Stable,
		Description: "stable",
	},
	deprecated: {
		Type:        feature.Deprecated,
		Description: "deprecated",
	},
}

func buildTestFlagSet() *feature.FlagSet {
	flags := feature.New()
	flags.SetFlags(testFlags)
	return flags
}

func TestFeatureDefaults(t *testing.T) {
	flags := buildTestFlagSet()
	for _, exp := range []struct {
		flag  feature.FlagName
		value bool
	}{
		{alpha, false},
		{beta, true},
		{stable, true},
		{deprecated, false},
	} {
		rtest.Assert(t, flags.Enabled(exp.flag) == exp.value, "expected flag %v to have value %v got %v", exp.flag, exp.value, flags.Enabled(exp.flag))
	}
}

func panicIfCalled(msg string) {
	panic(msg)
}

func TestEmptyApply(t *testing.T) {
	flags := buildTestFlagSet()
	rtest.OK(t, flags.Apply("", panicIfCalled))

	rtest.Assert(t, !flags.Enabled(alpha), "expected alpha feature to be disabled")
	rtest.Assert(t, flags.Enabled(beta), "expected beta feature to be enabled")
}

func TestFeatureApply(t *testing.T) {
	flags := buildTestFlagSet()
	rtest.OK(t, flags.Apply(string(alpha), panicIfCalled))
	rtest.Assert(t, flags.Enabled(alpha), "expected alpha feature to be enabled")

	rtest.OK(t, flags.Apply(fmt.Sprintf("%s=false", alpha), panicIfCalled))
	rtest.Assert(t, !flags.Enabled(alpha), "expected alpha feature to be disabled")

	rtest.OK(t, flags.Apply(fmt.Sprintf("%s=true", alpha), panicIfCalled))
	rtest.Assert(t, flags.Enabled(alpha), "expected alpha feature to be enabled again")

	rtest.OK(t, flags.Apply(fmt.Sprintf("%s=false", beta), panicIfCalled))
	rtest.Assert(t, !flags.Enabled(beta), "expected beta feature to be disabled")

	logMsg := ""
	log := func(msg string) {
		logMsg = msg
	}

	rtest.OK(t, flags.Apply(fmt.Sprintf("%s=false", stable), log))
	rtest.Assert(t, flags.Enabled(stable), "expected stable feature to remain enabled")
	rtest.Assert(t, strings.Contains(logMsg, string(stable)), "unexpected log message for stable flag: %v", logMsg)

	logMsg = ""
	rtest.OK(t, flags.Apply(fmt.Sprintf("%s=true", deprecated), log))
	rtest.Assert(t, !flags.Enabled(deprecated), "expected deprecated feature to remain disabled")
	rtest.Assert(t, strings.Contains(logMsg, string(deprecated)), "unexpected log message for deprecated flag: %v", logMsg)
}

func TestFeatureMultipleApply(t *testing.T) {
	flags := buildTestFlagSet()

	rtest.OK(t, flags.Apply(fmt.Sprintf("%s=true,%s=false", alpha, beta), panicIfCalled))
	rtest.Assert(t, flags.Enabled(alpha), "expected alpha feature to be enabled")
	rtest.Assert(t, !flags.Enabled(beta), "expected beta feature to be disabled")
}

func TestFeatureApplyInvalid(t *testing.T) {
	flags := buildTestFlagSet()

	err := flags.Apply("invalid-flag", panicIfCalled)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "unknown feature flag"), "expected unknown feature flag error, got: %v", err)

	err = flags.Apply(fmt.Sprintf("%v=invalid", alpha), panicIfCalled)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "failed to parse value"), "expected parsing error, got: %v", err)
}

func assertPanic(t *testing.T) {
	if r := recover(); r == nil {
		t.Fatal("should have panicked")
	}
}

func TestFeatureQueryInvalid(t *testing.T) {
	defer assertPanic(t)

	flags := buildTestFlagSet()
	flags.Enabled("invalid-flag")
}

func TestFeatureSetInvalidPhase(t *testing.T) {
	defer assertPanic(t)

	flags := feature.New()
	flags.SetFlags(map[feature.FlagName]feature.FlagDesc{
		"invalid": {
			Type: "invalid",
		},
	})
}

func TestFeatureList(t *testing.T) {
	flags := buildTestFlagSet()

	rtest.Equals(t, []feature.Help{
		{string(alpha), string(feature.Alpha), false, "alpha"},
		{string(beta), string(feature.Beta), true, "beta"},
		{string(deprecated), string(feature.Deprecated), false, "deprecated"},
		{string(stable), string(feature.Stable), true, "stable"},
	}, flags.List())
}
