package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gatechain/smart_contract_verifier/lib"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RestClient manages the REST interface for a calling user.
type Client struct {
	serverURL url.URL
	apiToken  string
}

// MakeRestClient is the factory for constructing a RestClient for a given endpoint
func MakeRestClient(url url.URL, apiToken string) Client {
	return Client{
		serverURL: url,
		apiToken:  apiToken,
	}
}

// extractError checks if the response signifies an error (for now, StatusCode != 200).
// If so, it returns the error.
// Otherwise, it returns nil.
func extractError(resp *http.Response) error {
	if resp.StatusCode == 200 {
		return nil
	}

	errorBuf, _ := ioutil.ReadAll(resp.Body) // ignore returned error
	return fmt.Errorf("HTTP %v: %s", resp.Status, errorBuf)
}

// submitForm is a helper used for submitting (ex.) GETs and POSTs to the server
func (client Client) submitForm(response interface{}, path string, request interface{}, requestMethod string, encodeJSON bool) error {
	var err error
	queryURL := client.serverURL
	queryURL.Path = path

	var req *http.Request
	var body io.Reader

	if request != nil {
		//if rawRequestPaths[path] {
			reqBytes, ok := request.([]byte)
			if !ok {
				return fmt.Errorf("couldn't decode raw request as bytes")
			}
			body = bytes.NewBuffer(reqBytes)
		//} else {
		//	v, err := query.Values(request)
		//	if err != nil {
		//		return err
		//	}
		//
		//	queryURL.RawQuery = v.Encode()
		//	if encodeJSON {
		//		jsonValue, _ := json.Marshal(request)
		//		body = bytes.NewBuffer(jsonValue)
		//	}
		//}
	}

	req, err = http.NewRequest(requestMethod, queryURL.String(), body)
	if err != nil {
		return err
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	err = extractError(resp)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(resp.Body)
	return dec.Decode(&response)
}

// get performs a GET request to the specific path against the server
func (client Client) get(response interface{}, path string, request interface{}) error {
	return client.submitForm(response, path, request, "GET", false /* encodeJSON */)
}

// post sends a POST request to the given path with the given request object.
// No query parameters will be sent if request is nil.
// response must be a pointer to an object as post writes the response there.
func (client Client) post(response interface{}, path string, request interface{}) error {
	return client.submitForm(response, path, request, "POST", true /* encodeJSON */)
}

func (client Client) FetchVersions(response *SolcVersion) error {
	path := lib.SolcPlatform + "/" + lib.SolcListVersions
	err := client.get(&response, path, nil)
	return err
}

func (client Client) FetchVersion(response SolcVersion, version string) (SolcBuild, error) {
	path := lib.RegisterApp.GetPath(lib.SolcBinApiUrl, lib.SolcBinApiUrl)
	err := client.get(&response, path, nil)
	if err != nil {
		return SolcBuild{}, err
	}
	for _, b := range response.Builds {
		if b.Version == version {
			return b, nil
		}
	}

	return SolcBuild{}, fmt.Errorf("given version not found")
}

func (client Client) Download(version string) bool {
	downloadPath := client.serverURL.String() + lib.SolcPlatform + "/" + version
	localPath := lib.CompilerLocalStoreDir() + version
	err := downloadFile(downloadPath, localPath, link)
	return err == nil
}

func downloadFile(url string, localPath string, fb func(path string) error) error {
	var (
		fsize   int64
		buf     = make([]byte, 32*1024)
		written int64
	)
	tmpFilePath := localPath + ".download"
	fmt.Println("Download temp: ", tmpFilePath)

	client := new(http.Client)
	//default timeout
	client.Timeout = time.Second * 600

	// check url
	resp, err := client.Get(url)
	if err != nil {
		return err
	}

	//get file size
	fsize, err = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 32)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Download size: ", fsize)

	// create tmp file
	file, err := os.Create(tmpFilePath)
	if err != nil {
		return err
	}
	defer file.Close()
	if resp.Body == nil {
		return errors.New("body is null")
	}
	defer resp.Body.Close()

	for {
		// read bytes
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			// write bytes
			nw, ew := file.Write(buf[0:nr])
			//data length > 0
			if nw > 0 {
				written += int64(nw)
			}
			// catch write error
			if ew != nil {
				err = ew
				break
			}
			// read & write length not equal
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	if err != nil {
		fmt.Println(err)
	} else {
		file.Close()
		err = os.Rename(tmpFilePath, localPath)
	}
	// callback, link current download
	err = fb(localPath)
	return err
}

func link(path string) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	strs := strings.Split(base, "+")
	if len(strs) < 2 {
		panic("")
	}
	fmt.Printf("link file: %s to %s \n", base, strs[0])
	cmd := fmt.Sprintf("ln -s %s %s", path, dir + "/" + strs[0])
	command := exec.Command("bash", "-c", cmd)
	err := command.Run()
	if err != nil {
		panic(err)
	}
	return nil
}