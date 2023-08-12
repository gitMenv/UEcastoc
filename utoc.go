package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

const (
	MagicUtoc       string = "-==--==--==--==-"
	UnrealSignature string = "\xC1\x83\x2A\x9E"
	MountPoint      string = "../../../"
	NoneEntry       uint32 = 0xffffffff
)
const (
	VersionInvalid                 uint8 = iota
	VersionInitial                       = iota
	VersionDirectoryIndex                = iota
	VersionPartitionSize                 = iota
	VersionPerfectHash                   = iota
	VersionPerfectHashWithOverflow       = iota
	VersionLatest                        = iota
)

type EIoContainerFlags uint8

const (
	NoneContainerFlag       EIoContainerFlags = 0
	CompressedContainerFlag                   = 1 << 0
	EncryptedContainerFlag                    = 1 << 1
	SignedContainerFlag                       = 1 << 2
	IndexedContainerFlag                      = 1 << 3
)

type FGuid struct {
	A, B, C, D uint32
}

type UTocHeader struct {
	Magic                            [16]byte
	Version                          uint8    // Current options are Initial(1), DirectoryIndex(2), PartitionSize(3)
	Reserved0                        [3]uint8 // actually a uint8 and uint16
	HeaderSize                       uint32   // value is 144
	EntryCount                       uint32
	CompressedBlockEntryCount        uint32
	CompressedBlockEntrySize         uint32 // they say "For sanity checking"
	CompressionMethodNameCount       uint32
	CompressionMethodNameLength      uint32
	CompressionBlockSize             uint32
	DirectoryIndexSize               uint32
	PartitionCount                   uint32 // should be 0
	ContainerID                      FIoContainerID
	EncryptionKeyGuid                FGuid
	ContainerFlags                   EIoContainerFlags
	Reserved1                        [3]byte
	TocChunkPerfectHashSeedsCount    uint32
	PartitionSize                    uint64
	TocChunksWithoutPerfectHashCount uint32
	Reserved2                        [44]byte
}

func (h *UTocHeader) isEncrypted() bool {
	return h.ContainerFlags&EncryptedContainerFlag != 0
}

// ucas file consists of files. For each file, there is an entry with this data.
// It states where you can find which file in the ucas file.
type GameFileMetaData struct {
	filepath          string
	chunkID           FIoChunkID
	offlen            FIoOffsetAndLength
	compressionBlocks []FIoStoreTocCompressedBlockEntry
	metadata          FIoStoreTocEntryMeta
}

type UTocData struct {
	hdr                UTocHeader
	mountPoint         string
	files              []GameFileMetaData
	compressionMethods []string
}

type GameFilePathData struct {
	fpath    string
	userData uint32
}

func (u *UTocData) unpackDependencies(ucasPath string) (*[]byte, error) {
	// find dependency file independent of index
	var depfile GameFileMetaData
	// depfile := u.files[len(u.files)-1]
	for _, f := range u.files {
		if f.filepath == DepFileName {
			depfile = f
		}
	}
	if depfile.filepath != DepFileName {
		fmt.Println(depfile.filepath)
		return nil, errors.New("could not derive dependencies")
	}
	// open ucas file
	openUcas, err := os.Open(ucasPath)
	if err != nil {
		return nil, err
	}
	var compressionblockData [][]byte
	for _, b := range depfile.compressionBlocks {
		_, err = openUcas.Seek(int64(b.GetOffset()), 0)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, b.GetCompressedSize())
		readBytes, err := openUcas.Read(buf)
		if err != nil {
			return nil, err
		}
		if uint32(readBytes) != b.GetCompressedSize() {
			return nil, errors.New("could not read the correct size")
		}
		compressionblockData = append(compressionblockData, buf)
	}
	// all separate blocks collected for file unpacking
	outputData := []byte{}
	for i := 0; i < len(compressionblockData); i++ {
		method := u.compressionMethods[depfile.compressionBlocks[i].CompressionMethod]
		decomp := getDecompressionFunction(method)
		if decomp == nil {
			return nil, errors.New(fmt.Sprintf("decompression method %s not known", method))
		}
		newData, err := decomp(&(compressionblockData[i]), depfile.compressionBlocks[i].GetUncompressedSize())
		if err != nil {
			return nil, err
		}
		outputData = append(outputData, (*newData)...)
	}
	return &outputData, nil
}

