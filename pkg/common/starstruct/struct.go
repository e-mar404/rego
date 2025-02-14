// pkg/common/starstruct/struct.go
package starstruct

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// ### StarStruct Options
// ---------------------------------------------------------------------

type pkgConfig struct {
	Sort        bool
	Generate    bool
	Headers     *[]string
	ExcludeNil  bool // If true, skip generating fields for nil pointer-structs
	IncludeZero bool
}

type Option func(*pkgConfig)

// WithSortFields tells the package to sort the fields of the struct
func WithSort() Option {
	return func(cfg *pkgConfig) {
		cfg.Sort = true
	}
}

// WithGenerateFields tells the package to generate fields dynamically
func WithGenerate() Option {
	return func(cfg *pkgConfig) {
		cfg.Generate = true
	}
}

// WithHeaders tells the package to use the provided headers
func WithHeaders(headers *[]string) Option {
	return func(cfg *pkgConfig) {
		cfg.Headers = headers
	}
}

// WithExcludeNilStructs instructs the package to skip expanding fields in nil pointer-structs.
func WithExcludeNil() Option {
	return func(cfg *pkgConfig) {
		cfg.ExcludeNil = true
	}
}

// ---------------------------------------------------------------------
// Utility Functions
// ---------------------------------------------------------------------

/*
 * Print a struct as a JSON string
 */
func PrettyJSON(data interface{}) (string, error) {
	buffer := new(bytes.Buffer)
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(data)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

// ToMap converts a struct (or map) to a map[string]interface{}.
// If includeZeroValues is false then any field with a zero value is skipped.
func ToMap(item interface{}, includeZeroValues bool) (map[string]interface{}, error) {
	out := make(map[string]interface{})

	v := reflect.ValueOf(item)
	for v.Kind() == reflect.Ptr && !v.IsNil() {
		v = v.Elem()
	}

	if v.Kind() == reflect.Map {
		return mapFromMap(v), nil
	}

	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected a struct, got %s", v.Kind())
	}

	typeOfItem := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)

		// Skip unexported fields
		if !field.CanInterface() {
			continue
		}

		if !includeZeroValues && field.IsZero() {
			continue
		}

		key := getMapKey(typeOfItem.Field(i))
		if key == "" {
			key = camelKey(typeOfItem.Field(i).Name)
		}

		var value interface{}
		switch field.Kind() {
		case reflect.Struct:
			nestedMap, err := ToMap(field.Interface(), includeZeroValues)
			if err != nil {
				return nil, err
			}
			value = nestedMap
		case reflect.Slice, reflect.Array:
			sliceValues, err := sliceToInterface(field, includeZeroValues)
			if err != nil {
				return nil, err
			}
			value = sliceValues
		default:
			value = field.Interface()
		}

		out[key] = value
	}

	return out, nil
}

// sliceToInterface converts a slice/array to a []interface{}.
func sliceToInterface(v reflect.Value, includeZeroValues bool) ([]interface{}, error) {
	var result []interface{}
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		if elem.Kind() == reflect.Struct {
			nestedMap, err := ToMap(elem.Interface(), includeZeroValues)
			if err != nil {
				return nil, err
			}
			result = append(result, nestedMap)
		} else {
			result = append(result, elem.Interface())
		}
	}
	return result, nil
}

// TableToStructs converts a [][]string into a slice of structs, with the first row as headers.
func TableToStructs(data [][]string) ([]interface{}, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data is empty")
	}

	headers := data[0]
	var results []interface{}

	// Create a dynamic struct type based on headers
	var fields []reflect.StructField
	c := cases.Title(language.English)
	for _, header := range headers {
		safeHeader := c.String(strings.ReplaceAll(header, " ", ""))
		safeHeader = ensureValidIdentifier(safeHeader) // Ensure a valid Go identifier.
		fields = append(fields, reflect.StructField{
			Name: safeHeader,
			Type: reflect.TypeOf(""),
			Tag:  reflect.StructTag(fmt.Sprintf(`json:"%s"`, header)),
		})
	}
	structType := reflect.StructOf(fields)

	// Populate struct instances
	for _, row := range data[1:] {
		if len(row) != len(headers) {
			return nil, fmt.Errorf("data row does not match headers length")
		}
		instance := reflect.New(structType).Elem()
		for i, value := range row {
			instance.Field(i).SetString(value)
		}
		results = append(results, instance.Interface())
	}

	return results, nil
}

