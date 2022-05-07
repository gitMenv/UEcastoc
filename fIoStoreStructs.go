package main

import "encoding/binary"

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
	// ID [12]uint8
	ID      uint64
	Index   uint16
	Padding uint8
	Type    uint8
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
	// return binary.LittleEndian.Uint64(normalize(f.Offset[:]))
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
