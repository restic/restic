package s3

import (
	"net/http"

	"gopkg.in/amz.v3/aws"
)

var originalStrategy = attempts

func BuildError(resp *http.Response) error {
	return buildError(resp)
}

func SetAttemptStrategy(s *aws.AttemptStrategy) {
	if s == nil {
		attempts = originalStrategy
	} else {
		attempts = *s
	}
}

func AttemptStrategy() aws.AttemptStrategy {
	return attempts
}

func SetListPartsMax(n int) {
	listPartsMax = n
}

func SetListMultiMax(n int) {
	listMultiMax = n
}