func recursiveDirExplorer(parentPath string, pDir uint32, outputList *[]GameFilePathData,
	strTable *[]string, dirs *[]FIoDirectoryIndexEntry, files *[]FIoFileIndexEntry) {

	dirIdx := (*dirs)[pDir].FirstChildEntry
	fileIdx := (*dirs)[pDir].FirstFileEntry
	if dirIdx == NoneEntry && fileIdx == NoneEntry { // base case
		return
	}

	for dirIdx != NoneEntry {
		dirEntry := (*dirs)[dirIdx]
		newDirName := parentPath + "/" + (*strTable)[dirEntry.Name]
		recursiveDirExplorer(newDirName, dirIdx, outputList, strTable, dirs, files)
		dirIdx = dirEntry.NextSiblingEntry
	}
	for fileIdx != NoneEntry {
		fileEntry := (*files)[fileIdx]
		filepath := parentPath + "/" + (*strTable)[fileEntry.Name]
		(*outputList) = append((*outputList), GameFilePathData{fpath: filepath, userData: fileEntry.UserData})
		fileIdx = fileEntry.NextFileEntry
	}
}

// returned is a slice of filepaths in the correct order.
// meaning that the indices correspond to their file-index userData field.
func parseDirectoryIndex(r *bytes.Reader, numberOfChunks int) (mountpoint string, filepaths *[]string) {
	var size, dirCount, fileCount, stringCount uint32
	var dirs []FIoDirectoryIndexEntry
	var files []FIoFileIndexEntry
	var strTable []string

	// mount point string
	binary.Read(r, binary.LittleEndian, &size)
	mntPt := make([]byte, size)
	binary.Read(r, binary.LittleEndian, &mntPt)
	mountPointName := string(mntPt[:len(mntPt)-1])
	if !strings.HasPrefix(mountPointName, MountPoint) {
		return "", nil
	}
	mountpoint = strings.TrimPrefix(mountPointName, MountPoint)
	var dirEntry FIoDirectoryIndexEntry
	binary.Read(r, binary.LittleEndian, &dirCount)
	for i := 0; uint32(i) < dirCount; i++ {
		binary.Read(r, binary.LittleEndian, &dirEntry)
		dirs = append(dirs, dirEntry)
	}

	var fileEntry FIoFileIndexEntry
	binary.Read(r, binary.LittleEndian, &fileCount)
	for i := 0; uint32(i) < fileCount; i++ {
		binary.Read(r, binary.LittleEndian, &fileEntry)
		files = append(files, fileEntry)
	}

	binary.Read(r, binary.LittleEndian, &stringCount)
	for i := 0; uint32(i) < stringCount; i++ {
		binary.Read(r, binary.LittleEndian, &size)
		newString := make([]byte, size)
		binary.Read(r, binary.LittleEndian, &newString)
		strTable = append(strTable, string(newString[:len(newString)-1]))
	}
	if dirs[0].Name != NoneEntry {
		return "", nil
	}

	var gamefilePaths []GameFilePathData

	recursiveDirExplorer("", 0, &gamefilePaths, &strTable, &dirs, &files)

	// order the filepaths according to their userdata
	orderedPaths := make([]string, numberOfChunks)
	for _, v := range gamefilePaths {
		orderedPaths[v.userData] = v.fpath
	}

	return mountpoint, &orderedPaths
}

func parseUtocHeader(r *bytes.Reader) (hdr UTocHeader, err error) {
	// read the header of the .utoc file
	err = binary.Read(r, binary.LittleEndian, &hdr)
	if err != nil {
		return hdr, err
	}
	if string(hdr.Magic[:]) != MagicUtoc {
		return hdr, errors.New("magic word of .utoc file was not found")
	}
	if hdr.Version < VersionDirectoryIndex {
		return hdr, errors.New("utoc version is outdated")
	}
	if hdr.Version > VersionLatest {
		return hdr, errors.New("too new utoc version")
	}

	if hdr.Version < VersionPartitionSize {
		hdr.PartitionCount = 1
		hdr.PartitionSize = 0xffffffffffffffff // limit of uint64
		fmt.Println("Warning: this is a version of the utoc file format that may not be supported yet")
	}
	if hdr.CompressedBlockEntrySize != 12 { // must be sizeof FIoStoreTocCompressedBlockEntry
		return hdr, errors.New("compressed block entry size was incorrect")
	}

	if hdr.ContainerFlags != 0 && uint8(hdr.ContainerFlags)&SignedContainerFlag != 0 {
		// the reference project may contain flags here, but no idea what it should do...
		return hdr, errors.New("the unreal engine dictates that this is an error. No idea why (yet)... Sorry!")
	}

	return hdr, nil
}

