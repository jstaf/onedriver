package onedriver

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
)

const graphURL = "https://graph.microsoft.com/v1.0"

// graphError is an internal struct used when decoding Graph's error messages
type graphError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Request performs an authenticated request to Microsoft Graph
func Request(resource string, auth Auth, method string, content io.Reader) ([]byte, error) {
	auth.Refresh()
	client := &http.Client{}
	request, _ := http.NewRequest("GET", graphURL+resource, content)
	request.Header.Add("Authorization", "bearer "+auth.AccessToken)
	response, err := client.Do(request)
	if err != nil {
		// the actual request failed
		return nil, err
	}
	defer response.Body.Close()
	body, _ := ioutil.ReadAll(response.Body)
	if response.StatusCode >= 400 {
		// something was wrong with the request
		var err graphError
		json.Unmarshal(body, &err)
		return nil, errors.New(err.Error.Code + ": " + err.Error.Message)
	}
	return body, nil
}
