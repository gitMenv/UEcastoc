package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/fatih/color"
)

const (
	MagicUtoc       string = "-==--==--==--==-"
	VersionUtoc     uint32 = 2
	UnrealSignature string = "\xC1\x83\x2A\x9E"
	MountPoint      string = "../../../"
	NoneEntry       uint32 = 0xffffffff
)

var (
	ErrWrongMagic         = errors.New("error: magic number was not found")
	ErrUnknownUtocVersion = errors.New("error: utoc version is unknown")
	ErrContainerFlag      = errors.New("WARNING: the container flags were not 0 nor the indexed flag, so it could be a special case")
	ErrExpectedBytesRead  = errors.New("expected to have read all bytes...")
	ErrTocEntryBounds     = errors.New("TocEntry out of container bounds")
)

type EIoContainerFlags uint8

const (
	NoneContainerFlag       EIoContainerFlags = iota
	CompressedContainerFlag                   = iota
	EncryptedContainerFlag                    = (1 << iota)
	SignedContainerFlag                       = (1 << iota)
	IndexedContainerFlag                      = (1 << iota)
)

// four components that make up a unique identifier.
// Seems a bit overkill, but: https://github.com/EpicGames/UnrealEngine/blob/release/Engine/Source/Runtime/Core/Public/Misc/Guid.h
type FGuid struct {
	A, B, C, D uint32
}

// Bible: https://github.com/jashking/UnrealPakViewer/blob/master/PakAnalyzer/Private/IoStoreDefines.h
// Jackpot: https://github.com/EpicGames/UnrealEngine/blob/99b6e203a15d04fc7bbbf554c421a985c1ccb8f1/Engine/Source/Runtime/Core/Private/IO/IoStore.h
type UTocHeader struct {
	Magic                       [16]byte
	Version                     uint32 // they state it's uint8, but I doubt it
	HeaderSize                  uint32
	EntryCount                  uint32
	CompressedBlockEntryCount   uint32
	CompressedBlockEntrySize    uint32 // they say "For sanity checking", going insane...
	CompressionMethodNameCount  uint32
	CompressionMethodNameLength uint32
	CompressionBlockSize        uint32
	DirectoryIndexSize          uint32
	PartitionCount              uint32 // should be 0
	ContainerID                 FIoContainerID
	EncryptionKeyGuid           FGuid
	ContainerFlags              EIoContainerFlags
	Padding                     [63]byte
}

type FIoStoreTocResourceInfo struct {
	Header               UTocHeader
	TocFileSize          uint64
	ChunkIDs             []FIoChunkID
	ChunkIDToIndex       map[FIoChunkID]uint32
	ChunkIDToOfflengths  map[FIoChunkID]FIoOffsetAndLength
	CompressionBlocks    []FIoStoreTocCompressedBlockEntry
	ChunkMetas           []FIoStoreTocEntryMeta
	ChunkBlockSignatures []FSHAHash
	CompressionMethods   []FName
	DirectoryResource    FIoDirectoryIndexResource
	// DirectoryIndexBuffer []byte
	Offlengths   []FIoOffsetAndLength
	filepath     string
	openUCASFile *os.File
}

type FIoDirectoryIndexResource struct {
	MountPoint       FString
	DirectoryEntries []FIoDirectoryIndexEntry
	FileEntries      []FIoFileIndexEntry
	StringTable      []FString
}

func (d *FIoDirectoryIndexResource) printFile(index int, level int) {

	file := d.FileEntries[index]

	for i := 0; i < level; i++ {
		fmt.Printf("\t")
	}

	color.Set(color.FgGreen)
	if file.Name != NoneEntry {
		fmt.Printf("F:")
		nullbyte := 0
		if d.StringTable[file.Name][len(d.StringTable[file.Name])-1] == 0x0 {
			nullbyte = 1
		}
		fmt.Println(d.StringTable[file.Name][:len(d.StringTable[file.Name])-nullbyte])
	}
	color.Unset()
	if file.NextFileEntry != NoneEntry {
		d.printFile(int(file.NextFileEntry), level)
	}
}

