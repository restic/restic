package minio

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// expirationDateFormat date format for expiration key in json policy.
const expirationDateFormat = "2006-01-02T15:04:05.999Z"

// policyCondition explanation: http://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-HTTPPOSTConstructPolicy.html
//
// Example:
//
//   policyCondition {
//       matchType: "$eq",
//       key: "$Content-Type",
//       value: "image/png",
//   }
//
type policyCondition struct {
	matchType string
	condition string
	value     string
}

// PostPolicy provides strict static type conversion and validation for Amazon S3's POST policy JSON string.
type PostPolicy struct {
	expiration time.Time         // expiration date and time of the POST policy.
	conditions []policyCondition // collection of different policy conditions.
	// contentLengthRange minimum and maximum allowable size for the uploaded content.
	contentLengthRange struct {
		min int64
		max int64
	}

	// Post form data.
	formData map[string]string
}

// NewPostPolicy instantiate new post policy.
func NewPostPolicy() *PostPolicy {
	p := &PostPolicy{}
	p.conditions = make([]policyCondition, 0)
	p.formData = make(map[string]string)
	return p
}

// SetExpires expiration time.
func (p *PostPolicy) SetExpires(t time.Time) error {
	if t.IsZero() {
		return errors.New("No expiry time set.")
	}
	p.expiration = t
	return nil
}

// SetKey Object name.
func (p *PostPolicy) SetKey(key string) error {
	if strings.TrimSpace(key) == "" || key == "" {
		return errors.New("Object name is not specified.")
	}
	policyCond := policyCondition{
		matchType: "eq",
		condition: "$key",
		value:     key,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData["key"] = key
	return nil
}

// SetKeyStartsWith Object name that can start with.
func (p *PostPolicy) SetKeyStartsWith(keyStartsWith string) error {
	if strings.TrimSpace(keyStartsWith) == "" || keyStartsWith == "" {
		return errors.New("Object prefix is not specified.")
	}
	policyCond := policyCondition{
		matchType: "starts-with",
		condition: "$key",
		value:     keyStartsWith,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData["key"] = keyStartsWith
	return nil
}

// SetBucket bucket name.
func (p *PostPolicy) SetBucket(bucketName string) error {
	if strings.TrimSpace(bucketName) == "" || bucketName == "" {
		return errors.New("Bucket name is not specified.")
	}
	policyCond := policyCondition{
		matchType: "eq",
		condition: "$bucket",
		value:     bucketName,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData["bucket"] = bucketName
	return nil
}

// SetContentType content-type.
func (p *PostPolicy) SetContentType(contentType string) error {
	if strings.TrimSpace(contentType) == "" || contentType == "" {
		return errors.New("No content type specified.")
	}
	policyCond := policyCondition{
		matchType: "eq",
		condition: "$Content-Type",
		value:     contentType,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData["Content-Type"] = contentType
	return nil
}

// SetContentLengthRange - set new min and max content length condition.
func (p *PostPolicy) SetContentLengthRange(min, max int64) error {
	if min > max {
		return errors.New("minimum limit is larger than maximum limit")
	}
	if min < 0 {
		return errors.New("minimum limit cannot be negative")
	}
	if max < 0 {
		return errors.New("maximum limit cannot be negative")
	}
	p.contentLengthRange.min = min
	p.contentLengthRange.max = max
	return nil
}

// addNewPolicy - internal helper to validate adding new policies.
func (p *PostPolicy) addNewPolicy(policyCond policyCondition) error {
	if policyCond.matchType == "" || policyCond.condition == "" || policyCond.value == "" {
		return errors.New("Policy fields empty.")
	}
	p.conditions = append(p.conditions, policyCond)
	return nil
}

// Stringer interface for printing in pretty manner.
func (p PostPolicy) String() string {
	return string(p.marshalJSON())
}

// marshalJSON provides Marshalled JSON.
func (p PostPolicy) marshalJSON() []byte {
	expirationStr := `"expiration":"` + p.expiration.Format(expirationDateFormat) + `"`
	var conditionsStr string
	conditions := []string{}
	for _, po := range p.conditions {
		conditions = append(conditions, fmt.Sprintf("[\"%s\",\"%s\",\"%s\"]", po.matchType, po.condition, po.value))
	}
	if p.contentLengthRange.min != 0 || p.contentLengthRange.max != 0 {
		conditions = append(conditions, fmt.Sprintf("[\"content-length-range\", %d, %d]",
			p.contentLengthRange.min, p.contentLengthRange.max))
	}
	if len(conditions) > 0 {
		conditionsStr = `"conditions":[` + strings.Join(conditions, ",") + "]"
	}
	retStr := "{"
	retStr = retStr + expirationStr + ","
	retStr = retStr + conditionsStr
	retStr = retStr + "}"
	return []byte(retStr)
}

// base64 produces base64 of PostPolicy's Marshalled json.
func (p PostPolicy) base64() string {
	return base64.StdEncoding.EncodeToString(p.marshalJSON())
}
