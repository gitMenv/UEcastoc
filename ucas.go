package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
)

const (
	CompSize = 0x10000
)

var (
	currentFileOffset uint64 = 0
)

func (s *FIoStoreTocResourceInfo) readUserData(userData uint32) *[]byte {
	var data []byte
	chunkID := s.ChunkIDs[userData]
	offlen := s.ChunkIDToOfflengths[chunkID]
	firstBlockIndex := (offlen.GetOffset() / uint64(s.Header.CompressionBlockSize))
	alignedLastBlock := (offlen.GetOffset() + offlen.GetLength() + uint64(s.Header.CompressionBlockSize)) - 1 & ^uint64(s.Header.CompressionBlockSize-1)
	lastBlockIndex := alignedLastBlock / uint64(s.Header.CompressionBlockSize)
	if userData != uint32(len(s.ChunkIDs))-1 {
		nextOfflen := s.ChunkIDToOfflengths[s.ChunkIDs[userData+1]]
		lastBlockIndex = nextOfflen.GetOffset() / uint64(s.Header.CompressionBlockSize) // better method!
	}
	// add the signature to the data at the start and at the end if it's .uasset file
	// -> Is this really necessary? I doubt it...
	// data = append(data, *fetchSignature(filepath)...)
	for i := firstBlockIndex; i < lastBlockIndex; i++ {
		block := s.CompressionBlocks[i]
		data = append(data, *s.decompressBlock(&block)...)
	}

	return &data
}

func (s *FIoStoreTocResourceInfo) unpackFile(index int, exportPath string, blacklist []string) {
	// check if this exportpath is among the blacklist
	for _, black := range blacklist {
		if strings.Contains(exportPath, black) {
			return
		}
	}
	// ensure the directory exists
	os.MkdirAll(exportPath, 0700)
	// get file data
	d := s.DirectoryResource
	file := d.FileEntries[index]
	filepath := exportPath + "/" + string(d.StringTable[file.Name][:len(d.StringTable[file.Name])-1])

	if file.UserData != NoneEntry {
		data := *s.readUserData(file.UserData)
		os.WriteFile(filepath, data[:], 0644)
	}

	if file.NextFileEntry != NoneEntry {
		s.unpackFile(int(file.NextFileEntry), exportPath, blacklist)
	}
}
func (s *FIoStoreTocResourceInfo) unpackDirectory(index int, exportPath string, blacklist []string) {
	d := s.DirectoryResource
	dir := d.DirectoryEntries[index]

	if dir.FirstChildEntry != NoneEntry {
		s.unpackDirectory(int(dir.FirstChildEntry), exportPath+"/"+string(d.StringTable[dir.Name][:len(d.StringTable[dir.Name])-1]), blacklist)
	}
	if dir.NextSiblingEntry != NoneEntry {
		s.unpackDirectory(int(dir.NextSiblingEntry), exportPath, blacklist)
	}
	if dir.FirstFileEntry != NoneEntry {
		s.unpackFile(int(dir.FirstFileEntry), exportPath+"/"+string(d.StringTable[dir.Name][:len(d.StringTable[dir.Name])-1]), blacklist)
	}
}

func (s *FIoStoreTocResourceInfo) UnpackUCAS(exportPath string, blacklist []string) {
	// find the first mountpoint entry child
	s.unpackDirectory(int(s.DirectoryResource.DirectoryEntries[0].FirstChildEntry), exportPath, blacklist)
	if s.openUCASFile != nil {
		s.openUCASFile.Close()
	}
}

func (h *UTocHeader) setConstants() {
	for i := 0; i < len(MagicUtoc); i++ {
		h.Magic[i] = MagicUtoc[i]
	}
	h.Version = VersionUtoc
	h.HeaderSize = uint32(binary.Size(h))

	var comp FIoStoreTocCompressedBlockEntry
	h.CompressedBlockEntrySize = uint32(binary.Size(comp))
	h.CompressionBlockSize = uint32(CompSize)
	h.PartitionCount = 0 // to be explicit
	h.ContainerFlags = (1 << 0) | (1 << 3)

	// will only be using zlib for compression
	h.CompressionMethodNameLength = 32
	h.CompressionMethodNameCount = 1

	// I _think_ it just needs to be unique...
	h.ContainerID = FIoContainerID(rand.Uint64())
}