// ensureValidIdentifier makes sure the string is a valid Go identifier.
func ensureValidIdentifier(name string) string {
	if name == "" || !isLetter(rune(name[0])) {
		name = "Field" + name // Prefix to ensure it's a valid identifier
	}
	return name
}

// isLetter checks if the rune is a letter (unicode compliant)
func isLetter(ch rune) bool {
	return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ch == '_'
}

// camelKey converts the first character to lower-case.
func camelKey(s string) string {
	if len(s) == 0 {
		return s
	}

	firstChar := s[0]
	if firstChar >= 'A' && firstChar <= 'Z' {
		// ASCII, convert in place
		return string(firstChar+32) + s[1:]
	}

	// Non-ASCII, use rune conversion
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// mapFromMap converts a reflect.Value of a map into a regular map[string]interface{}.
func mapFromMap(v reflect.Value) map[string]interface{} {
	out := make(map[string]interface{})
	for _, key := range v.MapKeys() {
		out[fmt.Sprint(key.Interface())] = v.MapIndex(key).Interface()
	}
	return out
}

// ---------------------------------------------------------------------
// Flattening and Field-Generation Functions
// ---------------------------------------------------------------------

// FlattenStructFields recursively flattens a struct and its nested fields into a two-dimensional slice.
func FlattenStructFields(item interface{}, opts ...Option) ([][]string, error) {
	// Default config
	cfg := &pkgConfig{
		Sort:     false,
		Generate: false,
		Headers:  nil,
	}

	// Process options
	for _, opt := range opts {
		opt(cfg)
	}

	val, err := DerefPointers(reflect.ValueOf(item))
	if err != nil {
		return nil, err
	}

	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected a struct or pointer to a struct, got %v", val.Kind())
	}

	// Dynamically generate headers (if requested)
	if cfg.Generate && (cfg.Headers == nil || len(*cfg.Headers) == 0) {
		cfg.Headers = &[]string{}
		generatedFields, err := GenerateFieldNames("", val)
		if err != nil {
			return nil, err
		}
		*cfg.Headers = append(*cfg.Headers, *generatedFields...)
	}

	// Build a map to hold flattened field names and their values
	fieldMap := make(map[string]string)
	err = FlattenNestedStructs(item, "", &fieldMap)
	if err != nil {
		return nil, err
	}

	// If not generating, limit the output to only the provided headers.
	if !cfg.Generate {
		newMap := make(map[string]string, len(fieldMap))
		headerSet := make(map[string]struct{}, len(*cfg.Headers))
		for _, field := range *cfg.Headers {
			headerSet[field] = struct{}{}
		}

		for key, value := range fieldMap {
			for field := range headerSet {
				if key == field || strings.HasPrefix(key, field+".") {
					newMap[key] = value
					break
				}
			}
		}

		// If any header is missing, add it with an empty string.
		if len(newMap) < len(*cfg.Headers) {
			for _, header := range *cfg.Headers {
				if _, ok := newMap[header]; !ok {
					newMap[header] = ""
				}
			}
		}
		fieldMap = newMap
	}

	// Convert the fieldMap into a 2D slice (field and value) while updating headers.
	fieldSlice, err := mapToSliceAndUpdateFields(&fieldMap, cfg.Headers)
	if err != nil {
		return nil, err
	}

	return fieldSlice, nil
}

