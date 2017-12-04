// Package script provides access to the Google Apps Script API.
//
// See https://developers.google.com/apps-script/execution/rest/v1/scripts/run
//
// Usage example:
//
//   import "google.golang.org/api/script/v1"
//   ...
//   scriptService, err := script.New(oauthHttpClient)
package script // import "google.golang.org/api/script/v1"

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	context "golang.org/x/net/context"
	ctxhttp "golang.org/x/net/context/ctxhttp"
	gensupport "google.golang.org/api/gensupport"
	googleapi "google.golang.org/api/googleapi"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Always reference these packages, just in case the auto-generated code
// below doesn't.
var _ = bytes.NewBuffer
var _ = strconv.Itoa
var _ = fmt.Sprintf
var _ = json.NewDecoder
var _ = io.Copy
var _ = url.Parse
var _ = gensupport.MarshalJSON
var _ = googleapi.Version
var _ = errors.New
var _ = strings.Replace
var _ = context.Canceled
var _ = ctxhttp.Do

const apiId = "script:v1"
const apiName = "script"
const apiVersion = "v1"
const basePath = "https://script.googleapis.com/"

// OAuth2 scopes used by this API.
const (
	// Read, send, delete, and manage your email
	MailGoogleComScope = "https://mail.google.com/"

	// Manage your calendars
	WwwGoogleComCalendarFeedsScope = "https://www.google.com/calendar/feeds"

	// Manage your contacts
	WwwGoogleComM8FeedsScope = "https://www.google.com/m8/feeds"

	// View and manage the provisioning of groups on your domain
	AdminDirectoryGroupScope = "https://www.googleapis.com/auth/admin.directory.group"

	// View and manage the provisioning of users on your domain
	AdminDirectoryUserScope = "https://www.googleapis.com/auth/admin.directory.user"

	// View and manage the files in your Google Drive
	DriveScope = "https://www.googleapis.com/auth/drive"

	// View and manage your forms in Google Drive
	FormsScope = "https://www.googleapis.com/auth/forms"

	// View and manage forms that this application has been installed in
	FormsCurrentonlyScope = "https://www.googleapis.com/auth/forms.currentonly"

	// View and manage your Google Groups
	GroupsScope = "https://www.googleapis.com/auth/groups"

	// View and manage your spreadsheets in Google Drive
	SpreadsheetsScope = "https://www.googleapis.com/auth/spreadsheets"

	// View your email address
	UserinfoEmailScope = "https://www.googleapis.com/auth/userinfo.email"
)

func New(client *http.Client) (*Service, error) {
	if client == nil {
		return nil, errors.New("client is nil")
	}
	s := &Service{client: client, BasePath: basePath}
	s.Scripts = NewScriptsService(s)
	return s, nil
}

type Service struct {
	client    *http.Client
	BasePath  string // API endpoint base URL
	UserAgent string // optional additional User-Agent fragment

	Scripts *ScriptsService
}

func (s *Service) userAgent() string {
	if s.UserAgent == "" {
		return googleapi.UserAgent
	}
	return googleapi.UserAgent + " " + s.UserAgent
}

func NewScriptsService(s *Service) *ScriptsService {
	rs := &ScriptsService{s: s}
	return rs
}

type ScriptsService struct {
	s *Service
}

