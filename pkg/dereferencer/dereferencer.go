package dereferencer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"
)

type dereferencer interface {
	Dereference(ref string, document []byte) error
}

const (
	inFileRef = "#"
	httpRef   = "http://"
)

func trimFirstRune(s string) string {
	_, i := utf8.DecodeRuneInString(s)
	return s[i:]
}

func eachJSONValue(obj *interface{}, handler func(*string, *int, *interface{})) {
	if obj == nil {
		return
	}
	// Yield all key/value pairs for objects.
	o, isObject := (*obj).(map[string]interface{})
	if isObject {
		for k, v := range o {
			handler(&k, nil, &v)
			eachJSONValue(&v, handler)
		}
	}
	// Yield each index/value for arrays.
	a, isArray := (*obj).([]interface{})
	if isArray {
		for i, x := range a {
			handler(nil, &i, &x)
			eachJSONValue(&x, handler)
		}
	}
	// Do nothing for primitives since the handler got them.
}

// Dereference resolves all references in the document
func Dereference(document []byte) (resolvedDoc []byte, err error) {
	var objmap map[string]interface{}
	err = json.Unmarshal(document, &objmap)
	if err != nil {
		return nil, err
	}
	var replacements = make(map[string]interface{})
	replacements, err = resolve(objmap, document)
	if err != nil {
		return nil, err
	}
	// Replace strings for its references
	// hardcoded value: 10 loops to resolve $ref inside $refs.
	for i := 1; i <= 10; i++ {
		for k, v := range replacements {
			key := fmt.Sprintf("{\"$ref\":\"%s\"}", k)
			find := []byte(key)
			document = bytes.Replace(document, find, v.([]byte), -1)
		}
	}

	resolvedDoc = document
	return resolvedDoc, nil
}

func resolve(objmap map[string]interface{}, document []byte) (replacements map[string]interface{}, err error) {
	replacements = make(map[string]interface{})
	fDef := fileDereferencer{}
	httpDef := httpDereferencer{}
	for _, v := range objmap {
		eachJSONValue(&v, func(key *string, index *int, value *interface{}) {
			if key != nil { // It's an object key/value pair...
				if *key == "$ref" {
					if strings.HasPrefix((*value).(string), inFileRef) {
						dv, err := fDef.Dereference((*value).(string), document)
						if err != nil {
							log.Fatal(err)
						}
						// TODO: Substitute obj for dereferencedValue(dv)
						// or use this dvs to generate another document
						replacements[(*value).(string)] = dv
					} else if strings.HasPrefix((*value).(string), httpRef) {
						urlData, ref, err := resolveURL((*value).(string))
						if err != nil {
							log.Fatal(err)
						}
						var dv []byte
						if ref == "" {
							dv, err = httpDef.Dereference((*value).(string), urlData)
						} else {
							dv, err = httpDef.Dereference(ref, urlData)
						}
						if err != nil {
							log.Fatal(err)
						}
						replacements[(*value).(string)] = dv
					} else {
						fileData, ref, err := checkFile((*value).(string))
						if err != nil {
							log.Fatal(err)
						}
						var dv []byte
						if ref == "" {
							dv, err = fDef.Dereference((*value).(string), fileData)
						} else {
							dv, err = fDef.Dereference(ref, fileData)
						}
						if err != nil {
							log.Fatal(err)
						}
						replacements[(*value).(string)] = dv
					}
				}
			}
		})

	}
	return
}