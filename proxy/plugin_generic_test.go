// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"errors"
	"testing"
)

func TestExecuteRequestModifiers_typed(t *testing.T) {
	modifiers := []func(RequestWrapper) (RequestWrapper, error){
		func(w RequestWrapper) (RequestWrapper, error) {
			return &requestWrapper{
				ctx:     w.(*requestWrapper).ctx,
				method:  w.Method(),
				url:     w.URL(),
				query:   w.Query(),
				path:    w.Path() + "/modified",
				body:    w.Body(),
				params:  w.Params(),
				headers: w.Headers(),
			}, nil
		},
	}

	r := &Request{Method: "GET", Path: "/foo"}
	res, err := executeRequestModifiers[RequestWrapper](modifiers, newRequestWrapper(context.Background(), r), r)
	if err != nil {
		t.Error(err.Error())
		return
	}
	if res.Path != "/foo/modified" {
		t.Errorf("unexpected path. have %s, want /foo/modified", res.Path)
	}
	if res.Method != "GET" {
		t.Errorf("unexpected method. have %s, want GET", res.Method)
	}
}

func TestExecuteRequestModifiers_error(t *testing.T) {
	expectedErr := errors.New("modifier error")
	modifiers := []func(RequestWrapper) (RequestWrapper, error){
		func(_ RequestWrapper) (RequestWrapper, error) {
			return nil, expectedErr
		},
	}

	r := &Request{Method: "GET", Path: "/foo"}
	if _, err := executeRequestModifiers[RequestWrapper](modifiers, newRequestWrapper(context.Background(), r), r); err != expectedErr {
		t.Errorf("unexpected error. have %v, want %v", err, expectedErr)
	}
}

func TestExecuteResponseModifiers_typed(t *testing.T) {
	modifiers := []func(ResponseWrapper) (ResponseWrapper, error){
		func(w ResponseWrapper) (ResponseWrapper, error) {
			return responseWrapper{
				data:       map[string]interface{}{"modified": true},
				isComplete: w.IsComplete(),
				metadata: metadataWrapper{
					headers:    map[string][]string{"X-Modified": {"true"}},
					statusCode: 201,
				},
			}, nil
		},
	}

	req := &Request{Method: "GET", Path: "/foo"}
	resp := &Response{
		Data:       map[string]interface{}{"foo": "bar"},
		IsComplete: true,
		Metadata: Metadata{
			Headers:    map[string][]string{},
			StatusCode: 200,
		},
	}

	res, err := executeResponseModifiers[ResponseWrapper](modifiers, newResponseWrapper(context.Background(), req, resp), resp)
	if err != nil {
		t.Error(err.Error())
		return
	}
	if v, ok := res.Data["modified"].(bool); !ok || !v {
		t.Errorf("unexpected data. have %v", res.Data)
	}
	if res.Metadata.StatusCode != 201 {
		t.Errorf("unexpected status code. have %d, want 201", res.Metadata.StatusCode)
	}
	if h := res.Metadata.Headers["X-Modified"]; len(h) != 1 || h[0] != "true" {
		t.Errorf("unexpected headers. have %v", res.Metadata.Headers)
	}
}

func TestExecuteResponseModifiers_error(t *testing.T) {
	expectedErr := errors.New("modifier error")
	modifiers := []func(ResponseWrapper) (ResponseWrapper, error){
		func(_ ResponseWrapper) (ResponseWrapper, error) {
			return nil, expectedErr
		},
	}

	req := &Request{Method: "GET", Path: "/foo"}
	resp := &Response{Data: map[string]interface{}{}}
	if _, err := executeResponseModifiers[ResponseWrapper](modifiers, newResponseWrapper(context.Background(), req, resp), resp); err != expectedErr {
		t.Errorf("unexpected error. have %v, want %v", err, expectedErr)
	}
}
