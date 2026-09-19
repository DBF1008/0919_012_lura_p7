// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"

	"github.com/luraproject/lura/v2/encoding"
)

// HTTPResponseParser defines how the response is parsed from http.Response to Response object
type HTTPResponseParser func(context.Context, *http.Response) (*Response, error)

// DefaultHTTPResponseParserConfig defines a default HTTPResponseParserConfig
var DefaultHTTPResponseParserConfig = HTTPResponseParserConfig{
	func(_ io.Reader, _ *map[string]interface{}) error { return nil },
	EntityFormatterFunc(func(r Response) Response { return r }),
}

// HTTPResponseParserConfig contains the config for a given HttpResponseParser
type HTTPResponseParserConfig struct {
	Decoder         encoding.Decoder
	EntityFormatter EntityFormatter
}

// HTTPResponseParserFactory creates HTTPResponseParser from a given HTTPResponseParserConfig
type HTTPResponseParserFactory func(HTTPResponseParserConfig) HTTPResponseParser

// DefaultHTTPResponseParserFactory is the default implementation of HTTPResponseParserFactory
func DefaultHTTPResponseParserFactory(cfg HTTPResponseParserConfig) HTTPResponseParser {
	return func(_ context.Context, resp *http.Response) (*Response, error) {
		defer resp.Body.Close()

		var reader io.ReadCloser
		switch resp.Header.Get("Content-Encoding") {
		case "gzip":
			gzipReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return nil, err
			}
			reader = gzipReader
			defer reader.Close()
		default:
			reader = resp.Body
		}

		var data map[string]interface{}
		if err := cfg.Decoder(reader, &data); err != nil {
			return nil, err
		}

		newResponse := Response{Data: data, IsComplete: true}
		newResponse = cfg.EntityFormatter.Format(newResponse)
		return &newResponse, nil
	}
}

// NoOpHTTPResponseParser is a HTTPResponseParser implementation that just copies the
// http response body into the proxy response IO
func NoOpHTTPResponseParser(ctx context.Context, resp *http.Response) (*Response, error) {
	return &Response{
		Data:       map[string]interface{}{},
		IsComplete: true,
		Io:         NewReadCloserWrapper(ctx, resp.Body),
		Metadata: Metadata{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
		},
	}, nil
}

// newResponseWrapper wraps the received response into a ResponseWrapper, so it
// can be passed to the plugin modifiers along with the originating request
func newResponseWrapper(ctx context.Context, req *Request, r *Response) responseWrapper {
	return responseWrapper{
		ctx:        ctx,
		request:    newRequestWrapper(ctx, req),
		data:       r.Data,
		isComplete: r.IsComplete,
		metadata: metadataWrapper{
			headers:    r.Metadata.Headers,
			statusCode: r.Metadata.StatusCode,
		},
		io: r.Io,
	}
}

// metadataWrapper is the ResponseWrapper metadata
type metadataWrapper struct {
	headers    map[string][]string
	statusCode int
}

func (m metadataWrapper) Headers() map[string][]string { return m.headers }
func (m metadataWrapper) StatusCode() int              { return m.statusCode }

// responseWrapper is the ResponseWrapper implementation for the Response type
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
