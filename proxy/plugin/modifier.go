// SPDX-License-Identifier: Apache-2.0

/*
Package plugin provides tools for loading and registering proxy plugins
*/
package plugin

import (
	"context"
	"fmt"
	"plugin"
	"strings"

	"github.com/luraproject/lura/v2/logging"
	luraplugin "github.com/luraproject/lura/v2/plugin"
	"github.com/luraproject/lura/v2/register"
)

const (
	// Namespace is the namespace for the extra_config section
	Namespace = "github.com/devopsfaith/krakend/proxy/plugin"
	// requestNamespace is the internal namespace for the register to be used with request modifiers
	requestNamespace = "github.com/devopsfaith/krakend/proxy/plugin/request"
	// responseNamespace is the internal namespace for the register to be used with response modifiers
	responseNamespace = "github.com/devopsfaith/krakend/proxy/plugin/response"
)

var modifierRegister = register.New()

// Modifier is a generic function that transforms a value of type T
type Modifier[T any] func(T) (T, error)

// ModifierFactory is a generic function that, given a config passed as a map, returns a Modifier
type ModifierFactory[T any] func(map[string]interface{}) Modifier[T]

// legacyModifierFactory is the modifier factory signature used by plugins compiled
// against the pre-generics API. It is kept as the storage format of the shared
// register so those plugins keep working unchanged.
type legacyModifierFactory = func(map[string]interface{}) func(interface{}) (interface{}, error)

// Register is a generic, type-safe view over a namespace of the shared modifier
// register. Factories registered through the legacy RegisterModifier entrypoint
// are visible to generic lookups and vice versa.
type Register[T any] struct {
	data *register.Untyped
}

// NewRegister returns the generic Register bound to the given namespace of the
// shared modifier register
func NewRegister[T any](namespace string) *Register[T] {
	modifierRegister.AddNamespace(namespace)
	r, _ := modifierRegister.Get(namespace)
	return &Register[T]{data: r}
}

// Register stores the given ModifierFactory under the given name
func (r *Register[T]) Register(name string, f ModifierFactory[T]) {
	r.data.Register(name, toLegacyFactory(f))
}

// Get returns the ModifierFactory stored under the given name
func (r *Register[T]) Get(name string) (ModifierFactory[T], bool) {
	v, ok := r.data.Get(name)
	if !ok {
		return nil, ok
	}
	legacy, ok := v.(legacyModifierFactory)
	if !ok {
		return nil, ok
	}
	return fromLegacyFactory[T](legacy), true
}

// GetRequestModifier returns a ModifierFactory from the request namespace by name
func GetRequestModifier[T any](name string) (ModifierFactory[T], bool) {
	return NewRegister[T](requestNamespace).Get(name)
}

// GetResponseModifier returns a ModifierFactory from the response namespace by name
func GetResponseModifier[T any](name string) (ModifierFactory[T], bool) {
	return NewRegister[T](responseNamespace).Get(name)
}

// RegisterModifierFactory registers a generic ModifierFactory with the given name
// at the selected namespaces. It is the type-safe counterpart of RegisterModifier.
func RegisterModifierFactory[T any](
	name string,
	modifierFactory ModifierFactory[T],
	appliesToRequest bool,
	appliesToResponse bool,
) {
	if appliesToRequest {
		NewRegister[T](requestNamespace).Register(name, modifierFactory)
	}
	if appliesToResponse {
		NewRegister[T](responseNamespace).Register(name, modifierFactory)
	}
}

// fromLegacyFactory adapts a legacy modifier factory into a generic one. Modifiers
// returning values that do not satisfy T are discarded, keeping the previous value,
// as the old execute*Modifiers loops did.
func fromLegacyFactory[T any](f legacyModifierFactory) ModifierFactory[T] {
	return func(cfg map[string]interface{}) Modifier[T] {
		m := f(cfg)
		if m == nil {
			return nil
		}
		return func(v T) (T, error) {
			res, err := m(v)
			if err != nil {
				return v, err
			}
			t, ok := res.(T)
			if !ok {
				return v, nil
			}
			return t, nil
		}
	}
}

// toLegacyFactory adapts a generic ModifierFactory into the legacy signature so it
// can be stored in the shared register and consumed by legacy clients. Values that
// do not satisfy T are passed through unchanged.
func toLegacyFactory[T any](f ModifierFactory[T]) legacyModifierFactory {
	return func(cfg map[string]interface{}) func(interface{}) (interface{}, error) {
		m := f(cfg)
		if m == nil {
			return nil
		}
		return func(v interface{}) (interface{}, error) {
			t, ok := v.(T)
			if !ok {
				return v, nil
			}
			return m(t)
		}
	}
}

