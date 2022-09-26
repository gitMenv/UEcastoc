package uecastoc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

/********************************************
The following structs are for parsing only
********************************************/
// StructProperty introduces levels of indentation
// everything until the next "None" belongs to this specific struct
type pStructProperty struct {
	StructLength uint64 // this value + 17 is the absolute value
	StructType   uint64
	Nullbytes    [17]byte
}
type pArrayProperty struct {
	LengthProperty uint64
	ArrayType      uint64
	Nullbyte       byte
	ArraySize      uint32
}
type pIntProperty struct {
	IntLength uint64 // always value 4
	Nullbyte  byte
	IntValue  int32
}
type pObjectProperty struct {
	IntLength uint64 // always value 4
	Nullbyte  byte
	IntValue  int32 // often negative
}
type pBoolProperty struct {
	Nullbytes [8]byte
	BoolValue uint8 // either 1 or 0
	Nullbyte  byte
}
type pFloatProperty struct {
	FloatLength uint64 // always value 4
	Nullbyte    byte
	FloatValue  float32
}
type pNameProperty struct {
	NameLength uint64 //always value 8
	Nullbyte   byte
	NameProp   uint64
}
type pEnumProperty struct {
	EnumLength   uint64 // should be 8
	EnumBaseType uint64
	Nullbyte     byte
	EnumValue    uint64
}

/*
The following structs are for the actual JSON.
Some are "Terminals", some introduce more structure
First, the structure structs are listed.
*/

// each regular property has a child that is another
// structure property or a "terminal"
type Property struct {
	Value string `json:"property"`
	Child any    `json:"child"`
}

// BaseProperty is the same as the regular Property
// except, its children are Properties
type BaseProperty struct {
	Value    string     `json:"baseproperty"`
	Children []Property `json:"properties"`
}

// keep parsing until "None"
// all properties that come in between are children of this Property
type StructProperty struct {
	StructType string `json:"structtype"`
	Children   []any  `json:"children"`
}

type ArrayProperty struct {
	ArrayType       string `json:"arraytype"`
	ArrayStructType string `json:"arraystructtype,omitempty"` // only if ArrayType == "StructProperty"
	Size            int    `json:"arraysize"`
	Children        []any
}

// the rest are "terminal" structures
type EnumProperty struct {
	BaseEnum string `json:"enumBase"`
	Value    string `json:"enumValue"`
}
type IntProperty struct {
	Value int32 `json:"int"`
}
type ObjectProperty struct {
	Value int32 `json:"object"`
}
type BoolProperty struct {
	Value bool `json:"bool"`
}
type FloatProperty struct {
	Value float32 `json:"float"`
}
type NameProperty struct {
	Value string `json:"name"`
}

// These interfaces are for finding the "None" value while parsing
type NameInterface interface{ GetName() string }

func (x Property) GetName() string       { return x.Value }
func (x StructProperty) GetName() string { return x.StructType }
func (x ArrayProperty) GetName() string  { return x.ArrayType }
func (x EnumProperty) GetName() string   { return x.Value }
func (x IntProperty) GetName() string    { return "IntProperty" }
func (x ObjectProperty) GetName() string { return "ObjectProperty" }
func (x BoolProperty) GetName() string   { return "BoolProperty" }
func (x FloatProperty) GetName() string  { return "FloatProperty" }
func (x NameProperty) GetName() string   { return x.Value }