//printDirectory starts at the root, and then prints every single subfolder in its own level.
// just for debugging purposes
func (d *FIoDirectoryIndexResource) printDirectory(index int, level int) {
	dir := d.DirectoryEntries[index]
	for i := 0; i < level; i++ {
		fmt.Printf("\t")
	}
	// find all siblings of this directory
	if dir.Name != NoneEntry {
		nullbyte := 0
		if d.StringTable[dir.Name][len(d.StringTable[dir.Name])-1] == 0x0 {
			nullbyte = 1
		}
		fmt.Println(d.StringTable[dir.Name][:len(d.StringTable[dir.Name])-nullbyte])
	}
	// depth
	if dir.FirstChildEntry != NoneEntry {
		d.printDirectory(int(dir.FirstChildEntry), level+1)
	}
	if dir.FirstFileEntry != NoneEntry {
		d.printFile(int(dir.FirstFileEntry), level+1)
	}
	if dir.NextSiblingEntry != NoneEntry {
		d.printDirectory(int(dir.NextSiblingEntry), level)
	}

}

func (h *FIoStoreTocResourceInfo) decompressBlock(block *FIoStoreTocCompressedBlockEntry) *[]byte {
	var requestedBytes []byte
	// open the .ucas file first
	if h.openUCASFile == nil {
		f, err := os.Open(h.filepath + ".ucas")
		if err != nil {
			fmt.Println("problem with file reading:", err)
			return &requestedBytes
		}
		h.openUCASFile = f
	}
	ucasFile := h.openUCASFile

	_, err := ucasFile.Seek(int64(block.GetOffset()), 0)
	if err != nil {
		fmt.Println("err:", err)
		return &requestedBytes
	}
	buf := make([]byte, block.GetCompressedSize())
	readBytes, err := ucasFile.Read(buf)
	if err != nil {
		fmt.Println("err:", err)
		return &requestedBytes
	}
	if uint32(readBytes) != block.GetCompressedSize() {
		fmt.Println("couldn't read enough bytes...")
	}
	// now decompress
	switch block.CompressionMethod {
	case 0:
		requestedBytes = append(requestedBytes, buf...)
	case 1:
		// decompress with zlib
		r, err := zlib.NewReader(bytes.NewBuffer(buf))
		defer r.Close()
		if err != nil {
			fmt.Println(err)
			return &requestedBytes
		}
		uncompressed, err := ioutil.ReadAll(r)
		if err != nil {
			fmt.Println(err)
			return &requestedBytes
		}
		if len(uncompressed) != int(block.GetUncompressedSize()) {
			fmt.Println("didn't decompress correctly!")
			return &requestedBytes
		}
		requestedBytes = append(requestedBytes, uncompressed...)
	default:
		fmt.Println("unknown compression method!!!")
		return &requestedBytes
	}

	return &requestedBytes

}

func fetchSignature(filepath string) *[]byte {
	var ret []byte
	if filepath[len(filepath)-7:] == ".uasset" {
		ret = []byte(UnrealSignature)
	}
	return &ret
}

func parseDirectoryBufferThing(directoryIndexBuffer *[]byte, size uint32) *FIoDirectoryIndexResource {
	buff := *directoryIndexBuffer
	var dir FIoDirectoryIndexResource

	// first parse the mountpoint
	mountStrLength := binary.LittleEndian.Uint32(buff)
	buff = buff[4:] // consume integer
	dir.MountPoint = FString(string(buff[:mountStrLength]))
	buff = buff[mountStrLength:]
	// now parse this beast
	dirEntriesCount := binary.LittleEndian.Uint32(buff)
	buff = buff[4:]
	var dirEntry FIoDirectoryIndexEntry
	for i := 0; i < int(dirEntriesCount); i++ {
		binary.Read(bytes.NewReader(buff), binary.LittleEndian, &dirEntry)
		buff = buff[binary.Size(dirEntry):]
		dir.DirectoryEntries = append(dir.DirectoryEntries, dirEntry)
	}
	fileIndexCount := binary.LittleEndian.Uint32(buff)
	buff = buff[4:]

	var fileEntry FIoFileIndexEntry
	for i := 0; uint32(i) < fileIndexCount; i++ {
		binary.Read(bytes.NewReader(buff), binary.LittleEndian, &fileEntry)
		buff = buff[binary.Size(fileEntry):]
		dir.FileEntries = append(dir.FileEntries, fileEntry)
	}
	numberOfStrings := binary.LittleEndian.Uint32(buff)
	buff = buff[4:]

	for i := 0; uint32(i) < numberOfStrings; i++ {
		// first read stringsize, then read string
		stringSize := binary.LittleEndian.Uint32(buff)
		buff = buff[4:]
		dir.StringTable = append(dir.StringTable, FString(string(buff[:stringSize])))
		buff = buff[stringSize:]
	}
	// dir.printDirectory(0, 0)
	// fmt.Println()

	return &dir
}