// compresses a file with a given filepath and puts all compression blocks, chunk IDs and everything into place.
// it returns the chunk ID index that it just created
func (h *FIoStoreTocResourceInfo) compressFile(filepath string) (chunkIDindex uint32) {
	// Do the following here:
	// - Read the file
	// - Calculate hash from the file
	// - Store
	// - Partition the file in compression blocks of size compressionBlockSize
	// -
	data, err := os.ReadFile(filepath)
	if err != nil {
		fmt.Println("ERROR: ", err)
		return 0
	}
	// return the chunk ID index that will be appended now
	chunkIDindex = uint32(len(h.ChunkIDs))

	// create hash and add to structure
	hasher := sha1.New()
	hasher.Write(data)
	fileHash := hasher.Sum(nil)

	var meta FIoStoreTocEntryMeta
	copy(meta.ChunkHash.Hash[:], fileHash[:20])
	meta.ChunkHash.Padding = [12]byte{} // explicitly set to 0
	meta.Flags = 1                      // always has the value 1

	h.ChunkMetas = append(h.ChunkMetas, meta)
	h.ChunkIDs = append(h.ChunkIDs, *createChunkID(filepath))

	// find and make offlenghts
	var newOfflen FIoOffsetAndLength
	newOfflen.SetLength(uint64(len(data)))
	thisOffset := uint64(0)
	if chunkIDindex != 0 {
		thisOffset = h.Offlengths[chunkIDindex-1].GetOffset() + h.Offlengths[chunkIDindex-1].GetLength()
		thisOffset = ((thisOffset + CompSize - 1) / CompSize) * CompSize
	}
	newOfflen.SetOffset(thisOffset)
	h.Offlengths = append(h.Offlengths, newOfflen)

	// now perform the actual compression
	var fullCompressedData []byte
	for len(data) != 0 {
		var chunk []byte
		var block FIoStoreTocCompressedBlockEntry
		chunkLen := len(data)
		if chunkLen > CompSize {
			chunkLen = CompSize
		}
		chunk = data[:chunkLen]
		block.CompressionMethod = 1 // for zlib compression
		block.SetOffset(currentFileOffset)
		block.SetUncompressedSize(uint32(chunkLen))

		// actual compression
		var b bytes.Buffer
		w := zlib.NewWriter(&b)
		w.Write(chunk)
		w.Close()
		compressedChunk := b.Bytes()
		block.SetCompressedSize(uint32(len(compressedChunk)))
		// align compessedChunk to 0x10
		compressedChunk = append(compressedChunk, getRandomBytes((0x10-(len(compressedChunk)%0x10))&(0x10-1))...)
		currentFileOffset += uint64(len(compressedChunk)) // include these "random bytes" for alignment in the offset
		fullCompressedData = append(fullCompressedData, compressedChunk...)
		data = data[chunkLen:]
		h.CompressionBlocks = append(h.CompressionBlocks, block)
	}
	h.openUCASFile.Write(fullCompressedData)

	return chunkIDindex
}

// NOTE: This function can not work multithreaded
func (h *FIoStoreTocResourceInfo) buildDirectoryStructure(path string, strToIdx *map[string]uint32) (dirIndex, fileIndex uint32) {
	files, err := ioutil.ReadDir(path)
	dirIndex = NoneEntry
	fileIndex = NoneEntry
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	var dirList []string
	var fileList []string
	dirOff := uint32(len(h.DirectoryResource.DirectoryEntries))
	fileOff := uint32(len(h.DirectoryResource.FileEntries))

	blankDirEntry := FIoDirectoryIndexEntry{NoneEntry, NoneEntry, NoneEntry, NoneEntry}
	blankFileEntry := FIoFileIndexEntry{NoneEntry, NoneEntry, NoneEntry}

	for _, file := range files {
		fname := file.Name()
		if _, exists := (*strToIdx)[fname]; !exists {
			h.DirectoryResource.StringTable = append(h.DirectoryResource.StringTable, FString(fname))
			(*strToIdx)[fname] = uint32(len(h.DirectoryResource.StringTable) - 1)
		}
		if file.IsDir() {
			dirList = append(dirList, fname)
			h.DirectoryResource.DirectoryEntries = append(h.DirectoryResource.DirectoryEntries, blankDirEntry)
		} else {
			fileList = append(fileList, fname)
			h.DirectoryResource.FileEntries = append(h.DirectoryResource.FileEntries, blankFileEntry)
		}
	}

	for i, dir := range dirList {
		dirIdx := dirOff + uint32(i)
		hasSibling := i != len(dirList)-1
		h.DirectoryResource.DirectoryEntries[dirIdx].Name = (*strToIdx)[dir]
		if hasSibling {
			h.DirectoryResource.DirectoryEntries[dirIdx].NextSiblingEntry = dirIdx + 1
		}
		cDir, cFile := h.buildDirectoryStructure(path+"/"+dir, strToIdx)
		h.DirectoryResource.DirectoryEntries[dirIdx].FirstChildEntry = cDir
		h.DirectoryResource.DirectoryEntries[dirIdx].FirstFileEntry = cFile
	}

	for i, file := range fileList {
		fileIdx := fileOff + uint32(i)
		hasSibling := i != len(fileList)-1
		h.DirectoryResource.FileEntries[fileIdx].Name = (*strToIdx)[file]
		if hasSibling {
			h.DirectoryResource.FileEntries[fileIdx].NextFileEntry = fileIdx + 1
		}
		h.DirectoryResource.FileEntries[fileIdx].UserData = h.compressFile(path + "/" + file)
	}
	if len(dirList) > 0 {
		dirIndex = dirOff
	}
	if len(fileList) > 0 {
		fileIndex = fileOff
	}
	return
}