// the UTocData can be used to extract all information from the ucas files
func parseUtocFile(utocFile string, aesKey []byte) (*UTocData, error) {
	var udata UTocData
	b, err := ioutil.ReadFile(utocFile)
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(b)

	udata.hdr, err = parseUtocHeader(r)
	if err != nil {
		return nil, err
	}
	if udata.hdr.isEncrypted() {
		if len(aesKey) == 0 {
			return &udata, errors.New("encrypted file, but no AES key was provided! Please pass the aes key as a string in hexadecimal format")
		}
	}

	// parse the following four sections of the file
	var chunkIDs []FIoChunkID
	var offlengths []FIoOffsetAndLength
	var perfectHashSeeds []uint32
	var withoutPerfectHashes []uint32
	var compressionBlocks []FIoStoreTocCompressedBlockEntry
	var filepaths []string
	var metas []FIoStoreTocEntryMeta

	// following the header is a list of chunk IDs.
	var chunkID FIoChunkID
	for i := 0; i < int(udata.hdr.EntryCount); i++ {
		binary.Read(r, binary.LittleEndian, &chunkID)
		chunkIDs = append(chunkIDs, chunkID)
	}

	var offlen FIoOffsetAndLength
	for i := 0; i < int(udata.hdr.EntryCount); i++ {
		binary.Read(r, binary.LittleEndian, &offlen)
		offlengths = append(offlengths, offlen)
	}
	// if there are Perfect Hash Seeds ?? then these must be parsed before the compression blocks
	var hashSeed uint32
	for i := 0; i < int(udata.hdr.TocChunkPerfectHashSeedsCount); i++ {
		binary.Read(r, binary.LittleEndian, &hashSeed)
		perfectHashSeeds = append(perfectHashSeeds, hashSeed)
	}
	// same goes for the chunks without perfect hashes???
	for i := 0; i < int(udata.hdr.TocChunksWithoutPerfectHashCount); i++ {
		binary.Read(r, binary.LittleEndian, &hashSeed)
		withoutPerfectHashes = append(withoutPerfectHashes, hashSeed)
	}

	// read compression blocks
	var cBlock FIoStoreTocCompressedBlockEntry
	for i := 0; i < int(udata.hdr.CompressedBlockEntryCount); i++ {
		binary.Read(r, binary.LittleEndian, &cBlock)
		compressionBlocks = append(compressionBlocks, cBlock)
	}
	// read compression methods
	udata.compressionMethods = append(udata.compressionMethods, "None")

	method := make([]byte, udata.hdr.CompressionMethodNameLength)
	for i := 0; i < int(udata.hdr.CompressionMethodNameCount); i++ {
		binary.Read(r, binary.LittleEndian, &method)
		udata.compressionMethods = append(udata.compressionMethods, string(bytes.Trim([]byte(method[:]), "\x00")))
	}

	// read directory index, but only if the containerFlags states that is present. TODO?
	dirIndexBuffer := make([]byte, udata.hdr.DirectoryIndexSize)
	binary.Read(r, binary.LittleEndian, &dirIndexBuffer) // normal reader is advanced here as well

	if udata.hdr.isEncrypted() {
		plaintext, err := decryptAES(&dirIndexBuffer, aesKey)
		if err != nil {
			return &udata, err
		}
		dirIndexBuffer = *plaintext
	}
	dirReader := bytes.NewReader(dirIndexBuffer)

	mntPt, fpaths := parseDirectoryIndex(dirReader, len(chunkIDs))
	udata.mountPoint = mntPt
	if fpaths == nil {
		return &udata, errors.New("something went wrong parsing the directory index!")
	}
	filepaths = *fpaths

	// read file chunk metas
	var meta FIoStoreTocEntryMeta
	for i := 0; i < int(udata.hdr.EntryCount); i++ {
		binary.Read(r, binary.LittleEndian, &meta)
		metas = append(metas, meta)
	}
	//temporary dependency "file" because it isn't always the last chunk
	var foundDeps bool
	var path string
	// aggregate file data
	for i, v := range filepaths {
		startBlock := offlengths[i].GetOffset() / uint64(udata.hdr.CompressionBlockSize)
		// hacky way of rounding the length to the next multiple of the compressionblocksize and intcasting
		endBlock := startBlock + (offlengths[i].GetLength()+(uint64(udata.hdr.CompressionBlockSize)-1))/uint64(udata.hdr.CompressionBlockSize)
		blocks := compressionBlocks[startBlock:endBlock]
		if v == "" {
			// check for "dependencies" chunk via type instead of assuming it's last.
			// in the sample im running this on, the chunkID matches with the one in the header.
			if chunkIDs[i].Type != 10 && uint64(udata.hdr.ContainerID) != chunkIDs[i].ID {
				// if the name is empty, the type is 10, and the chunkID doesnt match, then it's not the dependencies
				continue
			}
			foundDeps = true
			path = DepFileName
		} else {
			path = v
		}
		udata.files = append(udata.files, GameFileMetaData{
			filepath:          path,
			chunkID:           chunkIDs[i],
			offlen:            offlengths[i],
			compressionBlocks: blocks,
			metadata:          metas[i],
		})
	}
	if !foundDeps {
		return &udata, errors.New("couldn't find dependencies")
	}
	// the final file in the list will have filepath "dependencies"
	// //manually stick this on at the end for compatibility?
	// udata.files = append(udata.files, deps)
	return &udata, nil
}