func ParseUTocHeader(filepath string) (tr FIoStoreTocResourceInfo, err error) {
	// remove possible extensions from the file
	filepath = strings.TrimSuffix(filepath, ".utoc")
	filepath = strings.TrimSuffix(filepath, ".ucas")
	tr.filepath = filepath

	b, err := ioutil.ReadFile(tr.filepath + ".utoc")
	if err != nil {
		return
	}
	var hdr UTocHeader
	err = binary.Read(bytes.NewReader(b), binary.LittleEndian, &hdr)
	if err != nil {
		return
	}
	if string(hdr.Magic[:]) != MagicUtoc {
		err = ErrWrongMagic
		return
	}
	if hdr.Version != VersionUtoc {
		err = ErrUnknownUtocVersion
		return
	}
	// this "sanity check"
	if hdr.CompressedBlockEntrySize != 12 { // must be sizeof FIoStoreTocCompressedBlockEntry
		err = ErrUnknownUtocVersion // didn't want to create a new error for just this...
		return
	}

	// header properly read; now aggregate into a TocResourceInfo
	tr.ChunkIDToIndex = make(map[FIoChunkID]uint32)
	tr.TocFileSize = uint64(len(b) - binary.Size(tr.Header))
	tr.Header = hdr

	// read chunk IDs
	b = b[144:] // drop header from byte stream
	var chunkID FIoChunkID
	for i := 0; i < int(tr.Header.EntryCount); i++ {
		binary.Read(bytes.NewReader(b), binary.LittleEndian, &chunkID)
		tr.ChunkIDs = append(tr.ChunkIDs, chunkID)
		b = b[binary.Size(chunkID):]
	}
	for i, id := range tr.ChunkIDs {
		tr.ChunkIDToIndex[id] = uint32(i)
	}

	// chunk offsets
	var offlength FIoOffsetAndLength
	for i := 0; i < int(tr.Header.EntryCount); i++ {
		binary.Read(bytes.NewReader(b), binary.LittleEndian, &offlength)
		tr.Offlengths = append(tr.Offlengths, offlength)
		b = b[binary.Size(offlength):]
	}
	// Compression blocks
	var compressionBlock FIoStoreTocCompressedBlockEntry
	for i := 0; i < int(tr.Header.CompressedBlockEntryCount); i++ {
		binary.Read(bytes.NewReader(b), binary.LittleEndian, &compressionBlock)
		tr.CompressionBlocks = append(tr.CompressionBlocks, compressionBlock)
		b = b[binary.Size(compressionBlock):]
	}
	// compression methods
	var compressionMethods []FName
	compressionMethods = append(compressionMethods, FName("None"))
	for i := 0; i < int(tr.Header.CompressionMethodNameCount); i++ {
		compressionMethods = append(compressionMethods, FName(b[:tr.Header.CompressionMethodNameLength]))
		b = b[tr.Header.CompressionMethodNameLength:]
	}

	// chunk block signatures;
	indexedFlag := uint8(1 << 3)
	// compressedFlag := uint8(1 << 0)
	if hdr.ContainerFlags != 0 && uint8(hdr.ContainerFlags)&indexedFlag == 0 {
		// the reference project may contain flags here, but no idea what it should do...
		err = ErrContainerFlag
		return
	}

	directoryIndexBuffer := (b[:hdr.DirectoryIndexSize])
	tr.DirectoryResource = *parseDirectoryBufferThing(&directoryIndexBuffer, tr.Header.DirectoryIndexSize)

	b = b[hdr.DirectoryIndexSize:]

	var meta FIoStoreTocEntryMeta
	for i := 0; i < int(tr.Header.EntryCount); i++ {
		binary.Read(bytes.NewReader(b), binary.LittleEndian, &meta)
		tr.ChunkMetas = append(tr.ChunkMetas, meta)
		b = b[binary.Size(meta):]
	}

	if len(b) != 0 {
		err = ErrExpectedBytesRead
	}

	// https://github.com/luvyana/val-scripts/blob/6eda8d745f2dded067e91e345e48cdb6b09a5e90/utils/UE4Parse/IO/IoStoreReader.py
	// map chunk IDs to offlengths for easier access
	tr.ChunkIDToOfflengths = make(map[FIoChunkID]FIoOffsetAndLength)

	conUncompressedSize := uint64(tr.Header.CompressedBlockEntryCount) * uint64(tr.Header.CompressionBlockSize)
	for i := 0; i < int(tr.Header.EntryCount); i++ {
		if tr.Offlengths[i].GetOffset()+tr.Offlengths[i].GetLength() > conUncompressedSize {
			err = ErrTocEntryBounds
			break
		}
		tr.ChunkIDToOfflengths[tr.ChunkIDs[i]] = tr.Offlengths[i]
	}
	return
}

