package main

import (
	"encoding/binary"
	"fmt"
	"strconv"
)

type FIoContainerID uint64
type FIoStoreTocEntryMetaFlags uint8
type FString string // just a string I guess? But fancier...
type FName string

const (
	NoneMetaFlag FIoStoreTocEntryMetaFlags = iota
	CompressedMetaFlag
	MemoryMappedMetaFlag
)

type FIoDirectoryIndexEntry struct {
	Name             uint32
	FirstChildEntry  uint32
	NextSiblingEntry uint32
	FirstFileEntry   uint32
}

type FIoFileIndexEntry struct {
	Name          uint32
	NextFileEntry uint32
	UserData      uint32
}

type FIoChunkID struct {
	ID      uint64 // 8 bytes
	Index   uint16 // 2 bytes
	Padding uint8  // 1 byte
	Type    uint8  // 1 byte
}

type DirIndexWrapper struct {
	dirs     *[]*FIoDirectoryIndexEntry
	files    *[]*FIoFileIndexEntry
	strTable *map[string]int
	strSlice *[]string
}

func (r *FIoDirectoryIndexEntry) AddFile(fpathSections []string, fIndex uint32, structure *DirIndexWrapper) {
	if len(fpathSections) == 0 {
		return
	}
	if len(fpathSections) == 1 {
		// only one item, add file and return; base case
		fname := fpathSections[0]
		newFile := FIoFileIndexEntry{
			Name:          uint32((*(*structure).strTable)[fname]),
			NextFileEntry: NoneEntry,
			UserData:      fIndex,
		}
		newEntryIndex := uint32(len(*(*structure).files))
		*(*structure).files = append(*(*structure).files, &newFile)

		if (*r).FirstFileEntry == NoneEntry {
			(*r).FirstFileEntry = newEntryIndex
		} else {
			fentry := &(*(*structure).files)[(*r).FirstFileEntry]
			// filenames (with their path) are unique and will always be added
			for (*fentry).NextFileEntry != NoneEntry {
				fentry = &(*(*structure).files)[(*fentry).NextFileEntry]
			}
			(*fentry).NextFileEntry = newEntryIndex
		}
		return
	}
	// recursive case; find directory if present, otherwise add.
	var currDir *FIoDirectoryIndexEntry
	currDirName := fpathSections[0]
	currDirNameIndex := uint32((*(*structure).strTable)[currDirName])

	possibleNewEntryIndex := uint32(len(*(*structure).dirs))
	if (*r).FirstChildEntry == NoneEntry {
		newDirEntry := FIoDirectoryIndexEntry{
			Name:             currDirNameIndex,
			FirstChildEntry:  NoneEntry,
			NextSiblingEntry: NoneEntry,
			FirstFileEntry:   NoneEntry,
		}
		(*r).FirstChildEntry = possibleNewEntryIndex
		*(*structure).dirs = append(*(*structure).dirs, &newDirEntry)
		currDir = &newDirEntry
	} else {
		dentry := &(*(*structure).dirs)[(*r).FirstChildEntry]

		for (*dentry).Name != currDirNameIndex {
			if (*dentry).NextSiblingEntry != NoneEntry {
				dentry = &(*(*structure).dirs)[(*dentry).NextSiblingEntry]
			} else {
				break
			}
		}
		if (*dentry).Name == currDirNameIndex {
			// directory found
			currDir = (*dentry)
		} else {
			// add new directory
			newDirEntry := FIoDirectoryIndexEntry{
				Name:             currDirNameIndex,
				FirstChildEntry:  NoneEntry,
				NextSiblingEntry: NoneEntry,
				FirstFileEntry:   NoneEntry,
			}
			(*dentry).NextSiblingEntry = possibleNewEntryIndex
			*(*structure).dirs = append(*(*structure).dirs, &newDirEntry)
			currDir = &newDirEntry
		}
	}
	(*currDir).AddFile(fpathSections[1:], fIndex, structure)
}

