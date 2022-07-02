package options_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/test"
)

type secretTest struct {
	str options.SecretString
}

func assertNotIn(t *testing.T, str string, substr string) {
	if strings.Contains(str, substr) {
		t.Fatalf("'%s' should not contain '%s'", str, substr)
	}
}

func TestSecretString(t *testing.T) {
	keyStr := "secret-key"
	secret := options.NewSecretString(keyStr)

	test.Equals(t, "**redacted**", secret.String())
	test.Equals(t, `"**redacted**"`, secret.GoString())
	test.Equals(t, "**redacted**", fmt.Sprint(secret))
	test.Equals(t, "**redacted**", fmt.Sprintf("%v", secret))
	test.Equals(t, `"**redacted**"`, fmt.Sprintf("%#v", secret))
	test.Equals(t, keyStr, secret.Unwrap())
}

func TestSecretStringStruct(t *testing.T) {
	keyStr := "secret-key"
	secretStruct := &secretTest{
		str: options.NewSecretString(keyStr),
	}

	assertNotIn(t, fmt.Sprint(secretStruct), keyStr)
	assertNotIn(t, fmt.Sprintf("%v", secretStruct), keyStr)
	assertNotIn(t, fmt.Sprintf("%#v", secretStruct), keyStr)
}

func TestSecretStringEmpty(t *testing.T) {
	keyStr := ""
	secret := options.NewSecretString(keyStr)

	test.Equals(t, "", secret.String())
	test.Equals(t, `""`, secret.GoString())
	test.Equals(t, "", fmt.Sprint(secret))
	test.Equals(t, "", fmt.Sprintf("%v", secret))
	test.Equals(t, `""`, fmt.Sprintf("%#v", secret))
	test.Equals(t, keyStr, secret.Unwrap())
}

func TestSecretStringDefault(t *testing.T) {
	secretStruct := &secretTest{}

	test.Equals(t, "", secretStruct.str.String())
	test.Equals(t, `""`, secretStruct.str.GoString())
	test.Equals(t, "", secretStruct.str.Unwrap())
}
