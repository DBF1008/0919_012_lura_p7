// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"errors"
	"testing"

	"github.com/luraproject/lura/v2/config"
	"github.com/luraproject/lura/v2/logging"
	"github.com/luraproject/lura/v2/proxy/plugin"
)

func TestNewPluginMiddleware_genericModifiers(t *testing.T) {
	plugin.RegisterModifierFactory[RequestWrapper]("generic-test-request",
		func(_ map[string]interface{}) plugin.Modifier[RequestWrapper] {
			return func(w RequestWrapper) (RequestWrapper, error) {
				return &requestWrapper{
					method:  w.Method(),
					url:     w.URL(),
					query:   w.Query(),
					path:    w.Path() + "/generic",
					body:    w.Body(),
					params:  w.Params(),
					headers: w.Headers(),
				}, nil
			}
		}, true, false)

	plugin.RegisterModifierFactory[ResponseWrapper]("generic-test-response",
		func(_ map[string]interface{}) plugin.Modifier[ResponseWrapper] {
			return func(w ResponseWrapper) (ResponseWrapper, error) {
				return &responseWrapper{
					data:       map[string]interface{}{"modified": true},
					isComplete: w.IsComplete(),
					metadata: metadataWrapper{
						headers:    w.Headers(),
						statusCode: 418,
					},
					io: w.Io(),
				}, nil
			}
		}, false, true)

	validator := func(_ context.Context, r *Request) (*Response, error) {
		if r.Path != "/bar/generic" {
			return nil, errors.New("unexpected path: " + r.Path)
		}
		return &Response{Data: map[string]interface{}{"foo": "bar"}, IsComplete: true}, nil
	}

	p := NewPluginMiddleware(
		logging.NoOp,
		&config.EndpointConfig{
			ExtraConfig: map[string]interface{}{
				plugin.Namespace: map[string]interface{}{
					"name": []interface{}{"generic-test-request", "generic-test-response"},
				},
			},
		},
	)(validator)

	resp, err := p(context.Background(), &Request{Path: "/bar"})
	if err != nil {
		t.Fatal(err)
	}
	if modified, ok := resp.Data["modified"].(bool); !ok || !modified {
		t.Errorf("unexpected response data: %v", resp.Data)
	}
	if sc := resp.Metadata.StatusCode; sc != 418 {
		t.Errorf("unexpected status code: %d", sc)
	}
}

func TestNewPluginMiddleware_legacyModifiersStillWork(t *testing.T) {
	plugin.RegisterModifier("generic-test-legacy", func(_ map[string]interface{}) func(interface{}) (interface{}, error) {
		return func(input interface{}) (interface{}, error) {
			w, ok := input.(RequestWrapper)
			if !ok {
				return nil, errors.New("unknown type")
			}
			return &requestWrapper{
				method:  w.Method(),
				url:     w.URL(),
				query:   w.Query(),
				path:    w.Path() + "/legacy",
				body:    w.Body(),
				params:  w.Params(),
				headers: w.Headers(),
			}, nil
		}
	}, true, false)

	validator := func(_ context.Context, r *Request) (*Response, error) {
		if r.Path != "/bar/legacy" {
			return nil, errors.New("unexpected path: " + r.Path)
		}
		return &Response{Data: map[string]interface{}{}, IsComplete: true}, nil
	}

	p := NewPluginMiddleware(
		logging.NoOp,
		&config.EndpointConfig{
			ExtraConfig: map[string]interface{}{
				plugin.Namespace: map[string]interface{}{
					"name": []interface{}{"generic-test-legacy"},
				},
			},
		},
	)(validator)

	if _, err := p(context.Background(), &Request{Path: "/bar"}); err != nil {
		t.Fatal(err)
	}
}

func TestNewPluginMiddleware_legacyWrongTypeIsSkipped(t *testing.T) {
	plugin.RegisterModifier("generic-test-wrong-type", func(_ map[string]interface{}) func(interface{}) (interface{}, error) {
		return func(_ interface{}) (interface{}, error) {
			return "not a request wrapper", nil
		}
	}, true, false)

	validator := func(_ context.Context, r *Request) (*Response, error) {
		if r.Path != "/bar" {
			return nil, errors.New("unexpected path: " + r.Path)
		}
		return &Response{Data: map[string]interface{}{}, IsComplete: true}, nil
	}

	p := NewPluginMiddleware(
		logging.NoOp,
		&config.EndpointConfig{
			ExtraConfig: map[string]interface{}{
				plugin.Namespace: map[string]interface{}{
					"name": []interface{}{"generic-test-wrong-type"},
				},
			},
		},
	)(validator)

	if _, err := p(context.Background(), &Request{Path: "/bar"}); err != nil {
		t.Fatal(err)
	}
}
