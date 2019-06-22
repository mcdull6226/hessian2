// Copyright 2016-2019 Alex Stocks, Wongoo
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hessian

import (
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
)

import (
	perrors "github.com/pkg/errors"
)

var (
	listTypeNameMapper = map[string]string{
		"string": "[string",

		"int8": "[short",

		"int16":  "[short",
		"uint16": "[short",

		"int32":  "[int",
		"uint32": "[int",

		"int":  "[long",
		"uint": "[long",

		"int64":  "[long",
		"uint64": "[long",

		"float32": "[float",
		"float64": "[double",

		"bool": "[boolean",

		"time.Time": "[date",
	}
	listTypeMapper = map[string]reflect.Type{
		"string":           reflect.TypeOf(""),
		"java.lang.String": reflect.TypeOf(""),
		"char":             reflect.TypeOf(""),
		"short":            reflect.TypeOf(int32(0)),
		"int":              reflect.TypeOf(int32(0)),
		"long":             reflect.TypeOf(int64(0)),
		"float":            reflect.TypeOf(float64(0)),
		"double":           reflect.TypeOf(float64(0)),
		"boolean":          reflect.TypeOf(true),
		"java.util.Date":   reflect.TypeOf(time.Time{}),
		"date":             reflect.TypeOf(time.Time{}),
	}
	noType = perrors.New("no type name.")
)

func registerTypeName(gotype, javatype string) {
	listTypeNameMapper[gotype] = "[" + javatype
}

func getType(javalistname string) reflect.Type {
	javaname := javalistname
	if strings.Index(javaname, "[") == 0 {
		javaname = javaname[1:]
	}
	if strings.Index(javaname, "[") == 0 {
		return reflect.SliceOf(getType(javaname))
	}

	var sliceTy reflect.Type
	ltm := listTypeMapper[javaname]
	if ltm != nil {
		sliceTy = reflect.SliceOf(ltm)
	}

	if sliceTy == nil {
		tpStructInfo, _ := getStructInfo(javaname)
		tp := tpStructInfo.typ
		if tp == nil {
			return nil
		}
		if tp.Kind() != reflect.Ptr {
			tp = reflect.New(tp).Type()
		}
		sliceTy = reflect.SliceOf(tp)
	}
	return sliceTy
}

/////////////////////////////////////////
// List
/////////////////////////////////////////

// encList write list
func (e *Encoder) encList(v interface{}) error {
	if reflect.TypeOf(v).String() != "[]interface {}" {
		//if err := e.writeTypedList(v); err != noType {
		//	return err
		//}
		// todo support multidimensional arrays? object?
		return e.writeTypedList(v)
	}
	return e.writeUntypedList(v)
}

// writeTypedList write typed list
// Include 3 formats:
// ::= x55 type value* 'Z'   # variable-length list
// ::= 'V' type int value*   # fixed-length list
// ::= [x70-77] type value*  # fixed-length typed list
func (e *Encoder) writeTypedList(v interface{}) error {
	var (
		err error
	)

	value := reflect.ValueOf(v)

	// check ref
	if n, ok := e.checkRefMap(value); ok {
		e.buffer = encRef(e.buffer, n)
		return nil
	}

	value = UnpackPtrValue(value)
	var typeName = listTypeNameMapper[UnpackPtrType(value.Type().Elem()).String()]
	if typeName == "" {
		return noType
	}

	e.buffer = encByte(e.buffer, BC_LIST_FIXED) // 'V'
	e.buffer = encString(e.buffer, typeName)
	e.buffer = encInt32(e.buffer, int32(value.Len()))
	for i := 0; i < value.Len(); i++ {
		if err = e.Encode(value.Index(i).Interface()); err != nil {
			return err
		}
	}

	return nil
}

// writeUntypedList write untyped list
// Include 3 formats:
// ::= x57 value* 'Z'        # variable-length untyped list
// ::= x58 int value*        # fixed-length untyped list
// ::= [x78-7f] value*       # fixed-length untyped list
func (e *Encoder) writeUntypedList(v interface{}) error {
	var (
		err error
	)

	value := reflect.ValueOf(v)

	// check ref
	if n, ok := e.checkRefMap(value); ok {
		e.buffer = encRef(e.buffer, n)
		return nil
	}

	value = UnpackPtrValue(value)

	e.buffer = encByte(e.buffer, BC_LIST_FIXED_UNTYPED) // x58
	e.buffer = encInt32(e.buffer, int32(value.Len()))
	for i := 0; i < value.Len(); i++ {
		if err = e.Encode(value.Index(i).Interface()); err != nil {
			return err
		}
	}

	return nil
}

/////////////////////////////////////////
// List
/////////////////////////////////////////

// # list/vector
// ::= x55 type value* 'Z'   # variable-length list
// ::= 'V' type int value*   # fixed-length list
// ::= x57 value* 'Z'        # variable-length untyped list
// ::= x58 int value*        # fixed-length untyped list
// ::= [x70-77] type value*  # fixed-length typed list
// ::= [x78-7f] value*       # fixed-length untyped list

func (d *Decoder) readBufByte() (byte, error) {
	var (
		err error
		buf [1]byte
	)

	_, err = io.ReadFull(d.reader, buf[:1])
	if err != nil {
		return 0, perrors.WithStack(err)
	}

	return buf[0], nil
}

func listFixedTypedLenTag(tag byte) bool {
	return tag >= _listFixedTypedLenTagMin && tag <= _listFixedTypedLenTagMax
}

