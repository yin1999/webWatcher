//go:build tencent

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	cos "github.com/tencentyun/cos-go-sdk-v5"
)

const contentType = "text/plain"

func regist() (handler, error) {
	host := os.Getenv("SCF_RUNTIME_API")
	port := os.Getenv("SCF_RUNTIME_API_PORT")
	endpoint := fmt.Sprintf("http://%s:%s/", host, port)
	res, err := http.Post(endpoint+"runtime/init/ready", contentType, http.NoBody)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	u, err := url.Parse(os.Getenv("BUCKET_URL"))
	if err != nil {
		return nil, err
	}
	cosClient := cos.NewClient(&cos.BaseURL{BucketURL: u}, &http.Client{
		Timeout: time.Second * 10,
		Transport: &cos.AuthorizationTransport{
			SecretID:  os.Getenv("COS_SECRETID"),
			SecretKey: os.Getenv("COS_SECRETKEY"),
		},
	})
	return tencent{
		endpoint: endpoint,
		bucket:   cosClient,
	}, err
}

type tencent struct {
	endpoint string
	bucket   *cos.Client
}

type timerTrigger struct {
	TriggerTime string `json:"Time"`
	TriggerName string `json:"TriggerName"`
	Payload     string `json:"Message"`
}

func (h tencent) Next() (payload string, id string, err error) {
	var resp *http.Response
	resp, err = http.Get(h.endpoint + "runtime/invocation/next")
	if err != nil {
		return
	}
	id = resp.Header.Get("Request_id")
	defer resp.Body.Close()
	timeEvent := timerTrigger{}
	err = json.NewDecoder(resp.Body).Decode(&timeEvent)
	if err == nil {
		payload = timeEvent.Payload
	}
	return
}

func (h tencent) ReportSuccess(id string) {
	res, err := http.DefaultClient.Post(h.endpoint+"runtime/invocation/response", contentType, strings.NewReader(id))
	if err == nil {
		res.Body.Close()
	} else {
		Fatal.Log(err.Error() + "\n")
	}
}

func (h tencent) ReportError(msg string, _ string) {
	res, err := http.DefaultClient.Post(h.endpoint+"runtime/invocation/error",
		contentType,
		strings.NewReader(msg),
	)
	if err == nil {
		res.Body.Close()
	} else {
		Error.Log(err.Error() + "\n")
	}
}

func (h tencent) PutObject(name string, data []byte) error {
	_, err := h.bucket.Object.Put(context.Background(), name, bytes.NewReader(data), nil)
	return err
}

func (h tencent) GetObject(name string) (data []byte, err error) {
	var isExist bool
	isExist, err = h.bucket.Object.IsExist(context.Background(), name)
	if err != nil || !isExist {
		return
	}
	var res *cos.Response
	res, err = h.bucket.Object.Get(context.Background(), name, nil)
	if err != nil {
		return
	}
	defer res.Body.Close()
	data = make([]byte, res.ContentLength)
	var n int
	n, err = res.Body.Read(data)
	if err == io.EOF {
		err = nil
	}
	data = data[:n]
	return
}
