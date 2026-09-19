// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/luraproject/lura/v2/config"
	"github.com/luraproject/lura/v2/logging"
	"github.com/luraproject/lura/v2/proxy/plugin"
)

// NewPluginMiddleware returns an endpoint middleware wrapped (if required) with the plugin middleware.
// The plugin middleware will try to load all the required plugins from the register and execute them in order.
// RequestModifiers are executed before passing the request to the next middlware. ResponseModifiers are executed
// once the response is returned from the next middleware.
func NewPluginMiddleware(logger logging.Logger, endpoint *config.EndpointConfig) Middleware {
	cfg, ok := endpoint.ExtraConfig[plugin.Namespace].(map[string]interface{})

	if !ok {
		return emptyMiddlewareFallback(logger)
	}

	return newPluginMiddleware(logger, "ENDPOINT", endpoint.Endpoint, cfg)
}

// NewBackendPluginMiddleware returns a backend middleware wrapped (if required) with the plugin middleware.
// The plugin middleware will try to load all the required plugins from the register and execute them in order.
// RequestModifiers are executed before passing the request to the next middlware. ResponseModifiers are executed
// once the response is returned from the next middleware.
func NewBackendPluginMiddleware(logger logging.Logger, remote *config.Backend) Middleware {
	cfg, ok := remote.ExtraConfig[plugin.Namespace].(map[string]interface{})

	if !ok {
		return emptyMiddlewareFallback(logger)
	}

	return newPluginMiddleware(logger, "BACKEND",
		fmt.Sprintf("%s %s -> %s", remote.ParentEndpointMethod, remote.ParentEndpoint, remote.URLPattern), cfg)
}

func newPluginMiddleware(logger logging.Logger, tag, pattern string, cfg map[string]interface{}) Middleware {
	plugins, ok := cfg["name"].([]interface{})
	if !ok {
		return emptyMiddlewareFallback(logger)
	}

	var reqModifiers []plugin.Modifier[RequestWrapper]

	var respModifiers []plugin.Modifier[ResponseWrapper]

	for _, p := range plugins {
		name, ok := p.(string)
		if !ok {
			continue
		}

		if mf, ok := plugin.GetRequestModifier[RequestWrapper](name); ok {
			if fn := mf(cfg); fn != nil {
				reqModifiers = append(reqModifiers, fn)
			}
			continue
		}

		if mf, ok := plugin.GetResponseModifier[ResponseWrapper](name); ok {
			if fn := mf(cfg); fn != nil {
				respModifiers = append(respModifiers, fn)
			}
		}
	}

	totReqModifiers, totRespModifiers := len(reqModifiers), len(respModifiers)
	if totReqModifiers == totRespModifiers && totRespModifiers == 0 {
		return emptyMiddlewareFallback(logger)
	}

	logger.Debug(
		fmt.Sprintf(
			"[%s: %s][Modifier Plugins] Adding %d request and %d response modifiers",
			tag,
			pattern,
			totReqModifiers,
			totRespModifiers,
		),
	)

	return func(next ...Proxy) Proxy {
		if len(next) > 1 {
			logger.Fatal("too many proxies for this proxy middleware: newPluginMiddleware only accepts 1 proxy, got %d tag: %s, pattern: %s",
				len(next), tag, pattern)
			return nil
		}

		if totReqModifiers == 0 {
			return func(ctx context.Context, r *Request) (*Response, error) {
				resp, err := next[0](ctx, r)
				if err != nil {
					return resp, err
				}

				return executeResponseModifiers[ResponseWrapper](respModifiers, newResponseWrapper(ctx, newRequestWrapper(ctx, r), resp), resp)
			}
		}

		if totRespModifiers == 0 {
			return func(ctx context.Context, r *Request) (*Response, error) {
				var err error
				r, err = executeRequestModifiers[RequestWrapper](reqModifiers, newRequestWrapper(ctx, r), r)
				if err != nil {
					return nil, err
				}

				return next[0](ctx, r)
			}
		}

		return func(ctx context.Context, r *Request) (*Response, error) {
			var err error
			r, err = executeRequestModifiers[RequestWrapper](reqModifiers, newRequestWrapper(ctx, r), r)
			if err != nil {
				return nil, err
			}

			resp, err := next[0](ctx, r)
			if err != nil {
				return resp, err
			}

			return executeResponseModifiers[ResponseWrapper](respModifiers, newResponseWrapper(ctx, newRequestWrapper(ctx, r), resp), resp)
		}
	}
}

