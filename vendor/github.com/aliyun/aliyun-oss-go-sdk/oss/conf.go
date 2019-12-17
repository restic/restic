package oss

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

// Define the level of the output log
const (
	LogOff = iota
	Error
	Warn
	Info
	Debug
)

// LogTag Tag for each level of log
var LogTag = []string{"[error]", "[warn]", "[info]", "[debug]"}

// HTTPTimeout defines HTTP timeout.
type HTTPTimeout struct {
	ConnectTimeout   time.Duration
	ReadWriteTimeout time.Duration
	HeaderTimeout    time.Duration
	LongTimeout      time.Duration
	IdleConnTimeout  time.Duration
}

// HTTPMaxConns defines max idle connections and max idle connections per host
type HTTPMaxConns struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

// CredentialInf is interface for get AccessKeyID,AccessKeySecret,SecurityToken
type Credentials interface {
	GetAccessKeyID() string
	GetAccessKeySecret() string
	GetSecurityToken() string
}

// CredentialInfBuild is interface for get CredentialInf
type CredentialsProvider interface {
	GetCredentials() Credentials
}

type defaultCredentials struct {
	config *Config
}

func (defCre *defaultCredentials) GetAccessKeyID() string {
	return defCre.config.AccessKeyID
}

func (defCre *defaultCredentials) GetAccessKeySecret() string {
	return defCre.config.AccessKeySecret
}

func (defCre *defaultCredentials) GetSecurityToken() string {
	return defCre.config.SecurityToken
}

type defaultCredentialsProvider struct {
	config *Config
}

func (defBuild *defaultCredentialsProvider) GetCredentials() Credentials {
	return &defaultCredentials{config: defBuild.config}
}

// Config defines oss configuration
type Config struct {
	Endpoint            string              // OSS endpoint
	AccessKeyID         string              // AccessId
	AccessKeySecret     string              // AccessKey
	RetryTimes          uint                // Retry count by default it's 5.
	UserAgent           string              // SDK name/version/system information
	IsDebug             bool                // Enable debug mode. Default is false.
	Timeout             uint                // Timeout in seconds. By default it's 60.
	SecurityToken       string              // STS Token
	IsCname             bool                // If cname is in the endpoint.
	HTTPTimeout         HTTPTimeout         // HTTP timeout
	HTTPMaxConns        HTTPMaxConns        // Http max connections
	IsUseProxy          bool                // Flag of using proxy.
	ProxyHost           string              // Flag of using proxy host.
	IsAuthProxy         bool                // Flag of needing authentication.
	ProxyUser           string              // Proxy user
	ProxyPassword       string              // Proxy password
	IsEnableMD5         bool                // Flag of enabling MD5 for upload.
	MD5Threshold        int64               // Memory footprint threshold for each MD5 computation (16MB is the default), in byte. When the data is more than that, temp file is used.
	IsEnableCRC         bool                // Flag of enabling CRC for upload.
	LogLevel            int                 // Log level
	Logger              *log.Logger         // For write log
	UploadLimitSpeed    int                 // Upload limit speed:KB/s, 0 is unlimited
	UploadLimiter       *OssLimiter         // Bandwidth limit reader for upload
	CredentialsProvider CredentialsProvider // User provides interface to get AccessKeyID, AccessKeySecret, SecurityToken
	LocalAddr           net.Addr            // local client host info
}

// LimitUploadSpeed uploadSpeed:KB/s, 0 is unlimited,default is 0
func (config *Config) LimitUploadSpeed(uploadSpeed int) error {
	if uploadSpeed < 0 {
		return fmt.Errorf("invalid argument, the value of uploadSpeed is less than 0")
	} else if uploadSpeed == 0 {
		config.UploadLimitSpeed = 0
		config.UploadLimiter = nil
		return nil
	}

	var err error
	config.UploadLimiter, err = GetOssLimiter(uploadSpeed)
	if err == nil {
		config.UploadLimitSpeed = uploadSpeed
	}
	return err
}

// WriteLog output log function
func (config *Config) WriteLog(LogLevel int, format string, a ...interface{}) {
	if config.LogLevel < LogLevel || config.Logger == nil {
		return
	}

	var logBuffer bytes.Buffer
	logBuffer.WriteString(LogTag[LogLevel-1])
	logBuffer.WriteString(fmt.Sprintf(format, a...))
	config.Logger.Printf("%s", logBuffer.String())
}

// for get Credentials
func (config *Config) GetCredentials() Credentials {
	return config.CredentialsProvider.GetCredentials()
}

// getDefaultOssConfig gets the default configuration.
func getDefaultOssConfig() *Config {
	config := Config{}

	config.Endpoint = ""
	config.AccessKeyID = ""
	config.AccessKeySecret = ""
	config.RetryTimes = 5
	config.IsDebug = false
	config.UserAgent = userAgent()
	config.Timeout = 60 // Seconds
	config.SecurityToken = ""
	config.IsCname = false

	config.HTTPTimeout.ConnectTimeout = time.Second * 30   // 30s
	config.HTTPTimeout.ReadWriteTimeout = time.Second * 60 // 60s
	config.HTTPTimeout.HeaderTimeout = time.Second * 60    // 60s
	config.HTTPTimeout.LongTimeout = time.Second * 300     // 300s
	config.HTTPTimeout.IdleConnTimeout = time.Second * 50  // 50s
	config.HTTPMaxConns.MaxIdleConns = 100
	config.HTTPMaxConns.MaxIdleConnsPerHost = 100

	config.IsUseProxy = false
	config.ProxyHost = ""
	config.IsAuthProxy = false
	config.ProxyUser = ""
	config.ProxyPassword = ""

	config.MD5Threshold = 16 * 1024 * 1024 // 16MB
	config.IsEnableMD5 = false
	config.IsEnableCRC = true

	config.LogLevel = LogOff
	config.Logger = log.New(os.Stdout, "", log.LstdFlags)

	provider := &defaultCredentialsProvider{config: &config}
	config.CredentialsProvider = provider

	return &config
}