// RegisterModifier registers the injected modifier factory with the given name at the selected namespace.
// It keeps the pre-generics signature so plugins compiled against the legacy API keep working.
func RegisterModifier(
	name string,
	modifierFactory func(map[string]interface{}) func(interface{}) (interface{}, error),
	appliesToRequest bool,
	appliesToResponse bool,
) {
	if appliesToRequest {
		modifierRegister.Register(requestNamespace, name, modifierFactory)
	}
	if appliesToResponse {
		modifierRegister.Register(responseNamespace, name, modifierFactory)
	}
}

// Registerer defines the interface for the plugins to expose in order to be able to be loaded/registered
type Registerer interface {
	RegisterModifiers(func(
		name string,
		modifierFactory func(map[string]interface{}) func(interface{}) (interface{}, error),
		appliesToRequest bool,
		appliesToResponse bool,
	))
}

type LoggerRegisterer interface {
	RegisterLogger(interface{})
}

type ContextRegisterer interface {
	RegisterContext(context.Context)
}

// RegisterModifierFunc type is the function passed to the loaded Registerers
type RegisterModifierFunc func(
	name string,
	modifierFactory func(map[string]interface{}) func(interface{}) (interface{}, error),
	appliesToRequest bool,
	appliesToResponse bool,
)

// Load scans the given path using the pattern and registers all the found modifier plugins into the rmf
func Load(path, pattern string, rmf RegisterModifierFunc) (int, error) {
	return LoadWithLogger(path, pattern, rmf, nil)
}

// LoadWithLogger scans the given path using the pattern and registers all the found modifier plugins into the rmf
func LoadWithLogger(path, pattern string, rmf RegisterModifierFunc, logger logging.Logger) (int, error) {
	return LoadWithLoggerAndContext(context.Background(), path, pattern, rmf, logger)
}

func LoadWithLoggerAndContext(ctx context.Context, path, pattern string, rmf RegisterModifierFunc, logger logging.Logger) (int, error) {
	plugins, err := luraplugin.Scan(path, pattern)
	if err != nil {
		return 0, err
	}
	return load(ctx, plugins, rmf, logger)
}

func load(ctx context.Context, plugins []string, rmf RegisterModifierFunc, logger logging.Logger) (int, error) {
	var errors []error

	loadedPlugins := 0
	for k, pluginName := range plugins {
		if err := open(ctx, pluginName, rmf, logger); err != nil {
			errors = append(errors, fmt.Errorf("plugin #%d (%s): %s", k, pluginName, err.Error()))
			continue
		}
		loadedPlugins++
	}

	if len(errors) > 0 {
		return loadedPlugins, loaderError{errors: errors}
	}
	return loadedPlugins, nil
}

func open(ctx context.Context, pluginName string, rmf RegisterModifierFunc, logger logging.Logger) (err error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	var p Plugin
	p, err = pluginOpener(pluginName)
	if err != nil {
		return
	}
	var r interface{}
	r, err = p.Lookup("ModifierRegisterer")
	if err != nil {
		return
	}
	registerer, ok := r.(Registerer)
	if !ok {
		return fmt.Errorf("modifier plugin loader: unknown type")
	}

	if logger != nil {
		if lr, ok := r.(LoggerRegisterer); ok {
			lr.RegisterLogger(logger)
		}
	}

	if lr, ok := r.(ContextRegisterer); ok {
		lr.RegisterContext(ctx)
	}

	RegisterExtraComponents(r)

	registerer.RegisterModifiers(rmf)
	return
}

var RegisterExtraComponents = func(interface{}) {}

// Plugin is the interface of the loaded plugins
type Plugin interface {
	Lookup(name string) (plugin.Symbol, error)
}

// pluginOpener keeps the plugin open function in a var for easy testing
var pluginOpener = defaultPluginOpener

func defaultPluginOpener(name string) (Plugin, error) {
	return plugin.Open(name)
}

type loaderError struct {
	errors []error
}

// Error implements the error interface
func (l loaderError) Error() string {
	msgs := make([]string, len(l.errors))
	for i, err := range l.errors {
		msgs[i] = err.Error()
	}
	return fmt.Sprintf("plugin loader found %d error(s): \n%s", len(msgs), strings.Join(msgs, "\n"))
}

func (l loaderError) Len() int {
	return len(l.errors)
}

func (l loaderError) Errs() []error {
	return l.errors
}
