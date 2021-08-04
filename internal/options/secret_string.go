package options

type SecretString struct {
	s *string
}

func NewSecretString(s string) SecretString {
	return SecretString{s: &s}
}

func (s SecretString) GoString() string {
	return `"` + s.String() + `"`
}

func (s SecretString) String() string {
	if len(*s.s) == 0 {
		return ``
	}
	return `**redacted**`
}

func (s *SecretString) Unwrap() string {
	return *s.s
}
