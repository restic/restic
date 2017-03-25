package stow

import (
	"testing"
	. "restic/test"
)

// To run test: gb test -v restic/backend/stow -run ^TestGetAWSRegion$
func TestGetAWSRegion(t *testing.T) {
	m := map[string][]string{
		"us-east-1": []string{
			"s3.amazonaws.com",
			"s3-external-1.amazonaws.com",
			"s3.dualstack.us-east-1.amazonaws.com**",
		},
		"us-east-2": []string{
			"s3.us-east-2.amazonaws.com",
			"s3-us-east-2.amazonaws.com",
			"s3.dualstack.us-east-2.amazonaws.com",
		},
		"us-west-1": {
			"s3-us-west-1.amazonaws.com",
			"s3.dualstack.us-west-1.amazonaws.com**",
		},
		"us-west-2": {
			"s3-us-west-2.amazonaws.com",
			"s3.dualstack.us-west-2.amazonaws.com**",
		},
		"ca-central-1": []string{
			"s3.ca-central-1.amazonaws.com",
			"s3-ca-central-1.amazonaws.com",
			"s3.dualstack.ca-central-1.amazonaws.com**",
		},
		"ap-south-1": []string{
			"s3.ap-south-1.amazonaws.com",
			"s3-ap-south-1.amazonaws.com",
			"s3.dualstack.ap-south-1.amazonaws.com**",
		},
		"ap-northeast-2": []string{
			"s3.ap-northeast-2.amazonaws.com",
			"s3-ap-northeast-2.amazonaws.com",
			"s3.dualstack.ap-northeast-2.amazonaws.com**",
		},
		"ap-southeast-1": []string{
			"s3-ap-southeast-1.amazonaws.com",
			"s3.dualstack.ap-southeast-1.amazonaws.com**",
		},
		"ap-southeast-2": []string{
			"s3-ap-southeast-2.amazonaws.com",
			"s3.dualstack.ap-southeast-2.amazonaws.com**",
		},
		"ap-northeast-1": []string{
			"s3-ap-northeast-1.amazonaws.com",
			"s3.dualstack.ap-northeast-1.amazonaws.com**",
		},
		"eu-central-1": []string{
			"s3.eu-central-1.amazonaws.com",
			"s3-eu-central-1.amazonaws.com",
			"s3.dualstack.eu-central-1.amazonaws.com**",
		},
		"eu-west-1": []string{
			"s3-eu-west-1.amazonaws.com",
			"s3.dualstack.eu-west-1.amazonaws.com**",
		},
		"eu-west-2": []string{
			"s3.eu-west-2.amazonaws.com",
			"s3-eu-west-2.amazonaws.com",
			"s3.dualstack.eu-west-2.amazonaws.com**",
		},
		"sa-east-1": []string{
			"s3-sa-east-1.amazonaws.com",
			"s3.dualstack.sa-east-1.amazonaws.com**",
		},
	}
	for region, endpoints := range m {
		for _, endpoint := range endpoints {
			Equals(t, region, getAWSRegion(endpoint))
		}
	}
}