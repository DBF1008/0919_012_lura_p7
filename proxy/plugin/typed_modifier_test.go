// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"errors"
	"testing"
)

func TestRegisterTypedModifier(t *testing.T) {
	RegisterTypedModifier[string]("typed-unit-test-request",
		func(_ map[string]interface{}) func(string) (string, error) {
			return func(v string) (string, error) {
				return v + "-modified", nil
			}
		},
		true, false,
	)

	mf, ok := GetRequestModifier[string]("typed-unit-test-request")
	if !ok {
		t.Fatal("typed modifier factory not found in the request register")
	}

	res, err := mf(map[string]interface{}{})("input")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if res != "input-modified" {
		t.Errorf("unexpected result. have %s, want input-modified", res)
	}

	if _, ok := GetResponseModifier[string]("typed-unit-test-request"); ok {
		t.Error("the modifier should not be registered in the response namespace")
	}
}

func TestGetModifier_legacyFactoryAdaptation(t *testing.T) {
	RegisterModifier("legacy-unit-test-request",
		func(_ map[string]interface{}) func(interface{}) (interface{}, error) {
			return func(v interface{}) (interface{}, error) {
				return v.(string) + "-legacy", nil
			}
		},
		true, false,
	)

	mf, ok := GetRequestModifier[string]("legacy-unit-test-request")
	if !ok {
		t.Fatal("legacy modifier factory not found in the request register")
	}

	res, err := mf(map[string]interface{}{})("input")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if res != "input-legacy" {
		t.Errorf("unexpected result. have %s, want input-legacy", res)
	}
}

func TestGetModifier_legacyFactoryDiscardsIncompatibleResults(t *testing.T) {
	RegisterModifier("legacy-unit-test-incompatible",
		func(_ map[string]interface{}) func(interface{}) (interface{}, error) {
			return func(_ interface{}) (interface{}, error) {
				return 42, nil
			}
		},
		true, false,
	)

	mf, ok := GetRequestModifier[string]("legacy-unit-test-incompatible")
	if !ok {
		t.Fatal("legacy modifier factory not found in the request register")
	}

	res, err := mf(map[string]interface{}{})("input")
	if err != nil {
		t.Error(err.Error())
		return
	}
	if res != "input" {
		t.Errorf("incompatible results must keep the previous value. have %s, want input", res)
	}
}

func TestGetModifier_legacyFactoryPropagatesErrors(t *testing.T) {
	expectedErr := errors.New("legacy error")
	RegisterModifier("legacy-unit-test-error",
		func(_ map[string]interface{}) func(interface{}) (interface{}, error) {
			return func(_ interface{}) (interface{}, error) {
				return nil, expectedErr
			}
		},
		false, true,
	)

	mf, ok := GetResponseModifier[string]("legacy-unit-test-error")
	if !ok {
		t.Fatal("legacy modifier factory not found in the response register")
	}

	if _, err := mf(map[string]interface{}{})("input"); err != expectedErr {
		t.Errorf("unexpected error. have %v, want %v", err, expectedErr)
	}
}

func TestGetModifier_unknownName(t *testing.T) {
	if _, ok := GetRequestModifier[string]("unknown-modifier-name"); ok {
		t.Error("no modifier factory should be registered under this name")
	}
	if _, ok := GetResponseModifier[string]("unknown-modifier-name"); ok {
		t.Error("no modifier factory should be registered under this name")
	}
}