// GenerateFieldNames recursively generates field names from a struct (or slice/map thereof), dereferencing pointers as needed.
func GenerateFieldNames(prefix string, val reflect.Value, opts ...Option) (*[]string, error) {
	cfg := &pkgConfig{
		Sort:       false,
		Generate:   false,
		Headers:    nil,
		ExcludeNil: false,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	var err error
	val, err = DerefPointers(val)
	if err != nil {
		return nil, err
	}

	// Prepare the field names slice
	fields := make([]string, 0)

	switch val.Kind() {
	case reflect.Map:
		mapFields, err := generateMapFieldNames(prefix, val)
		if err != nil {
			return nil, err
		}
		fields = append(fields, *mapFields...)
		return &fields, nil

	case reflect.Slice, reflect.Array:
		if val.Len() == 0 {
			return nil, fmt.Errorf("GenerateFieldNames: empty slice or array")
		}

		var mergedFields []string
		// Use the first non-nil candidate as the baseline.
		for i := 0; i < val.Len(); i++ {
			mergeCandidate := val.Index(i)
			if (mergeCandidate.Kind() == reflect.Ptr || mergeCandidate.Kind() == reflect.Interface) && mergeCandidate.IsNil() {
				continue
			}
			mergeCandidate, err = DerefPointers(mergeCandidate)
			if err != nil {
				return nil, err
			}
			subFieldsPtr, err := GenerateFieldNames(prefix, mergeCandidate, opts...)
			if err != nil {
				return nil, err
			}
			mergedFields = *subFieldsPtr
			break
		}
		// Now, for every candidate, merge in its field names into the baseline.
		for i := 0; i < val.Len(); i++ {
			mergeCandidate := val.Index(i)
			if (mergeCandidate.Kind() == reflect.Ptr || mergeCandidate.Kind() == reflect.Interface) && mergeCandidate.IsNil() {
				continue
			}
			mergeCandidate, err = DerefPointers(mergeCandidate)
			if err != nil {
				return nil, err
			}
			subFieldsPtr, err := GenerateFieldNames(prefix, mergeCandidate, opts...)
			if err != nil {
				return nil, err
			}
			mergedFields = MergeFields(mergedFields, *subFieldsPtr)
		}
		// If still empty, fall back to a zero value.
		if len(mergedFields) == 0 {
			mergeCandidate := reflect.Zero(val.Type().Elem())
			subFieldsPtr, err := GenerateFieldNames(prefix, mergeCandidate, opts...)
			if err != nil {
				return nil, err
			}
			mergedFields = *subFieldsPtr
		}
		return &mergedFields, nil

	case reflect.Struct:
		typ := val.Type()

		// Handle struct fields
		for i := 0; i < val.NumField(); i++ {
			field := typ.Field(i)
			fieldVal := val.Field(i)
			jsonTag := getFirstTag(field.Tag.Get("json"))

			// If the type of the struct itself is time.Time and it's not an embedded field, add it to the fields
			switch {
			case field.Type.String() == "time.Time" && !field.Anonymous:
				fields = append(fields, jsonTag)
				continue
			}

			// Skip ignored field
			if jsonTag == "-" {
				continue
			}

			// Exclude nil pointer expansions, if set
			if cfg.ExcludeNil {
				if (fieldVal.Kind() == reflect.Ptr || fieldVal.Kind() == reflect.Interface) && fieldVal.IsNil() {
					continue
				}
			}

			fieldKey := joinPrefixKey(prefix, jsonTag)

			// Recursively handle nested structs and inline structs if specified
			if shouldInline(field) {
				subFields, err := GenerateFieldNames(prefix, val.Field(i), opts...)
				if err != nil {
					return nil, err
				}
				fields = append(fields, *subFields...)
			} else if field.Type.Kind() == reflect.Struct ||
				(field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct) {
				subPrefix := fieldKey
				subFields, err := GenerateFieldNames(subPrefix, val.Field(i), opts...)
				if err != nil {
					return nil, err
				}
				fields = append(fields, *subFields...)
			} else if field.Type.Kind() == reflect.Map ||
				(field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Map) {
				subFields, err := generateMapFieldNames(fieldKey, val.Field(i))
				if err != nil {
					return nil, err
				}
				fields = append(fields, *subFields...)
			} else {
				fields = append(fields, fieldKey)
			}
		}
		return &fields, nil

	case reflect.Interface:
		if val.IsNil() {
			return &fields, nil
		}
		return GenerateFieldNames(prefix, val.Elem(), opts...)
	case reflect.Ptr:
		if val.IsNil() {
			return &fields, nil
		}
		return GenerateFieldNames(prefix, val.Elem(), opts...)
	case reflect.Invalid:
		return &[]string{prefix}, nil
	default:
		return nil, fmt.Errorf("GenerateFieldNames: unsupported input type: %v", val.Kind())
	}
}

// generateMapFieldNames generates field names from a map value.
// The keys are sorted to ensure deterministic ordering.
func generateMapFieldNames(prefix string, val reflect.Value) (*[]string, error) {
	var err error
	val, err = DerefPointers(val)
	if err != nil {
		return nil, err
	}

	if val.Kind() != reflect.Map {
		return nil, fmt.Errorf("generateMapFieldNames: expected a map, got %v", val.Kind())
	}

	var fields []string

	// Sort map keys for consistent ordering.
	keys := val.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
	})

	for _, key := range keys {
		keyStr := fmt.Sprint(key.Interface())
		fieldKey := joinPrefixKey(prefix, keyStr)
		value := val.MapIndex(key)
		switch value.Kind() {
		case reflect.Map, reflect.Struct:
			subFields, err := GenerateFieldNames(fieldKey, value)
			if err != nil {
				return nil, err
			}
			fields = append(fields, *subFields...)
		default:
			fields = append(fields, fieldKey)
		}
	}
	return &fields, nil
}

