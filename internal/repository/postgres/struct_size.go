// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import "reflect"

// structByteSize estimates the size of an acyclic row struct by counting its
// inline bytes and the data reached through exported fields. Slices and strings
// contribute their contents up to their length.
func structByteSize(row any) uint64 {
	value := reflect.ValueOf(row)
	return uint64(value.Type().Size()) + referencedByteSize(value)
}

func referencedByteSize(value reflect.Value) uint64 {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			elem := value.Elem()
			return uint64(elem.Type().Size()) + referencedByteSize(elem)
		}
	case reflect.String:
		return uint64(value.Len())
	case reflect.Slice:
		return uint64(value.Len())*uint64(value.Type().Elem().Size()) + elementReferencesByteSize(value)
	case reflect.Array:
		return elementReferencesByteSize(value)
	case reflect.Struct:
		var size uint64
		for i := range value.NumField() {
			if value.Type().Field(i).IsExported() {
				size += referencedByteSize(value.Field(i))
			}
		}
		return size
	}
	return 0
}

func elementReferencesByteSize(value reflect.Value) uint64 {
	switch value.Type().Elem().Kind() {
	case reflect.Pointer, reflect.Interface, reflect.String, reflect.Slice, reflect.Array, reflect.Struct:
		var size uint64
		for i := range value.Len() {
			size += referencedByteSize(value.Index(i))
		}
		return size
	default:
		return 0
	}
}

// pageByteBudget tracks the estimated size of the rows accepted into a page.
type pageByteBudget struct {
	max  uint64
	used uint64
}

func (b *pageByteBudget) add(row any) bool {
	if b.max == 0 {
		return true
	}
	size := structByteSize(row)
	if size > b.max-b.used {
		return false
	}
	b.used += size
	return true
}
