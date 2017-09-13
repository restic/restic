// +build go1.7

package management

import (
	"io/ioutil"
	"net/http"
)

func getResponseBody(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	return ioutil.ReadAll(response.Body)
}