// DerefPointers takes a reflect.Value and recursively dereferences it if it's a pointer.
func DerefPointers(val reflect.Value) (reflect.Value, error) {
	for val.Kind() == reflect.Pointer || val.Kind() == reflect.Interface {
		if val.IsNil() {
			// Return the relevant nil value for the current type, "" for string, 0 for int, etc.
			switch val.Kind() {
			case reflect.String:
				return reflect.ValueOf(""), nil
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return reflect.ValueOf(0), nil
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return reflect.ValueOf(uint(0)), nil
			case reflect.Float32, reflect.Float64:
				return reflect.ValueOf(float64(0)), nil
			case reflect.Bool:
				return reflect.ValueOf(false), nil
			default:
				//return reflect.Value{}, fmt.Errorf("DerefPointers: nil pointer/interface element")
			}
		}
		if val.Kind() == reflect.Pointer {
			val = val.Elem()
		}
		if val.Kind() == reflect.Interface {
			val = val.Elem()
		}
	}
	return val, nil
}

// FlattenNestedStructs recursively flattens a struct (and its nested fields) into a map.
// The keys are generated using the provided prefix.
func FlattenNestedStructs(item interface{}, prefix string, fieldMap *map[string]string) error {
	val, err := DerefPointers(reflect.ValueOf(item))
	if err != nil {
		return err
	}

	typ := val.Type()

	// For non-struct types, handle maps or slices separately.
	if val.Kind() != reflect.Struct {
		if val.Kind() == reflect.Map {
			return flattenMap(val, prefix, fieldMap)
		}
		if val.Kind() == reflect.Slice {
			return flattenSlice(val, prefix, fieldMap)
		}
		return fmt.Errorf("expected a struct or pointer to a struct, got %v", val.Kind())
	}

	// Iterate over struct fields
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		keyPrefix := joinPrefixKey(prefix, getMapKey(field))

		switch fieldVal.Kind() {
		case reflect.Slice:
			if fieldVal.Len() == 0 {
				(*fieldMap)[keyPrefix] = "" // Handle empty slice
			} else {
				err := flattenSlice(fieldVal, keyPrefix, fieldMap)
				if err != nil {
					return err
				}
			}
		case reflect.Struct:
			// If the type of the struct itself is time.Time and it's not an embedded field, add it to the map
			switch {
			case field.Type.String() == "time.Time" && !field.Anonymous:
				(*fieldMap)[keyPrefix] = fmt.Sprint(fieldVal.Interface())
				continue
			}

			// Check if the struct should be inlined
			if shouldInline(field) {
				err := FlattenNestedStructs(fieldVal.Interface(), prefix, fieldMap)
				if err != nil {
					return err
				}
			} else {
				// Recursively handle nested structs
				err := FlattenNestedStructs(fieldVal.Interface(), keyPrefix, fieldMap)
				if err != nil {
					return err
				}
			}
		case reflect.Interface:
			if !fieldVal.IsNil() {
				elem := fieldVal.Elem()
				if elem.Kind() == reflect.Struct {
					err := FlattenNestedStructs(elem.Interface(), prefix, fieldMap)
					if err != nil {
						return err
					}
				}
			}
		case reflect.Map:
			if fieldVal.Len() == 0 {
				(*fieldMap)[keyPrefix] = ""
			} else {
				if shouldInline(field) {
					err := flattenMap(fieldVal, prefix, fieldMap)
					if err != nil {
						return err
					}
				} else {
					err := flattenMap(fieldVal, keyPrefix, fieldMap)
					if err != nil {
						return err
					}
				}
			}
		case reflect.Ptr:
			if fieldVal.IsNil() {
				(*fieldMap)[keyPrefix] = "<nil>"
			} else {
				underlying := fieldVal.Elem()
				switch underlying.Kind() {
				case reflect.Struct:
					if shouldInline(field) {
						err = FlattenNestedStructs(underlying.Interface(), prefix, fieldMap)
					} else {
						err = FlattenNestedStructs(underlying.Interface(), keyPrefix, fieldMap)
					}
				case reflect.Map, reflect.Slice, reflect.Array:
					err = FlattenNestedStructs(underlying.Interface(), keyPrefix, fieldMap)
				default:
					(*fieldMap)[keyPrefix] = fmt.Sprint(underlying.Interface())
				}
				if err != nil {
					return err
				}
			}
		default:
			if fieldVal.IsValid() {
				(*fieldMap)[keyPrefix] = fmt.Sprint(fieldVal.Interface())
			} else {
				(*fieldMap)[keyPrefix] = "<nil>"
			}
		}
	}

	return nil
}