// ExecutionError: An object that provides information about the nature
// of an error resulting
// from an attempted execution of a script function using the Apps
// Script API.
// If a run call
// succeeds but the script function (or Apps Script itself) throws an
// exception,
// the response body's error field
// contains a
// Status object. The `Status` object's `details` field
// contains an array with a single one of these `ExecutionError`
// objects.
type ExecutionError struct {
	// ErrorMessage: The error message thrown by Apps Script, usually
	// localized into the user's
	// language.
	ErrorMessage string `json:"errorMessage,omitempty"`

	// ErrorType: The error type, for example `TypeError` or
	// `ReferenceError`. If the error
	// type is unavailable, this field is not included.
	ErrorType string `json:"errorType,omitempty"`

	// ScriptStackTraceElements: An array of objects that provide a stack
	// trace through the script to show
	// where the execution failed, with the deepest call first.
	ScriptStackTraceElements []*ScriptStackTraceElement `json:"scriptStackTraceElements,omitempty"`

	// ForceSendFields is a list of field names (e.g. "ErrorMessage") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "ErrorMessage") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ExecutionError) MarshalJSON() ([]byte, error) {
	type NoMethod ExecutionError
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ExecutionRequest: A request to run the function in a script. The
// script is identified by the
// specified `script_id`. Executing a function on a script returns
// results
// based on the implementation of the script.
type ExecutionRequest struct {
	// DevMode: If `true` and the user is an owner of the script, the script
	// runs at the
	// most recently saved version rather than the version deployed for use
	// with
	// the Apps Script API. Optional; default is `false`.
	DevMode bool `json:"devMode,omitempty"`

	// Function: The name of the function to execute in the given script.
	// The name does not
	// include parentheses or parameters.
	Function string `json:"function,omitempty"`

	// Parameters: The parameters to be passed to the function being
	// executed. The object type
	// for each parameter should match the expected type in Apps
	// Script.
	// Parameters cannot be Apps Script-specific object types (such as
	// a
	// `Document` or a `Calendar`); they can only be primitive types such
	// as
	// `string`, `number`, `array`, `object`, or `boolean`. Optional.
	Parameters []interface{} `json:"parameters,omitempty"`

	// SessionState: For Android add-ons only. An ID that represents the
	// user's current session
	// in the Android app for Google Docs or Sheets, included as extra data
	// in
	// the
	// [Intent](https://developer.android.com/guide/components/intents-fi
	// lters.html)
	// that launches the add-on. When an Android add-on is run with a
	// session
	// state, it gains the privileges of
	// a
	// [bound](https://developers.google.com/apps-script/guides/bound)
	// scri
	// pt&mdash;that is, it can access information like the user's
	// current
	// cursor position (in Docs) or selected cell (in Sheets). To retrieve
	// the
	// state,
	// call
	// `Intent.getStringExtra("com.google.android.apps.docs.addons.Sessi
	// onState")`.
	// Optional.
	SessionState string `json:"sessionState,omitempty"`

	// ForceSendFields is a list of field names (e.g. "DevMode") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "DevMode") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ExecutionRequest) MarshalJSON() ([]byte, error) {
	type NoMethod ExecutionRequest
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ExecutionResponse: An object that provides the return value of a
// function executed using the
// Apps Script API. If the script function returns successfully, the
// response
// body's response field contains this
// `ExecutionResponse` object.
type ExecutionResponse struct {
	// Result: The return value of the script function. The type matches the
	// object type
	// returned in Apps Script. Functions called using the Apps Script API
	// cannot
	// return Apps Script-specific objects (such as a `Document` or a
	// `Calendar`);
	// they can only return primitive types such as a `string`, `number`,
	// `array`,
	// `object`, or `boolean`.
	Result interface{} `json:"result,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Result") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Result") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ExecutionResponse) MarshalJSON() ([]byte, error) {
	type NoMethod ExecutionResponse
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// Operation: A representation of a execution of an Apps Script function
// that is started using run. The execution response does not arrive
// until the function finishes executing. The maximum execution runtime
// is listed in the [Apps Script quotas
// guide](/apps-script/guides/services/quotas#current_limitations).
// <p>After the execution is started, it can have one of four
// outcomes:</p> <ul> <li> If the script function returns successfully,
// the
//   response field contains an
//   ExecutionResponse object
//   with the function's return value in the object's `result`
// field.</li>
// <li> If the script function (or Apps Script itself) throws an
// exception, the
//   error field contains a
//   Status object. The `Status` object's `details`
//   field contains an array with a single
//   ExecutionError object that
//   provides information about the nature of the error.</li>
// <li> If the execution has not yet completed,
//   the done field is `false` and
//   the neither the `response` nor `error` fields are
// present.</li>
// <li> If the `run` call itself fails (for example, because of a
//   malformed request or an authorization error), the method returns an
// HTTP
//   response code in the 4XX range with a different format for the
// response
//   body. Client libraries automatically convert a 4XX response into
// an
//   exception class.</li>
// </ul>
type Operation struct {
	// Done: This field indicates whether the script execution has
	// completed. A completed execution has a populated `response` field
	// containing the ExecutionResponse from function that was executed.
	Done bool `json:"done,omitempty"`

	// Error: If a `run` call succeeds but the script function (or Apps
	// Script itself) throws an exception, this field contains a Status
	// object. The `Status` object's `details` field contains an array with
	// a single ExecutionError object that provides information about the
	// nature of the error.
	Error *Status `json:"error,omitempty"`

	// Response: If the script function returns successfully, this field
	// contains an ExecutionResponse object with the function's return
	// value.
	Response googleapi.RawMessage `json:"response,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Done") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Done") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *Operation) MarshalJSON() ([]byte, error) {
	type NoMethod Operation
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// ScriptStackTraceElement: A stack trace through the script that shows
// where the execution failed.
type ScriptStackTraceElement struct {
	// Function: The name of the function that failed.
	Function string `json:"function,omitempty"`

	// LineNumber: The line number where the script failed.
	LineNumber int64 `json:"lineNumber,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Function") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Function") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *ScriptStackTraceElement) MarshalJSON() ([]byte, error) {
	type NoMethod ScriptStackTraceElement
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// Status: If a `run` call succeeds but the script function (or Apps
// Script itself) throws an exception, the response body's error field
// contains this `Status` object.
type Status struct {
	// Code: The status code. For this API, this value either: <ul> <li> 3,
	// indicating an `INVALID_ARGUMENT` error, or</li> <li> 1, indicating a
	// `CANCELLED` execution.</li> </ul>
	Code int64 `json:"code,omitempty"`

	// Details: An array that contains a single ExecutionError object that
	// provides information about the nature of the error.
	Details []googleapi.RawMessage `json:"details,omitempty"`

	// Message: A developer-facing error message, which is in English. Any
	// user-facing error message is localized and sent in the
	// [google.rpc.Status.details](google.rpc.Status.details) field, or
	// localized by the client.
	Message string `json:"message,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Code") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Code") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *Status) MarshalJSON() ([]byte, error) {
	type NoMethod Status
	raw := NoMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// method id "script.scripts.run":

type ScriptsRunCall struct {
	s                *Service
	scriptId         string
	executionrequest *ExecutionRequest
	urlParams_       gensupport.URLParams
	ctx_             context.Context
	header_          http.Header
}

// Run: Runs a function in an Apps Script project. The project must be
// deployed
// for use with the Apps Script API.
//
// This method requires authorization with an OAuth 2.0 token that
// includes at
// least one of the scopes listed in the
// [Authorization](#authorization)
// section; script projects that do not require authorization cannot
// be
// executed through this API. To find the correct scopes to include in
// the
// authentication token, open the project in the script editor, then
// select
// **File > Project properties** and click the **Scopes** tab.
func (r *ScriptsService) Run(scriptId string, executionrequest *ExecutionRequest) *ScriptsRunCall {
	c := &ScriptsRunCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.scriptId = scriptId
	c.executionrequest = executionrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ScriptsRunCall) Fields(s ...googleapi.Field) *ScriptsRunCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ScriptsRunCall) Context(ctx context.Context) *ScriptsRunCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ScriptsRunCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ScriptsRunCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.executionrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "v1/scripts/{scriptId}:run")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"scriptId": c.scriptId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "script.scripts.run" call.
