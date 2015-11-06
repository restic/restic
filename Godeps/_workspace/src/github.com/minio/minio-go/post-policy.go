package minio

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// expirationDateFormat date format for expiration key in json policy
const expirationDateFormat = "2006-01-02T15:04:05.999Z"

// Policy explanation: http://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-HTTPPOSTConstructPolicy.html
type policy struct {
	matchType string
	key       string
	value     string
}

// PostPolicy provides strict static type conversion and validation for Amazon S3's POST policy JSON string.
type PostPolicy struct {
	expiration         time.Time // expiration date and time of the POST policy.
	policies           []policy
	contentLengthRange struct {
		min int
		max int
	}

	// Post form data
	formData map[string]string
}

// NewPostPolicy instantiate new post policy
func NewPostPolicy() *PostPolicy {
	p := &PostPolicy{}
	p.policies = make([]policy, 0)
	p.formData = make(map[string]string)
	return p
}

// SetExpires expiration time
func (p *PostPolicy) SetExpires(t time.Time) error {
	if t.IsZero() {
		return errors.New("time input invalid")
	}
	p.expiration = t
	return nil
}

// SetKey Object name
func (p *PostPolicy) SetKey(key string) error {
	if strings.TrimSpace(key) == "" || key == "" {
		return errors.New("key invalid")
	}
	policy := policy{"eq", "$key", key}
	p.policies = append(p.policies, policy)
	p.formData["key"] = key
	return nil
}

// SetKeyStartsWith Object name that can start with
func (p *PostPolicy) SetKeyStartsWith(keyStartsWith string) error {
	if strings.TrimSpace(keyStartsWith) == "" || keyStartsWith == "" {
		return errors.New("key-starts-with invalid")
	}
	policy := policy{"starts-with", "$key", keyStartsWith}
	p.policies = append(p.policies, policy)
	p.formData["key"] = keyStartsWith
	return nil
}

// SetBucket bucket name
func (p *PostPolicy) SetBucket(bucket string) error {
	if strings.TrimSpace(bucket) == "" || bucket == "" {
		return errors.New("bucket invalid")
	}
	policy := policy{"eq", "$bucket", bucket}
	p.policies = append(p.policies, policy)
	p.formData["bucket"] = bucket
	return nil
}

// SetContentType content-type
func (p *PostPolicy) SetContentType(contentType string) error {
	if strings.TrimSpace(contentType) == "" || contentType == "" {
		return errors.New("contentType invalid")
	}
	policy := policy{"eq", "$Content-Type", contentType}
	if err := p.addNewPolicy(policy); err != nil {
		return err
	}
	p.formData["Content-Type"] = contentType
	return nil
}

// SetContentLength - set new min and max content legnth condition
func (p *PostPolicy) SetContentLength(min, max int) error {
	if min > max {
		return errors.New("minimum cannot be bigger than maximum")
	}
	if min < 0 {
		return errors.New("minimum cannot be negative")
	}
	if max < 0 {
		return errors.New("maximum cannot be negative")
	}
	p.contentLengthRange.min = min
	p.contentLengthRange.max = max
	return nil
}

// addNewPolicy - internal helper to validate adding new policies
func (p *PostPolicy) addNewPolicy(po policy) error {
	if po.matchType == "" || po.key == "" || po.value == "" {
		return errors.New("policy invalid")
	}
	p.policies = append(p.policies, po)
	return nil
}

// Stringer interface for printing in pretty manner
func (p PostPolicy) String() string {
	return string(p.marshalJSON())
}

// marshalJSON provides Marshalled JSON
func (p PostPolicy) marshalJSON() []byte {
	expirationStr := `"expiration":"` + p.expiration.Format(expirationDateFormat) + `"`
	var policiesStr string
	policies := []string{}
	for _, po := range p.policies {
		policies = append(policies, fmt.Sprintf("[\"%s\",\"%s\",\"%s\"]", po.matchType, po.key, po.value))
	}
	if p.contentLengthRange.min != 0 || p.contentLengthRange.max != 0 {
		policies = append(policies, fmt.Sprintf("[\"content-length-range\", %d, %d]",
			p.contentLengthRange.min, p.contentLengthRange.max))
	}
	if len(policies) > 0 {
		policiesStr = `"conditions":[` + strings.Join(policies, ",") + "]"
	}
	retStr := "{"
	retStr = retStr + expirationStr + ","
	retStr = retStr + policiesStr
	retStr = retStr + "}"
	return []byte(retStr)
}

// base64 produces base64 of PostPolicy's Marshalled json
func (p PostPolicy) base64() string {
	return base64.StdEncoding.EncodeToString(p.marshalJSON())
}