// joinPrefixKey joins a prefix and key, helping to avoid extra trailing dots.
func joinPrefixKey(prefix, key string) string {
	switch {
	case prefix == "" && key == "":
		return ""
	case prefix == "":
		return key
	case key == "":
		return prefix
	default:
		return prefix + "." + key
	}
}

/*
* shouldInline reports whether the field should be embedded, making it appear as if it belongs to the parent struct.
* It returns true if the field has the "inline" tag.

* Example:
* Field: profile.customAttributes `json:",inline"`
* profile.customAttributes.key1 ==> profile.key1
*
* as opposed to:
*
* Field: profile.customAttributes `json:"customAttributes,omitempty"`
* profile.customAttributes.key1 ==> profile.customAttributes.key1
 */
func shouldInline(field reflect.StructField) bool {
	tag := field.Tag.Get("json")
	return strings.Contains(tag, ",inline")
}

// flattenSlice flattens a slice field.
// It computes the index format (with a minimum width of 2 digits) for consistent ordering.
func flattenSlice(slice reflect.Value, keyPrefix string, fieldMap *map[string]string) error {
	width := len(strconv.Itoa(slice.Len() - 1))
	if width < 2 {
		width = 2
	}
	indexFormat := fmt.Sprintf("%%0%dd", width)

	for j := 0; j < slice.Len(); j++ {
		elem := slice.Index(j)
		elemKey := joinPrefixKey(keyPrefix, fmt.Sprintf(indexFormat, j))
		if elem.Kind() == reflect.Struct {
			// Recursively handle struct elements in a slice
			err := FlattenNestedStructs(elem.Interface(), elemKey, fieldMap)
			if err != nil {
				return err
			}
		} else {
			(*fieldMap)[elemKey] = fmt.Sprint(elem.Interface())
		}
	}
	return nil
}

