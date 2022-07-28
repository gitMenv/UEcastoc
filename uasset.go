package uecastoc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// The ucas file consists of .uasset files with a structure that differs from the original .uasset structure.
// it is similar, but not exactly the same.

// static header with the first 64 bytes of the .uasset file
type UAssetHeader struct {
	RepeatNumber             [2]uint64 // same number repeated twice
	PackageFlags             uint32    // value will be 0x80000000
	TotalHeaderSize          uint32    // of the original file header size; but that included the dependencies...
	NamesDirectoryOffset     uint32    // points to nullbyte, so do +1
	NamesDirectoryLength     uint32    // length is in bytes
	UnknownOffset1           uint32    // always points to value of 00 00 64 C1
	LengthOfUO1              uint32    // length includes the value of 00 00 64 C1
	UnknownOffset2           uint32
	ExportObjectsOffset      uint32 // offset to some extra header information
	UnknownOffset3           uint32 // points to some memory, just store the either 24 or 32 bytes.
	DependencyPackagesOffset uint32 // first value is a uint32, number of dependency packages.
	DependencyPackagesSize   uint64
}

// total bytes are variable
type DependencyPackage struct {
	ID              uint64
	NumberOfEntries uint32 // not sure what this does exactly
	IsPresent       int32
	SomeValue       uint32 // mostly one, but sometimes 2
	// depending on the NumberOfEntries, there are multiple entries here
	ExtraEntries []uint32
}

// total bytes: 72
type ExportObject struct {
	SerialOffset     uint64
	SerialSize       uint64
	ObjectNameOffset uint64 // index in the Names Directory list; name of this "file"
	ClassNameOffset  uint64
	OtherProperties  [40]byte
}
type UAssetResource struct {
	Header             UAssetHeader
	NamesDir           []string
	ExportObjects      []ExportObject
	DependencyPackages []DependencyPackage
	// not sure what I have to do with these unknowns yet, just store for now
	Unknown1 []byte
	Unknown2 []byte
	Unknown3 []byte
}

func (u *UAssetResource) PrintNamesDirectory() {
	for i, v := range u.NamesDir {
		fmt.Printf("[%02d][0x%02x]: %s\n", i, i, v)
	}
}
func parseNamesDirectory(namesBuffer *[]byte) *[]string {
	var strlist []string
	buff := (*namesBuffer)
	for len(buff) != 0 {
		strlen := uint8(buff[0])
		name := string(buff[1 : 1+strlen])
		strlist = append(strlist, name)
		buff = buff[strlen+2:]
	}
	return &strlist
}
func parseExportObjects(exportObjectBuff *[]byte) *[]ExportObject {
	buff := *exportObjectBuff
	var exports []ExportObject
	var export ExportObject
	// first, find the number of entries
	numberOfEntries := len(buff) / binary.Size(export)

	for i := 0; i < int(numberOfEntries); i++ {
		binary.Read(bytes.NewReader(buff), binary.LittleEndian, &export)
		exports = append(exports, export)
		buff = buff[binary.Size(export):]
	}
	return &exports
}

func parseDependencyPackages(depPackageBuff *[]byte) *[]DependencyPackage {
	var packageCount uint32
	var extraEntry uint32
	var deps []DependencyPackage
	var dep DependencyPackage

	r := bytes.NewReader(*depPackageBuff)
	// first, one uint32 is read stating the number of dependency packages
	binary.Read(r, binary.LittleEndian, &packageCount)
	for i := 0; uint32(i) < packageCount; i++ {
		dep.ExtraEntries = []uint32{}

		binary.Read(r, binary.LittleEndian, &dep.ID)
		binary.Read(r, binary.LittleEndian, &dep.NumberOfEntries)
		binary.Read(r, binary.LittleEndian, &dep.IsPresent)
		binary.Read(r, binary.LittleEndian, &dep.SomeValue)

		if dep.NumberOfEntries > 1 {
			for extra := 0; uint32(extra) < dep.NumberOfEntries-1; extra++ {
				// two entries per extra thing
				for j := 0; j < 2; j++ {
					binary.Read(r, binary.LittleEndian, &extraEntry)
					dep.ExtraEntries = append(dep.ExtraEntries, extraEntry)
				}
			}
		}
		deps = append(deps, dep)
	}
	return &deps
}

// ParseUAssetFile takes a string and it expects the full .uasset file, including .uexp
// If the .uexp isn't there, there might be problems.
func ParseUAssetFile(path string) (uasset UAssetResource, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}

	err = binary.Read(f, binary.LittleEndian, &uasset.Header)
	if err != nil {
		return
	}

	namesBuffer := make([]byte, uasset.Header.NamesDirectoryLength)
	f.Seek(int64(uasset.Header.NamesDirectoryOffset+1), io.SeekStart)
	n, err := f.Read(namesBuffer)
	if n != len(namesBuffer) || err != nil {
		return
	}
	uasset.NamesDir = *parseNamesDirectory(&namesBuffer)

	uasset.Unknown1 = make([]byte, uasset.Header.LengthOfUO1)
	f.Seek(int64(uasset.Header.UnknownOffset1), io.SeekStart)

	_, err = f.Read(uasset.Unknown1)
	if err != nil {
		return
	}
	f.Seek(int64(uasset.Header.UnknownOffset2), 0)
	unknown2len := uasset.Header.ExportObjectsOffset - uasset.Header.UnknownOffset2
	uasset.Unknown2 = make([]byte, unknown2len)

	_, err = f.Read(uasset.Unknown2)
	if err != nil {
		return
	}

	exportLen := uasset.Header.UnknownOffset3 - uasset.Header.ExportObjectsOffset
	f.Seek(int64(uasset.Header.ExportObjectsOffset), io.SeekStart)

	exportObjectsBuff := make([]byte, exportLen)
	_, err = f.Read(exportObjectsBuff)
	if err != nil {
		return
	}
	uasset.ExportObjects = *parseExportObjects(&exportObjectsBuff)

	unknown3len := uasset.Header.DependencyPackagesOffset - uasset.Header.UnknownOffset3
	uasset.Unknown3 = make([]byte, unknown3len)
	f.Seek(int64(uasset.Header.UnknownOffset3), io.SeekStart)

	_, err = f.Read(uasset.Unknown3)
	if err != nil {
		return
	}

	depPackageBuff := make([]byte, uasset.Header.DependencyPackagesSize)
	f.Seek(int64(uasset.Header.DependencyPackagesOffset), io.SeekStart)
	_, err = f.Read(depPackageBuff)
	if err != nil {
		return
	}

	uasset.DependencyPackages = *parseDependencyPackages(&depPackageBuff)

	// the actual data starts after this area
	uexpdataOffset := uint64(uasset.Header.DependencyPackagesOffset) + uasset.Header.DependencyPackagesSize

	f.Seek(int64(uexpdataOffset), io.SeekStart)
	uasset.PrintNamesDirectory()
	return

}