// Include 3 formats:
// list ::= x55 type value* 'Z'   # variable-length list
//      ::= 'V' type int value*   # fixed-length list
//      ::= [x70-77] type value*  # fixed-length typed list
func typedListTag(tag byte) bool {
	return tag == BC_LIST_FIXED || tag == BC_LIST_VARIABLE || listFixedTypedLenTag(tag)
}

func listFixedUntypedLenTag(tag byte) bool {
	return tag >= _listFixedUntypedLenTagMin && tag <= _listFixedUntypedLenTagMax
}

// Include 3 formats:
//      ::= x57 value* 'Z'        # variable-length untyped list
//      ::= x58 int value*        # fixed-length untyped list
//      ::= [x78-7f] value*       # fixed-length untyped list
func untypedListTag(tag byte) bool {
	return tag == BC_LIST_FIXED_UNTYPED || tag == BC_LIST_VARIABLE_UNTYPED || listFixedUntypedLenTag(tag)
}

//decList read list
func (d *Decoder) decList(flag int32) (interface{}, error) {
	var (
		err error
		tag byte
	)

	if flag != TAG_READ {
		tag = byte(flag)
	} else {
		tag, err = d.readByte()
		if err != nil {
			return nil, perrors.WithStack(err)
		}
	}

	switch {
	case tag == BC_NULL:
		return nil, nil
	case tag == BC_REF:
		return d.decRef(int32(tag))
	case typedListTag(tag):
		return d.readTypedList(tag)
	case untypedListTag(tag):
		return d.readUntypedList(tag)
	default:
		return nil, perrors.Errorf("error list tag: 0x%x", tag)
	}
}

// readTypedList read typed list
// Include 3 formats:
// list ::= x55 type value* 'Z'   # variable-length list
//      ::= 'V' type int value*   # fixed-length list
//      ::= [x70-77] type value*  # fixed-length typed list
func (d *Decoder) readTypedList(tag byte) (interface{}, error) {
	listTyp, err := d.decString(TAG_READ)
	if err != nil {
		return nil, perrors.Errorf("error to read list type[%s]: %v", listTyp, err)
	}

	isVariableArr := tag == BC_LIST_VARIABLE

	length := -1
	if listFixedTypedLenTag(tag) {
		length = int(tag - _listFixedTypedLenTagMin)
	} else if tag == BC_LIST_FIXED {
		ii, err := d.decInt32(TAG_READ)
		if err != nil {
			return nil, perrors.WithStack(err)
		}
		length = int(ii)
	} else if isVariableArr {
		length = 0
	} else {
		return nil, perrors.Errorf("error typed list tag: 0x%x", tag)
	}

	// return when no element
	if length < 0 {
		return nil, nil
	}

	var (
		aryValue reflect.Value
		arrType  reflect.Type
	)
	t, err := strconv.Atoi(listTyp)
	if err == nil {
		arrType = d.typeRefs[t]
	} else {
		arrType = getType(listTyp)
	}

	if arrType != nil {
		aryValue = reflect.MakeSlice(arrType, length, length)
		d.appendTypeRefs(arrType)
	} else {
		aryValue = reflect.ValueOf(make([]interface{}, length, length))
	}
	holder := d.appendRefs(aryValue)
	for j := 0; j < length || isVariableArr; j++ {
		it, err := d.DecodeValue()
		if err != nil {
			if err == io.EOF && isVariableArr {
				break
			}
			return nil, perrors.WithStack(err)
		}

		if isVariableArr {
			if it != nil {
				aryValue = reflect.Append(aryValue, EnsureRawValue(it))
			} else {
				aryValue = reflect.Append(aryValue, reflect.Zero(aryValue.Type().Elem()))
			}
			holder.change(aryValue)
		} else {
			if it != nil {
				aryValue.Index(j).Set(EnsureRawValue(it))
			} else {
				SetValue(aryValue.Index(j), EnsureRawValue(it))
			}
		}
	}

	return holder, nil
}

//readUntypedList read untyped list
// Include 3 formats:
//      ::= x57 value* 'Z'        # variable-length untyped list
//      ::= x58 int value*        # fixed-length untyped list
//      ::= [x78-7f] value*       # fixed-length untyped list
func (d *Decoder) readUntypedList(tag byte) (interface{}, error) {
	isVariableArr := tag == BC_LIST_VARIABLE_UNTYPED

	var length int
	if listFixedUntypedLenTag(tag) {
		length = int(tag - _listFixedUntypedLenTagMin)
	} else if tag == BC_LIST_FIXED_UNTYPED {
		ii, err := d.decInt32(TAG_READ)
		if err != nil {
			return nil, perrors.WithStack(err)
		}
		length = int(ii)
	} else if isVariableArr {
		length = 0
	} else {
		return nil, perrors.Errorf("error untyped list tag: %x", tag)
	}

	ary := make([]interface{}, length)
	aryValue := reflect.ValueOf(ary)
	holder := d.appendRefs(aryValue)

	for j := 0; j < length || isVariableArr; j++ {
		it, err := d.DecodeValue()
		if err != nil {
			if err == io.EOF && isVariableArr {
				break
			}
			return nil, perrors.WithStack(err)
		}

		if isVariableArr {
			if it != nil {
				aryValue = reflect.Append(aryValue, EnsureRawValue(it))
			} else {
				aryValue = reflect.Append(aryValue, reflect.Zero(aryValue.Type().Elem()))
			}
			holder.change(aryValue)
		} else {
			ary[j] = it
		}
	}

	return holder, nil
}