// flattenMap flattens a map field. The keys are sorted to guarantee a deterministic order.
func flattenMap(m reflect.Value, prefix string, fieldMap *map[string]string) error {
	keys := m.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
	})
	for _, key := range keys {
		keyStr := fmt.Sprint(key.Interface())
		newKey := joinPrefixKey(prefix, keyStr)

		value := m.MapIndex(key)
		value, err := DerefPointers(value)
		if err != nil {
			return err
		}

		switch value.Kind() {
		case reflect.Map, reflect.Struct, reflect.Slice, reflect.Array:
			err := FlattenNestedStructs(value.Interface(), newKey, fieldMap)
			if err != nil {
				return err
			}
		default:
			(*fieldMap)[newKey] = fmt.Sprint(value.Interface())
		}
	}

	return nil
}

// getFirstTag extracts the first comma-separated part of a tag.
func getFirstTag(tag string) string {
	return strings.Split(tag, ",")[0]
}

// getMapKey determines the key to use based on the field’s tags.
func getMapKey(field reflect.StructField) string {
	jsonTag := getFirstTag(field.Tag.Get("json"))
	urlTag := getFirstTag(field.Tag.Get("url"))
	xmlTag := getFirstTag(field.Tag.Get("xml"))
	mapKey := field.Name

	if jsonTag != "" && jsonTag != "-" {
		mapKey = jsonTag
	} else if urlTag != "" && urlTag != "-" {
		mapKey = urlTag
	} else if xmlTag != "" && xmlTag != "-" {
		mapKey = xmlTag
	} else {
		mapKey = camelKey(mapKey)
	}

	return mapKey
}

// mapToSliceAndUpdateFields converts the internal field map into a 2D slice
// and updates the headers. It groups keys under each header and sorts them
// using a custom comparator that is numeric-aware.
func mapToSliceAndUpdateFields(fieldMap *map[string]string, headers *[]string) ([][]string, error) {
	var newFields []string
	var fieldSlice [][]string

	// custom sort function for keys within a header group.
	sortGroupKeys := func(header string, keys []string) {
		sort.Slice(keys, func(i, j int) bool {
			a := keys[i]
			b := keys[j]
			// If one key exactly equals the header, it should come first.
			if a == header && b != header {
				return true
			}
			if b == header && a != header {
				return false
			}
			// Both keys should start with header+"."
			aSuffix := strings.TrimPrefix(a, header+".")
			bSuffix := strings.TrimPrefix(b, header+".")
			partsA := strings.SplitN(aSuffix, ".", 2)
			partsB := strings.SplitN(bSuffix, ".", 2)
			// If the first parts are numeric, compare as numbers.
			nA, errA := strconv.Atoi(partsA[0])
			nB, errB := strconv.Atoi(partsB[0])
			if errA == nil && errB == nil {
				if nA != nB {
					return nA < nB
				}
				// If numeric parts are equal, compare any additional suffix.
				if len(partsA) > 1 && len(partsB) > 1 {
					return partsA[1] < partsB[1]
				}
				return len(partsA) < len(partsB)
			}
			// Fallback to lexicographic.
			return a < b
		})
	}

	// Process each header in the order provided.
	for _, header := range *headers {
		var group []string
		for key := range *fieldMap {
			if key == header || strings.HasPrefix(key, header+".") {
				group = append(group, key)
			}
		}
		if len(group) > 0 {
			sortGroupKeys(header, group)
			for _, key := range group {
				newFields = append(newFields, key)
				fieldSlice = append(fieldSlice, []string{key, (*fieldMap)[key]})
			}
		}
	}

	// Process any keys not matched by the provided headers.
	var leftovers []string
	for key := range *fieldMap {
		found := false
		for _, header := range *headers {
			if key == header || strings.HasPrefix(key, header+".") {
				found = true
				break
			}
		}
		if !found {
			leftovers = append(leftovers, key)
		}
		// Use lexicographical order for leftovers.
	}
	sort.Slice(leftovers, func(i, j int) bool { return leftovers[i] < leftovers[j] })
	for _, key := range leftovers {
		newFields = append(newFields, key)
		fieldSlice = append(fieldSlice, []string{key, (*fieldMap)[key]})
	}

	*headers = newFields
	return fieldSlice, nil
}
