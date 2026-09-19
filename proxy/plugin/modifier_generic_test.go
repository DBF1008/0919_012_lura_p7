// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"errors"
	"testing"
)

type genericTestWrapper struct{ value string }

func (w genericTestWrapper) Value() string { return w.value }

type genericTestWrapperIface interface{ Value() string }

func TestGenericRegister_roundTrip(t *testing.T) {
	factory := func(_ map[string]interface{}) Modifier[genericTestWrapperIface] {
		return func(w genericTestWrapperIface) (genericTestWrapperIface, error) {
			return genericTestWrapper{value: w.Value() + "-modified"}, nil
		}
	}

	RegisterModifierFactory[genericTestWrapperIface]("generic-roundtrip", factory, true, true)

	for _, get := range []func(string) (ModifierFactory[genericTestWrapperIface], bool){
		GetRequestModifier[genericTestWrapperIface],
		GetResponseModifier[genericTestWrapperIface],
	} {
		mf, ok := get("generic-roundtrip")
		if !ok {
			t.Fatal("modifier factory not found in the register")
		}
		res, err := mf(map[string]interface{}{})(genericTestWrapper{value: "input"})
		if err != nil {
			t.Fatal(err)
		}
		if got := res.Value(); got != "input-modified" {
			t.Errorf("unexpected result. have %s, want input-modified", got)
		}
	}
}

func TestGenericRegister_legacyCompatibility(t *testing.T) {
	RegisterModifier("generic-legacy", func(_ map[string]interface{}) func(interface{}) (interface{}, error) {
		return func(input interface{}) (interface{}, error) {
			w, ok := input.(genericTestWrapperIface)
			if !ok {
				return nil, errors.New("unknown type")
			}
			return genericTestWrapper{value: w.Value() + "-legacy"}, nil
		}
	}, true, false)

	mf, ok := GetRequestModifier[genericTestWrapperIface]("generic-legacy")
	if !ok {
		t.Fatal("modifier factory not found in the register")
	}
	res, err := mf(map[string]interface{}{})(genericTestWrapper{value: "input"})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Value(); got != "input-legacy" {
		t.Errorf("unexpected result. have %s, want input-legacy", got)
	}
}

func TestGenericRegister_legacyWrongTypeIsDiscarded(t *testing.T) {
	RegisterModifier("generic-legacy-wrong-type", func(_ map[string]interface{}) func(interface{}) (interface{}, error) {
		return func(_ interface{}) (interface{}, error) {
			return "not a wrapper", nil
		}
	}, true, false)

	mf, ok := GetRequestModifier[genericTestWrapperIface]("generic-legacy-wrong-type")
	if !ok {
		t.Fatal("modifier factory not found in the register")
	}
	input := genericTestWrapper{value: "input"}
	res, err := mf(map[string]interface{}{})(input)
	if err != nil {
		t.Fatal(err)
	}
	if res != genericTestWrapperIface(input) {
		t.Errorf("unexpected result. have %v, want the previous value", res)
	}
}

func TestGenericRegister_legacyErrorIsPropagated(t *testing.T) {
	errExpected := errors.New("legacy error")
	RegisterModifier("generic-legacy-error", func(_ map[string]interface{}) func(interface{}) (interface{}, error) {
		return func(_ interface{}) (interface{}, error) {
			return nil, errExpected
		}
	}, true, false)

	mf, ok := GetRequestModifier[genericTestWrapperIface]("generic-legacy-error")
	if !ok {
		t.Fatal("modifier factory not found in the register")
	}
	if _, err := mf(map[string]interface{}{})(genericTestWrapper{}); err != errExpected {
		t.Errorf("unexpected error. have %v, want %v", err, errExpected)
	}
}

func TestGenericRegister_nilModifierIsKept(t *testing.T) {
	RegisterModifierFactory[genericTestWrapperIface]("generic-nil", func(_ map[string]interface{}) Modifier[genericTestWrapperIface] {
		return nil
	}, true, false)

	mf, ok := GetRequestModifier[genericTestWrapperIface]("generic-nil")
	if !ok {
		t.Fatal("modifier factory not found in the register")
	}
	if m := mf(map[string]interface{}{}); m != nil {
		t.Errorf("unexpected modifier. have %v, want nil", m)
	}
}

func TestGenericRegister_notFound(t *testing.T) {
	if _, ok := GetRequestModifier[genericTestWrapperIface]("generic-unknown"); ok {
		t.Error("unexpected modifier factory for an unknown name")
	}
	if _, ok := GetResponseModifier[genericTestWrapperIface]("generic-roundtrip-unknown"); ok {
		t.Error("unexpected modifier factory for an unknown name")
	}
}