// executeRequestModifiers runs the given request modifiers in order, threading the
// wrapper returned by each one into the next, and copies the final state into r.
func executeRequestModifiers[T RequestWrapper](reqModifiers []plugin.Modifier[T], tmp T, r *Request) (*Request, error) {
	for _, f := range reqModifiers {
		res, err := f(tmp)
		if err != nil {
			return nil, err
		}
		tmp = res
	}

	r.Method = tmp.Method()
	r.URL = tmp.URL()
	r.Query = tmp.Query()
	r.Path = tmp.Path()
	r.Body = tmp.Body()
	r.Params = tmp.Params()
	r.Headers = tmp.Headers()

	return r, nil
}

// executeResponseModifiers runs the given response modifiers in order, threading the
// wrapper returned by each one into the next, and copies the final state into r.
func executeResponseModifiers[T ResponseWrapper](respModifiers []plugin.Modifier[T], tmp T, r *Response) (*Response, error) {
	for _, f := range respModifiers {
		res, err := f(tmp)
		if err != nil {
			return nil, err
		}
		tmp = res
	}

	r.Data = tmp.Data()
	r.IsComplete = tmp.IsComplete()
	r.Io = tmp.Io()
	r.Metadata = Metadata{}
	r.Metadata.Headers = tmp.Headers()
	r.Metadata.StatusCode = tmp.StatusCode()
	return r, nil
}

// RequestWrapper is the generic constraint for passing proxy requests between the
// lura pipe and the loaded plugins
type RequestWrapper interface {
	Params() map[string]string
	Headers() map[string][]string
	Body() io.ReadCloser
	Method() string
	URL() *url.URL
	Query() url.Values
	Path() string
}

// ResponseWrapper is the generic constraint for passing proxy responses between the
// lura pipe and the loaded plugins
type ResponseWrapper interface {
	Data() map[string]interface{}
	Io() io.Reader
	IsComplete() bool
	Headers() map[string][]string
	StatusCode() int
}

var (
	_ RequestWrapper  = (*requestWrapper)(nil)
	_ ResponseWrapper = (*responseWrapper)(nil)
)

func newRequestWrapper(ctx context.Context, r *Request) *requestWrapper {
	return &requestWrapper{
		ctx:     ctx,
		method:  r.Method,
		url:     r.URL,
		query:   r.Query,
		path:    r.Path,
		body:    r.Body,
		params:  r.Params,
		headers: r.Headers,
	}
}

func newResponseWrapper(ctx context.Context, req RequestWrapper, r *Response) *responseWrapper {
	return &responseWrapper{
		ctx:        ctx,
		request:    req,
		data:       r.Data,
		isComplete: r.IsComplete,
		metadata: metadataWrapper{
			headers:    r.Metadata.Headers,
			statusCode: r.Metadata.StatusCode,
		},
		io: r.Io,
	}
}

type requestWrapper struct {
	ctx     context.Context
	method  string
	url     *url.URL
	query   url.Values
	path    string
	body    io.ReadCloser
	params  map[string]string
	headers map[string][]string
}

func (r *requestWrapper) Context() context.Context     { return r.ctx }
func (r *requestWrapper) Method() string               { return r.method }
func (r *requestWrapper) URL() *url.URL                { return r.url }
func (r *requestWrapper) Query() url.Values            { return r.query }
func (r *requestWrapper) Path() string                 { return r.path }
func (r *requestWrapper) Body() io.ReadCloser          { return r.body }
func (r *requestWrapper) Params() map[string]string    { return r.params }
func (r *requestWrapper) Headers() map[string][]string { return r.headers }

type metadataWrapper struct {
	headers    map[string][]string
	statusCode int
}

func (m metadataWrapper) Headers() map[string][]string { return m.headers }
func (m metadataWrapper) StatusCode() int              { return m.statusCode }

type responseWrapper struct {
	ctx        context.Context
	request    interface{}
	data       map[string]interface{}
	isComplete bool
	metadata   metadataWrapper
	io         io.Reader
}

func (r responseWrapper) Context() context.Context     { return r.ctx }
func (r responseWrapper) Request() interface{}         { return r.request }
func (r responseWrapper) Data() map[string]interface{} { return r.data }
func (r responseWrapper) IsComplete() bool             { return r.isComplete }
func (r responseWrapper) Io() io.Reader                { return r.io }
func (r responseWrapper) Headers() map[string][]string { return r.metadata.headers }
func (r responseWrapper) StatusCode() int              { return r.metadata.statusCode }
