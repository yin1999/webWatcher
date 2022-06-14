//go:build aliyun || !tencent

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type timerTrigger struct {
	TriggerTime string `json:"triggerTime"`
	TriggerName string `json:"triggerName"`
	Payload     string `json:"payload"`
}

const (
	fcStatus    = "X-Fc-Status"
	fcRequestId = "X-Fc-Request-Id"
	apiVersion  = "2020-11-11"
	contentType = "text/plain"
)

func regist() (handler, error) {
	fmt.Println(os.Environ())
	address := os.Getenv("FC_RUNTIME_API")
	endpoint := fmt.Sprintf("http://%s/%s/runtime/invocation/", address, apiVersion)
	client, err := oss.New(os.Getenv("OSS_ENDPOINT"),
		os.Getenv("accessKeyID"),
		os.Getenv("accessKeySecret"),
		oss.SecurityToken(os.Getenv("securityToken")),
	)
	if err != nil {
		return nil, err
	}
	var bucket *oss.Bucket
	bucket, err = client.Bucket(os.Getenv("OSS_BUCKET"))
	if err != nil {
		return nil, err
	}
	return &aliyun{
		endpoint: endpoint,
		client:   http.Client{},
		bucket:   bucket,
	}, nil
}

type aliyun struct {
	endpoint string
	ua       string
	client   http.Client
	bucket   *oss.Bucket
}

func (a aliyun) Next() (payload string, reqID string, err error) {
	var resp *http.Response
	resp, err = http.Get(a.endpoint + "next")
	if err != nil {
		return
	}
	reqID = resp.Header.Get(fcRequestId)
	defer resp.Body.Close()
	timeEvent := timerTrigger{}
	err = json.NewDecoder(resp.Body).Decode(&timeEvent)
	if err == nil {
		payload = timeEvent.Payload
	}
	return
}

func (a aliyun) ReportSuccess(id string) {
	res, err := http.DefaultClient.Post(a.endpoint+id+"/response", contentType, http.NoBody)
	if err == nil {
		res.Body.Close()
	} else {
		Error.Log(err.Error() + "\n")
	}
}

func (a aliyun) ReportError(msg string, id string) {
	res, err := http.Post(a.endpoint+id+"/error",
		contentType,
		strings.NewReader(msg),
	)
	if err == nil {
		res.Body.Close()
	} else {
		Error.Log(err.Error() + "\n")
	}
}

func (a aliyun) PutObject(key string, data []byte) error {
	return a.bucket.PutObject(key, bytes.NewReader(data))
}

// GetObject returns the object with the given key.
//
// If the object is not found, it returns nil and no error.
func (a aliyun) GetObject(key string) (data []byte, err error) {
	resp, err := a.bucket.GetObject(key)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			err = nil
		}
		return
	}
	defer resp.Close()
	return io.ReadAll(resp)
}
