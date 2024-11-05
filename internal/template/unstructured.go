package template

import (
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// A function type for applying changes to string fields. Accepts the field
// current value as a string and returns a new value, a boolean indicating if to
// apply the new value to the original object and an error value that should be
// returned from the calling function if not nil
type fieldFunc func(string) (string, bool, error)

// Given a possibly nested map structure, navigate to a particular scalar value
// using path - a list of string keys. Then treat that value as a template and
// apply it in-place while using the provided values.
func applyFieldTemplate(obj map[string]any, path []string, values map[string]string) error {
	return applyFieldFunc(obj, path, func(valueTemplate string) (string, bool, error) {
		value, err := executeTemplate(valueTemplate, values)
		return value, true, err
	})
}

func applyFieldFunc(obj map[string]any, path []string, ff fieldFunc) error {
	if ind := slices.Index(path, "[]"); ind != -1 {
		if ind == len(path)-1 {
			return applySliceFieldFunc(obj, path[:ind], ff)
		}
		return processNestedSlice(obj, path, ff)
	} else {
		return applyPlainFieldFunc(obj, path, ff)
	}
}

func applyPlainFieldFunc(obj map[string]any, path []string, ff fieldFunc) error {
	existingValue, ok, err := unstructured.NestedString(obj, path...)
	if err != nil {
		return fmt.Errorf("error reading object: %s", err)
	}
	if !ok {
		// If the path is not found in the structure, we ignore it
		return nil
	}
	value, set, err := ff(existingValue)
	if err != nil {
		return err
	}
	if set {
		err = unstructured.SetNestedField(obj, value, path...)
		if err != nil {
			return fmt.Errorf("error updating object: %s", err)
		}
	}
	return nil
}

func applySliceFieldFunc(obj map[string]any, path []string, ff fieldFunc) error {
	exValArr, ok, err := unstructured.NestedStringSlice(obj, path...)
	if err != nil {
		return fmt.Errorf("error reading object: %s", err)
	}
	if !ok {
		// If the path is not found in the structure, we ignore it
		return nil
	}
	valueArr := make([]string, len(exValArr))
	var setAny bool
	for i, existingValue := range exValArr {
		value, set, err := ff(existingValue)
		if err != nil {
			return err
		}
		if set {
			valueArr[i] = value
		} else {
			valueArr[i] = existingValue
		}
		setAny = setAny || set
	}
	if setAny {
		err = unstructured.SetNestedStringSlice(obj, valueArr, path...)
		if err != nil {
			return fmt.Errorf("error updating object: %s", err)
		}
	}
	return nil
}

func processNestedSlice(obj map[string]any, path []string, ff fieldFunc) error {
	ind := slices.Index(path, "[]")
	nestedSlice, found, err := unstructured.NestedSlice(obj, path[:ind]...)
	// May return error if same key points to map in one place and to string in another
	if !found || err != nil {
		return nil
	}
	updatedNestedSlice := make([]any, len(nestedSlice))

	for i, nestedObj := range nestedSlice {
		err := applyFieldFunc(nestedObj.(map[string]any), path[ind+1:], ff)
		if err != nil {
			return err
		}
		updatedNestedSlice[i] = nestedObj
	}

	err = unstructured.SetNestedSlice(obj, updatedNestedSlice, path[:ind]...)
	if err != nil {
		return err
	}

	return nil
}
