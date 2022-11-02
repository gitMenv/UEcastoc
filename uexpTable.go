package uecastoc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// implements UexpStructure
type UexpDataTable struct {
	Name           string         `json:"name"`
	Header         []Property     `json:"datatableheader"`
	BaseProperties []BaseProperty `json:"datatable"`
}

func (u UexpDataTable) GetType() EOType { return EOTypeTable }

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
	NameProp   uint32
	Secondary  uint32
}
type pEnumProperty struct {
	EnumLength   uint64 // should be 8
	EnumBaseType uint64
	Nullbyte     byte
	EnumValue    uint64
}
type pByteProperty struct {
	ByteValue uint64 // not sure
	NoneValue uint64
	Nullbytes [2]byte
}

// not sure if this should be static or dynamic
type pSoftObjectProperty struct {
	Length         uint64 // seems to be always 0xC (12)
	Nullbyte       byte
	Value          uint32 // maps to string table
	SecondaryValue int32
	Nullbytes      [4]byte // together with Value, this makes 12 (Length) bytes
}

/*
The following structs are for the actual JSON.
Some are "Terminals", some introduce more structure
First, the structure structs are listed.
*/

// each regular property has a child that is another
// structure property or a "terminal"
type Property struct {
	Value     string `json:"property"`
	Secondary int32  `json:"secondary,omitempty"`
	Child     any    `json:"child,omitempty"`
}

