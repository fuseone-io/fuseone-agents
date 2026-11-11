package judge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

/*
strictJSON refuses what a decoder into Go values would quietly forgive (NT-016).

encoding/json keeps the last of two keys of one name and reads null as an absent field,
and a closed contract must do neither: a key written twice is two declarations, and a
field written as null was written. This walks the tokens once, refusing a duplicate key
in any object, a null anywhere, and data after the value.
*/
func strictJSON(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := strictValue(dec); err != nil {
		return err
	}
	if _, err := dec.Token(); err == nil {
		return errors.New("data after the value")
	}
	return nil
}

func strictValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch tok {
	case nil:
		return errors.New("null is not a declaration")
	case json.Delim('{'):
		return strictObject(dec)
	case json.Delim('['):
		for dec.More() {
			if err := strictValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	}
	return nil
}

func strictObject(dec *json.Decoder) error {
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		if seen[key] {
			return fmt.Errorf("key %q appears twice", key)
		}
		seen[key] = true
		if err := strictValue(dec); err != nil {
			return err
		}
	}
	_, err := dec.Token()
	return err
}