// yes, these functions are repetitive.
// However, it provides a more clear insight in the file format in the
// WriteUTOC function.
// Some of them are a bit more complex
func (h *FIoStoreTocResourceInfo) writeChunkIDs(utoc *os.File) {
	for _, chunkID := range h.ChunkIDs {
		writeStructToFile(chunkID, utoc)
	}
}
func (h *FIoStoreTocResourceInfo) writeOfflengths(utoc *os.File) {
	for _, offlength := range h.Offlengths {
		writeStructToFile(offlength, utoc)
	}
}
func (h *FIoStoreTocResourceInfo) writeCompressionBlocks(utoc *os.File) {
	for _, compressionBlock := range h.CompressionBlocks {
		writeStructToFile(compressionBlock, utoc)
	}
}
func (h *FIoStoreTocResourceInfo) writeCompressionMethods(utoc *os.File) {
	// must be only one, which is zlib
	// if h.Header.CompressionMethodCount is NOT 1, make sure to extend this to loop!
	buff := make([]byte, h.Header.CompressionMethodNameLength)
	zlib := []byte("Zlib")
	copy(buff, zlib)
	utoc.Write(buff)
}

func (h *FIoStoreTocResourceInfo) writeDirectoryIndex(utoc *os.File) {
	dir := h.DirectoryResource
	dirCount := uint32(len(dir.DirectoryEntries))
	fileCount := uint32(len(dir.FileEntries))
	strCount := uint32(len(dir.StringTable))
	// mount point string
	mountPointStr := stringToFString(string(dir.MountPoint))
	utoc.Write(mountPointStr)

	// directory entries

	writeUint32ToFile(&dirCount, utoc)
	for _, directoryEntry := range dir.DirectoryEntries {
		writeStructToFile(directoryEntry, utoc)
	}
	writeUint32ToFile(&fileCount, utoc)
	for _, fileEntry := range dir.FileEntries {
		writeStructToFile(fileEntry, utoc)
	}
	writeUint32ToFile(&strCount, utoc)
	for _, str := range dir.StringTable {
		utoc.Write(stringToFString(string(str)))
	}
}

func (h *FIoStoreTocResourceInfo) writeChunkMetas(utoc *os.File) {
	for _, meta := range h.ChunkMetas {
		writeStructToFile(meta, utoc)
	}
}

func (h *FIoStoreTocResourceInfo) WriteTOC() {
	// create clear utoc file
	tocPath := h.filepath + "/../" + "packed_P.utoc"
	os.Remove(tocPath)

	f, err := os.OpenFile(tocPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer f.Close()

	// write first static 144 bytes:
	err = binary.Write(f, binary.LittleEndian, h.Header)
	if err != nil {
		fmt.Println("ERROR:", err)
	}

	// write the rest of the file
	h.writeChunkIDs(f)
	h.writeOfflengths(f)
	h.writeCompressionBlocks(f)
	h.writeCompressionMethods(f)
	h.writeDirectoryIndex(f)
	h.writeChunkMetas(f)
	// file written in full
}