// BaseProperty is the same as the regular Property
// except, its children are Properties
type BaseProperty struct {
	Value     string     `json:"baseproperty"`
	Secondary int32      `json:"secondary,omitempty"`
	Children  []Property `json:"properties"`
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
type Int64Property struct {
	Value int64 `json:"int64"`
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
	Value          string `json:"name"`
	SecondaryValue int32  `json:"secondary,omitempty"`
}
type ByteProperty struct {
	Value byte `json:"byte"`
}
type SoftObjectProperty struct {
	Value     string `json:"softproperty"`
	Secondary int32  `json:"secondary,omitempty"`
}

// These interfaces are for finding the "None" value while parsing
type NameInterface interface{ GetName() string }

func (x Property) GetName() string       { return x.Value }
func (x StructProperty) GetName() string { return x.StructType }
func (x ArrayProperty) GetName() string  { return x.ArrayType }
func (x EnumProperty) GetName() string   { return x.Value }
func (x IntProperty) GetName() string    { return "IntProperty" }
func (x Int64Property) GetName() string  { return "Int64Property" }
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
		curr, _ := r.Seek(0, io.SeekCurrent)
		fmt.Println("ERROR: No property found!!!!! offset:", curr)
		fmt.Printf("value:%x \n", newProperty)
		r.Seek(-0x10, io.SeekCurrent)
		newbytes := [0x20]byte{}
		binary.Read(r, binary.LittleEndian, &newbytes)
		for _, v := range newbytes {
			fmt.Printf("%02x ", v)
		}
		fmt.Println()
		return nil
	}

	propertyName := u.NamesDir[newProperty]

	switch propertyName {
	case "ArrayProperty":
		// ArrayProperty has a certain number of values.
		// If its type is a StructProperty, that StructProperty also has its type.
		// Each array element will have that type and must be ended with "None" property.
		// The array itself is not ended with "None" and just stops after the N elements.
		var arr pArrayProperty
		err = binary.Read(r, binary.LittleEndian, &arr)
		var arp ArrayProperty
		arp.ArrayType = u.NamesDir[arr.ArrayType]
		arp.Size = int(arr.ArraySize)

		bStructProperty := arp.ArrayType == "StructProperty"
		bObjectProperty := arp.ArrayType == "ObjectProperty"
		var pstrp pStructProperty
		var strp StructProperty

		if bStructProperty {
			binary.Read(r, binary.LittleEndian, &newProperty)
			arp.ArrayStructType = u.NamesDir[newProperty]

			// *read* one struct property. This seems to be the overarching name
			binary.Read(r, binary.LittleEndian, &newProperty)
			if u.NamesDir[newProperty] != "StructProperty" {
				fmt.Println("error: expected a StructProperty here, got:", u.NamesDir[newProperty])
				panic("whoopsie")
			}
			binary.Read(r, binary.LittleEndian, &pstrp)
			strp.StructType = u.NamesDir[pstrp.StructType]
		} else if bObjectProperty {

		} else {
			fmt.Println("property::", arp.ArrayType)
			curr, _ := r.Seek(0, io.SeekCurrent)
			fmt.Println("array at offset:", curr)
			fmt.Println("Not sure what to do with this case yet.")
			panic("to be continued...")
		}
		for len(arp.Children) != arp.Size {
			if bStructProperty {
				var arpChild StructProperty
				arpChild.StructType = strp.StructType // copy base class value
				// continue parsing children of this struct until "None"
				for {
					structChild := u.parseProperty(r).(NameInterface)
					if structChild.GetName() == "None" {
						break
					}
					arpChild.Children = append(arpChild.Children, structChild)

				}
				arp.Children = append(arp.Children, arpChild)
			} else if bObjectProperty {
				var arpChild ObjectProperty
				binary.Read(r, binary.LittleEndian, &arpChild.Value)
				arp.Children = append(arp.Children, arpChild)
			} else {

				arp.Children = append(arp.Children, u.parseProperty(r))
			}
		}
		retValue = arp

	case "StructProperty":
		var pstr pStructProperty
		err = binary.Read(r, binary.LittleEndian, &pstr)
		sp := StructProperty{StructType: u.NamesDir[pstr.StructType]}
		if pstr.StructLength == 0 {
			// this struct doesn't have children
			retValue = sp
			break
		}
		// this is apparently an exception
		if sp.StructType == "GameplayTagContainer" {
			for i := uint64(0); i < pstr.StructLength/4; i++ {
				var newChild IntProperty
				binary.Read(r, binary.LittleEndian, &newChild.Value)
				sp.Children = append(sp.Children, newChild)
			}
			retValue = sp
			break
		} else if sp.StructType == "Guid" {
			for i := uint64(0); i < pstr.StructLength/8; i++ {
				var newChild Int64Property
				binary.Read(r, binary.LittleEndian, &newChild.Value)
				sp.Children = append(sp.Children, newChild)
			}
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
		retValue = NameProperty{Value: u.NamesDir[nm.NameProp], SecondaryValue: int32(nm.Secondary)}
	case "EnumProperty":
		var en pEnumProperty
		err = binary.Read(r, binary.LittleEndian, &en)

		retValue = EnumProperty{
			BaseEnum: u.NamesDir[en.EnumBaseType],
			Value:    u.NamesDir[en.EnumValue],
		}
	case "SoftObjectProperty":
		var sop pSoftObjectProperty
		err = binary.Read(r, binary.LittleEndian, &sop)
		retValue = SoftObjectProperty{Value: u.NamesDir[sop.Value], Secondary: sop.SecondaryValue}
	case "ByteProperty":
		var bp pByteProperty
		err = binary.Read(r, binary.LittleEndian, &bp)
		retValue = ByteProperty{Value: byte(bp.ByteValue)}
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

func (u *UAssetResource) parseTableHeader(r *bytes.Reader) []Property {
	var headerProps []Property

	// Add each child to the base property
	for {
		child := u.parseProperty(r)
		if child.(NameInterface).GetName() == "None" {
			// none, continue with the next base property
			break
		}
		headerProps = append(headerProps, child.(Property))
	}

	// skip 4 bytes that is necessary...
	r.Seek(4, io.SeekCurrent)
	return headerProps
}

func (u *UAssetResource) parseTable(data *[]byte) UexpDataTable {
	var ret UexpDataTable
	var newProperty uint64
	var numberOfEntries uint32
	r := bytes.NewReader(*data)

	// I really should fix this skipover thingy
	ret.Header = u.parseTableHeader(r)

	binary.Read(r, binary.LittleEndian, &numberOfEntries)
	// Each entry is a "BaseProperty".
	// this base property has a name and children properties.
	// These children properties also have additional properties, etc.
	// The "None" property marks the end of the current base property
	for uint32(len(ret.BaseProperties)) != numberOfEntries {
		var bp BaseProperty
		binary.Read(r, binary.LittleEndian, &newProperty)
		bp.Secondary = int32(newProperty >> 32)
		if uint32(newProperty) >= uint32(len(u.NamesDir)) {
			fmt.Println("an error has occurred; a property value was higher than the number of names")
			return ret
		}
		bp.Value = u.NamesDir[uint32(newProperty)]

		// Add each child to the base property
		for {
			child := u.parseProperty(r).(NameInterface)
			if child.GetName() == "None" {
				// none, continue with the next base property
				break
			}
			bp.Children = append(bp.Children, child.(Property))
		}
		// fmt.Println(bp.Value)
		ret.BaseProperties = append(ret.BaseProperties, bp)
	}

	return ret
}