// Exactly one of *Operation or error will be non-nil. Any non-2xx
// status code is an error. Response headers are in either
// *Operation.ServerResponse.Header or (if a response was returned at
// all) in error.(*googleapi.Error).Header. Use googleapi.IsNotModified
// to check whether the returned error was because
// http.StatusNotModified was returned.
func (c *ScriptsRunCall) Do(opts ...googleapi.CallOption) (*Operation, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &Operation{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := gensupport.DecodeResponse(target, res); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Runs a function in an Apps Script project. The project must be deployed\nfor use with the Apps Script API.\n\nThis method requires authorization with an OAuth 2.0 token that includes at\nleast one of the scopes listed in the [Authorization](#authorization)\nsection; script projects that do not require authorization cannot be\nexecuted through this API. To find the correct scopes to include in the\nauthentication token, open the project in the script editor, then select\n**File \u003e Project properties** and click the **Scopes** tab.",
	//   "flatPath": "v1/scripts/{scriptId}:run",
	//   "httpMethod": "POST",
	//   "id": "script.scripts.run",
	//   "parameterOrder": [
	//     "scriptId"
	//   ],
	//   "parameters": {
	//     "scriptId": {
	//       "description": "The script ID of the script to be executed. To find the script ID, open\nthe project in the script editor and select **File \u003e Project properties**.",
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "v1/scripts/{scriptId}:run",
	//   "request": {
	//     "$ref": "ExecutionRequest"
	//   },
	//   "response": {
	//     "$ref": "Operation"
	//   },
	//   "scopes": [
	//     "https://mail.google.com/",
	//     "https://www.google.com/calendar/feeds",
	//     "https://www.google.com/m8/feeds",
	//     "https://www.googleapis.com/auth/admin.directory.group",
	//     "https://www.googleapis.com/auth/admin.directory.user",
	//     "https://www.googleapis.com/auth/drive",
	//     "https://www.googleapis.com/auth/forms",
	//     "https://www.googleapis.com/auth/forms.currentonly",
	//     "https://www.googleapis.com/auth/groups",
	//     "https://www.googleapis.com/auth/spreadsheets",
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