func (c *FIoChunkID) ToHexString() string {
	return fmt.Sprintf("%016x%04x%02x%02x", c.ID, c.Index, c.Padding, c.Type)
}

func FromHexString(s string) FIoChunkID {
	var c FIoChunkID
	v, _ := strconv.ParseUint(s[:16], 16, 64)
	c.ID = uint64(v)
	idx, _ := strconv.ParseUint(s[16:20], 16, 16)
	c.Index = uint16(idx)
	b, _ := strconv.ParseUint(s[20:22], 16, 8)
	c.Padding = uint8(b)
	b, _ = strconv.ParseUint(s[22:], 16, 8)
	c.Type = uint8(b)
	return c
}

type FIoStoreTocCompressedBlockEntry struct {
	Offset            [5]uint8
	CompressedSize    [3]uint8
	UncompressedSize  [3]uint8
	CompressionMethod uint8
}
type FIoOffsetAndLength struct {
	Offset [5]uint8
	Length [5]uint8
}

type FIoStoreTocEntryMeta struct {
	ChunkHash FIoChunkHash
	Flags     FIoStoreTocEntryMetaFlags
}

type FIoChunkHash struct {
	Hash    [20]uint8 //SHA1
	Padding [12]uint8
}

func normalize(s []byte) []byte {
	return append(s, make([]byte, 8-len(s))...)
}
func (f *FIoOffsetAndLength) GetOffset() uint64 {
	return uint64(f.Offset[4]) |
		(uint64(f.Offset[3]) << 8) |
		(uint64(f.Offset[2]) << 16) |
		(uint64(f.Offset[1]) << 24) |
		(uint64(f.Offset[0]) << 32)
}
func (f *FIoOffsetAndLength) GetLength() uint64 {
	return uint64(f.Length[4]) |
		(uint64(f.Length[3]) << 8) |
		(uint64(f.Length[2]) << 16) |
		(uint64(f.Length[1]) << 24) |
		(uint64(f.Length[0]) << 32)
}
func (f *FIoOffsetAndLength) SetOffset(offset uint64) {
	f.Offset[0] = uint8(offset >> 32)
	f.Offset[1] = uint8(offset >> 24)
	f.Offset[2] = uint8(offset >> 16)
	f.Offset[3] = uint8(offset >> 8)
	f.Offset[4] = uint8(offset >> 0)
}
func (f *FIoOffsetAndLength) SetLength(length uint64) {
	f.Length[0] = uint8(length >> 32)
	f.Length[1] = uint8(length >> 24)
	f.Length[2] = uint8(length >> 16)
	f.Length[3] = uint8(length >> 8)
	f.Length[4] = uint8(length >> 0)
}

func (f *FIoStoreTocCompressedBlockEntry) GetOffset() uint64 {
	return binary.LittleEndian.Uint64(normalize(f.Offset[:]))
}

func (f *FIoStoreTocCompressedBlockEntry) GetCompressedSize() uint32 {
	return binary.LittleEndian.Uint32(normalize(f.CompressedSize[:]))
}
func (f *FIoStoreTocCompressedBlockEntry) GetUncompressedSize() uint32 {
	return binary.LittleEndian.Uint32(normalize(f.UncompressedSize[:]))
}

func (f *FIoStoreTocCompressedBlockEntry) SetOffset(offset uint64) {
	r := make([]byte, 5)
	for i := uint64(0); i < 5; i++ {
		r[i] = byte((offset >> (i * 8)) & 0xff)
	}
	copy(f.Offset[:], r)
}
func (f *FIoStoreTocCompressedBlockEntry) SetUncompressedSize(size uint32) {
	f.UncompressedSize[0] = uint8(size >> 0)
	f.UncompressedSize[1] = uint8(size >> 8)
	f.UncompressedSize[2] = uint8(size >> 16)
}
func (f *FIoStoreTocCompressedBlockEntry) SetCompressedSize(size uint32) {
	f.CompressedSize[0] = uint8(size >> 0)
	f.CompressedSize[1] = uint8(size >> 8)
	f.CompressedSize[2] = uint8(size >> 16)
}