func PackToUCAS(directory string) {
	var resource FIoStoreTocResourceInfo
	currentFileOffset = 0
	resource.Header.setConstants()
	// also set the mountpoint
	resource.DirectoryResource.MountPoint = FString("../../../")

	// set the current directory as the mount directory
	err := os.Chdir(directory)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	// create the directory index resource
	resource.filepath, err = os.Getwd() // "mount directory", relative from this, will be ../.
	if err != nil {
		fmt.Println("err:", err)
	}
	// before packing the file, first write data for the userData == 0 case
	// I have no idea what this data represents...
	// for now, place some empty data.

	// ensure file is empty; remove file first, ignore any errors (could be problematic)
	ucasPath := resource.filepath + "/../" + "packed_P.ucas"
	os.Remove(ucasPath)

	resource.openUCASFile, err = os.OpenFile(ucasPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer resource.openUCASFile.Close()
	// add first empty datapoint
	resource.Offlengths = append(resource.Offlengths, FIoOffsetAndLength{[5]uint8{0, 0, 0, 0, 0}, [5]uint8{0, 0, 0, 0, 0}}) // set 0 for now

	// NOT SURE if this is necessary, or whether this is even correct...
	var cid FIoChunkID
	cid.ID = rand.Uint64()
	cid.Index = 0
	cid.Padding = 0
	cid.Type = 10 // for the first thing, it's always 10 for some reason
	resource.ChunkIDs = append(resource.ChunkIDs, cid)
	resource.CompressionBlocks = append(resource.CompressionBlocks, FIoStoreTocCompressedBlockEntry{
		Offset:            [5]uint8{0, 0, 0, 0, 0},
		CompressedSize:    [3]uint8{0, 0, 0},
		UncompressedSize:  [3]uint8{0, 0, 0},
		CompressionMethod: 0})
	// build the file structure
	strToIdx := make(map[string]uint32)
	var rootDirEntry FIoDirectoryIndexEntry
	rootDirEntry.Name = NoneEntry
	rootDirEntry.NextSiblingEntry = NoneEntry
	resource.DirectoryResource.DirectoryEntries = append(resource.DirectoryResource.DirectoryEntries, rootDirEntry)
	dir, file := resource.buildDirectoryStructure(resource.filepath, &strToIdx)
	resource.DirectoryResource.DirectoryEntries[0].FirstChildEntry = dir
	resource.DirectoryResource.DirectoryEntries[0].FirstFileEntry = file

	resource.Header.HeaderSize = uint32(binary.Size(resource.Header))
	resource.Header.EntryCount = uint32(len(resource.ChunkIDs))
	resource.Header.CompressedBlockEntryCount = uint32(len(resource.CompressionBlocks))

	// directory index resource thing consists of:
	// mount point string length, length of string + nullbyte
	// then, a uint32 count for the number of directory entries + variable number of entries
	// count for file entries + variable number of entries
	// count for number of strings + variable number of strings
	var dummyFileEntry FIoFileIndexEntry
	mountPointStuff := uint32(4 + len(resource.DirectoryResource.MountPoint) + 1)
	dirEntries := uint32(4 + len(resource.DirectoryResource.DirectoryEntries)*binary.Size(resource.DirectoryResource.DirectoryEntries[0]))
	fileEntries := uint32(4 + len(resource.DirectoryResource.FileEntries)*binary.Size(dummyFileEntry))
	stringEntries := uint32(4)

	for _, s := range resource.DirectoryResource.StringTable {
		stringEntries += 4                  // for storing strlen + 1
		stringEntries += uint32(len(s)) + 1 // include  nullbyte
	}
	resource.Header.DirectoryIndexSize = mountPointStuff + dirEntries + fileEntries + stringEntries
	// everything was written to the .ucas file.
	// Now we generate a .utoc file
	resource.WriteTOC()
	for _, b := range resource.CompressionBlocks {
		fmt.Printf("Blockrange: [%d, %d] (length: %d)\n", b.GetOffset(), b.GetOffset()+uint64(b.GetCompressedSize()), b.GetCompressedSize())
	}

	fmt.Println("_________________")
	fmt.Println("len", len(resource.ChunkIDs), len(resource.Offlengths), len(resource.CompressionBlocks))
}