func (u *UAssetResource) parseProperty(r *bytes.Reader) any {
	var retValue any
	var newProperty uint64
	err := binary.Read(r, binary.LittleEndian, &newProperty)

	if err != nil {
		fmt.Println()
		// fmt.Println("error:", err)
		return nil
	}
	if newProperty > uint64(len(u.NamesDir)) {
		fmt.Println("ERROR: No property found!!!!!")
		return nil
	}

	propertyName := u.NamesDir[newProperty]
	fmt.Println("New property:", propertyName)
	switch propertyName {
	case "ArrayProperty":
		fmt.Println("STARTING ARRAY PROPERTY NOW")
		var arr pArrayProperty
		err = binary.Read(r, binary.LittleEndian, &arr)
		var arp ArrayProperty
		arp.ArrayType = u.NamesDir[arr.ArrayType]
		arp.Size = int(arr.ArraySize)
		fmt.Println("debugging arrayproperty....")
		fmt.Println("Size of the array:", arp.Size)
		fmt.Println("type of the array:", arp.ArrayType)
		if arp.ArrayType == "StructProperty" {
			// read the structproperty type
			binary.Read(r, binary.LittleEndian, &newProperty)
			arp.ArrayStructType = u.NamesDir[newProperty]
			fmt.Println("the arraystructtype:", arp.ArrayStructType)
		}
		if arp.Size == 0 {
			if arp.ArrayType == "StructProperty" {
				// "read" a structproperty
				r.Seek(8, io.SeekCurrent)
				var s pStructProperty
				binary.Read(r, binary.LittleEndian, &s)
				fmt.Printf("%+v\n", s)
			}
			fmt.Println("Ending array property now")
			retValue = arp
			break
		}
		// keep continue adding children in this array until "None" is found
		for {

			// if this array is of structProperty, other measures are required...
			if arp.ArrayType == "StructProperty" {
				// add another level of children...
				arpChild := StructProperty{
					StructType: arp.ArrayStructType,
				}
				for {
					childProp := u.parseProperty(r).(NameInterface)
					if childProp.GetName() == "None" {
						break
					}

					arpChild.Children = append(arpChild.Children, childProp)
				}

				fmt.Println("adding child to the array:", arpChild.GetName())
				arp.Children = append(arp.Children, arpChild)
				fmt.Println("length:", len(arp.Children), arp.Size)
				if len(arp.Children) == arp.Size {
					break
				}
			} else {
				fmt.Println("The array without structproperty should not have been triggered (yet)")
				arpChildProp := u.parseProperty(r).(NameInterface)
				if arpChildProp.GetName() == "None" {
					break
				}
				arp.Children = append(arp.Children, arpChildProp)

			}

		}
		retValue = arp
		fmt.Println("ENDING ARRAY PROPERTY NOW")

	case "StructProperty":
		var pstr pStructProperty
		fmt.Println("STARTING STRUCT PROPERTY NOW")
		err = binary.Read(r, binary.LittleEndian, &pstr)
		sp := StructProperty{StructType: u.NamesDir[pstr.StructType]}
		if pstr.StructLength == 0 {
			// this struct doesn't have children
			// go back 17 bytes?
			retValue = sp
			break
		}
		// continue adding children until None is found
		childProp := NameInterface(sp)
		for childProp.GetName() != "None" {
			childProp = u.parseProperty(r).(NameInterface)
			if childProp.GetName() == "None" {
				break
			}
			sp.Children = append(sp.Children, childProp)
		}
		retValue = sp
		fmt.Println("ENDING STRUCT PROPERTY NOW")

	case "IntProperty":
		var pin pIntProperty
		err = binary.Read(r, binary.LittleEndian, &pin)
		retValue = IntProperty{Value: pin.IntValue}

	case "ObjectProperty":
		var ob pObjectProperty
		err = binary.Read(r, binary.LittleEndian, &ob)
		retValue = ObjectProperty{Value: ob.IntValue}

	case "BoolProperty":
		var boo pBoolProperty
		err = binary.Read(r, binary.LittleEndian, &boo)
		retValue = BoolProperty{Value: boo.BoolValue == 1}

	case "FloatProperty":
		var fl pFloatProperty
		err = binary.Read(r, binary.LittleEndian, &fl)
		retValue = FloatProperty{Value: fl.FloatValue}
	case "NameProperty":
		var nm pNameProperty
		err = binary.Read(r, binary.LittleEndian, &nm)
		retValue = NameProperty{Value: u.NamesDir[nm.NameProp]}
	case "EnumProperty":
		var en pEnumProperty
		err = binary.Read(r, binary.LittleEndian, &en)
		fmt.Printf("%+v\n", en)

		retValue = EnumProperty{
			BaseEnum: u.NamesDir[en.EnumBaseType],
			Value:    u.NamesDir[en.EnumValue],
		}
	case "None":
		var pr Property
		pr.Value = propertyName
		retValue = pr
	default:
		var pr Property
		pr.Value = propertyName
		pr.Child = u.parseProperty(r)
		retValue = pr
	}
	if err != nil {
		// fmt.Println("error:", err)
		fmt.Println()
	}

	return retValue
}

// this function skips over the first part, up until the first None value
// this makes it so I can start reading from the list of actual data, which is nice.
func (u *UAssetResource) skipOver(r *bytes.Reader) {
	var b byte = 0
	var num uint64
	noneNumber := uint64(0xff)
	for i, v := range u.NamesDir {
		if v == "None" {
			noneNumber = uint64(i)
		}
	}
	bytesNone := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytesNone, noneNumber)
	matches := 0
	for b != bytesNone[matches] {
		b, _ = r.ReadByte()
	}
	r.UnreadByte()
	binary.Read(r, binary.LittleEndian, &num)
	if num == noneNumber {
		// found the number, now skip 4 bytes and return
		fourbytes := make([]byte, 4)
		binary.Read(r, binary.LittleEndian, &fourbytes)
		for _, v := range fourbytes {
			if v != 0 {
				fmt.Println("ERROR")
			}
		}
	} else {
		// not found yet, try again
		u.skipOver(r)
	}
}

// I'm gonna build a tree!
func (u *UAssetResource) ParseUexp(data *[]byte) []any {
	var ret []any
	var newProperty uint64
	var numberOfEntries uint32
	r := bytes.NewReader(*data)
	u.skipOver(r)
	binary.Read(r, binary.LittleEndian, &numberOfEntries)
	// the number stored in numberOfEntries should be reached in subsequent steps.
	fmt.Println("Number of things:", numberOfEntries)

	for r.Len() != 0 {

		// here we only deal with baseproperties
		var bprop BaseProperty
		err := binary.Read(r, binary.LittleEndian, &newProperty)
		if err != nil {
			fmt.Println("err:", err)
		}
		if newProperty > uint64(len(u.NamesDir)) {
			fmt.Println("error: there should be an actual property here!")
			ret = append(ret, nil)
			return ret
		}
		bprop.Value = u.NamesDir[newProperty]
		if bprop.Value == "None" {
			fmt.Println("concluded this child")
			ret = append(ret, bprop)
			continue
		}

		// here deal with baseproperties' children
		for {
			var prop Property
			binary.Read(r, binary.LittleEndian, &newProperty)

			if newProperty < uint64(len(u.NamesDir)) {
				fmt.Println("Value of new property:", u.NamesDir[newProperty])
			} else {
				fmt.Printf("value not a string: %d, 0x%x\n", newProperty, newProperty)
			}

			prop.Value = u.NamesDir[newProperty]

			if prop.Value == "None" {
				fmt.Println("Adding another child for this thing")
				break
			}
			prop.Child = u.parseProperty(r)

			bprop.Children = append(bprop.Children, prop)
		}
		fmt.Println("Added this to the base properties.")
		fmt.Println("moving on!")
		ret = append(ret, bprop)
	}

	return ret
}
