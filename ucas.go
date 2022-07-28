package uecastoc

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"os"
)

const (
	CompSize = 0x10000
)

// this function compresses all files and updates the file information accordingly,
// including the offset, chunks, and hash
// this operation may take quite some time
func compressFilesToUcas(finfos *[]*FileInfo, unpackPath string, fd *os.File) {
	for _, f := range *finfos {
		data, err := os.ReadFile(f.getFilePath(unpackPath))
		if err != nil {
			fmt.Println("err: ", err)
			return
		}

		f.FileHash = *sha1Hash(&data)
		f.length = uint64(len(data))
		signedOffset, _ := fd.Seek(0, os.SEEK_CUR)
		f.offset = uint64(signedOffset)

		// now perform compression, write per compressed block
		for len(data) != 0 {
			var chunk []byte
			var block FIoStoreTocCompressedBlockEntry
			chunkLen := len(data)
			if chunkLen > CompSize {
				chunkLen = CompSize
			}
			chunk = data[:chunkLen]
			block.CompressionMethod = 1 // for zlib compression
			currOffset, _ := fd.Seek(0, os.SEEK_CUR)
			block.SetOffset(uint64(currOffset))
			block.SetUncompressedSize(uint32(chunkLen))

			// actual compression
			// This may not be 100% correct; comparing to the original ucas files, the zlib compression size
			// differs slightly.
			// the original files seem to always be an even number of bytes long
			var b bytes.Buffer
			w := zlib.NewWriter(&b)
			w.Write(chunk)
			w.Close()
			compressedChunk := b.Bytes()
			block.SetCompressedSize(uint32(len(compressedChunk)))
			// align compessedChunk to 0x10
			compressedChunk = append(compressedChunk, getRandomBytes((0x10-(len(compressedChunk)%0x10))&(0x10-1))...)
			data = data[chunkLen:]
			f.blocks = append(f.blocks, block)

			fd.Write(compressedChunk)
		}

	}
}

// sets magic word, version, header size, sizes of structures,
// partitioncount, containerflags & compressionCount
func (h *UTocHeader) setConstants() {
	for i := 0; i < len(MagicUtoc); i++ {
		h.Magic[i] = MagicUtoc[i]
	}
	h.Version = VersionUtoc
	h.HeaderSize = uint32(binary.Size(h))

	var comp FIoStoreTocCompressedBlockEntry
	h.CompressedBlockEntrySize = uint32(binary.Size(comp))
	h.CompressionBlockSize = uint32(CompSize)
	h.PartitionCount = 1 // to be explicit
	h.ContainerFlags = (1 << 0) | (1 << 3)

	// will only be using zlib for compression
	h.CompressionMethodNameLength = 32
	h.CompressionMethodNameCount = 1
	for i := 7; i < 15; i++ {
		h.Padding[i] = 0xff
	}
}
