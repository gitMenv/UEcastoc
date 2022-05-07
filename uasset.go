package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

// static header with the first 64 bytes of the .uasset file
type UAssetHeader struct {
	RepeatNumber             [2]uint64 // same number repeated twice
	PackageFlags             uint32    // value will be 0x80000000
	TotalHeaderSize          uint32    // deviates from this style's actual .uasset file
	NamesDirectoryOffset     uint32    // points to nullbyte, so do +1
	NamesDirectoryLength     uint32    // length is in bytes
	UnknownOffset1           uint32    // always points to value of 00 00 64 C1
	LengthOfUO1              uint32    // length includes the value of 00 00 64 C1
	UnknownOffset2           uint32
	ExportObjectsOffset      uint32 // offset to some extra header information
	UnknownOffset3           uint32 // points to some memory, just store the either 24 or 32 bytes.
	DependencyPackagesOffset uint32 // first value is a uint32, number of dependency packages.
	NoClue                   uint64
}

// total bytes: 20, probably
type DependencyPackage struct {
	Data [20]byte
}

// total bytes: 72
// quite sure this one is correct
type ExportObject struct {
	TotalHeaderSize uint64
	TotalUexpSize   uint64
	FileNameOffset  uint64 // index in the Names Directory list; name of this "file"
	OtherProperties [48]byte
}
type UAssetResource struct {
	Header             UAssetHeader
	NamesDir           []string
	ExportObjects      []ExportObject
	DependencyPackages []DependencyPackage
	Unknown1           []byte
	Unknown2           []byte
	Unknown3           []byte
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
	buff := *depPackageBuff
	var deps []DependencyPackage
	var dep DependencyPackage
	// first, one uint32 is read stating the number of dependency packages
	packageCount := binary.LittleEndian.Uint32(buff)
	buff = buff[4:]

	for i := 0; uint32(i) < packageCount; i++ {
		binary.Read(bytes.NewReader(buff), binary.LittleEndian, &dep)
		deps = append(deps, dep)
		buff = buff[binary.Size(dep):]
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
	fmt.Printf("[%08d - %08d]\n", 0, binary.Size(uasset.Header))
	namesBuffer := make([]byte, uasset.Header.NamesDirectoryLength)
	f.Seek(int64(uasset.Header.NamesDirectoryOffset+1), 0)
	n, err := f.Read(namesBuffer)
	if n != len(namesBuffer) || err != nil {
		return
	}
	uasset.NamesDir = *parseNamesDirectory(&namesBuffer)
	fmt.Printf("[%08d - %08d]\n", uasset.Header.NamesDirectoryOffset+1, uasset.Header.NamesDirectoryOffset+1+uint32(len(namesBuffer)))

	uasset.Unknown1 = make([]byte, uasset.Header.LengthOfUO1)
	f.Seek(int64(uasset.Header.UnknownOffset1), 0)
	fmt.Printf("[%08d - %08d]\n", uasset.Header.UnknownOffset1, uasset.Header.UnknownOffset1+uasset.Header.LengthOfUO1)
	_, err = f.Read(uasset.Unknown1)
	if err != nil {
		return
	}
	f.Seek(int64(uasset.Header.UnknownOffset2), 0)
	unknown2len := uasset.Header.ExportObjectsOffset - uasset.Header.UnknownOffset2
	uasset.Unknown2 = make([]byte, unknown2len)
	fmt.Printf("[%08d - %08d]\n", uasset.Header.UnknownOffset2, uasset.Header.UnknownOffset2+unknown2len)
	_, err = f.Read(uasset.Unknown2)
	if err != nil {
		return
	}

	exportLen := uasset.Header.UnknownOffset3 - uasset.Header.ExportObjectsOffset
	f.Seek(int64(uasset.Header.ExportObjectsOffset), 0)
	fmt.Printf("[%08d - %08d]\n", uasset.Header.ExportObjectsOffset, uasset.Header.ExportObjectsOffset+exportLen)
	exportObjectsBuff := make([]byte, exportLen)
	_, err = f.Read(exportObjectsBuff)
	if err != nil {
		return
	}
	uasset.ExportObjects = *parseExportObjects(&exportObjectsBuff)

	unknown3len := uasset.Header.DependencyPackagesOffset - uasset.Header.UnknownOffset3
	uasset.Unknown3 = make([]byte, unknown3len)
	f.Seek(int64(uasset.Header.UnknownOffset3), 0)
	fmt.Printf("[%08d - %08d]\n", uasset.Header.UnknownOffset3, uasset.Header.UnknownOffset3+unknown3len)
	_, err = f.Read(uasset.Unknown3)
	if err != nil {
		return
	}

	depPackageLen := uasset.Header.DependencyPackagesOffset - uasset.Header.UnknownOffset3
	depPackageBuff := make([]byte, depPackageLen)
	f.Seek(int64(uasset.Header.DependencyPackagesOffset), 0)
	_, err = f.Read(depPackageBuff)
	if err != nil {
		return
	}
	fmt.Printf("[%08d - %08d]\n", uasset.Header.DependencyPackagesOffset, uasset.Header.DependencyPackagesOffset+depPackageLen)
	uasset.DependencyPackages = *parseDependencyPackages(&depPackageBuff)

	return

}
