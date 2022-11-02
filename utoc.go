package uecastoc

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

const (
	MagicUtoc       string = "-==--==--==--==-"
	VersionUtoc     uint32 = 3
	LegacyUtoc      uint32 = 2
	UnrealSignature string = "\xC1\x83\x2A\x9E"
	MountPoint      string = "../../../"
	NoneEntry       uint32 = 0xffffffff
	PackFileName    string = "Packed_P"
)

var (
	ErrWrongMagic         = errors.New("error: magic number was not found")
	ErrUnknownUtocVersion = errors.New("error: utoc version is unknown")
	ErrContainerFlag      = errors.New("WARNING: the container flags were not 0 nor the indexed flag, so it could be a special case")
	ErrExpectedBytesRead  = errors.New("expected to have read all bytes")
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

type FileInfo struct {
	Name          string
	ID            uint64
	Parent        *Directory
	Dependencies  []uint64
	ExportObjects uint32
	offset        uint64
	length        uint64
	blocks        []FIoStoreTocCompressedBlockEntry
	chunkType     uint8
	FileHash      FIoChunkHash
	someValue     uint32
	randomIndex   uint64
}

// gets the relative path based on the prefix provided
func (f *FileInfo) getFilePath(prefix string) string {
	path := f.Name
	parent := f.Parent

	for parent != nil {
		if parent.Name == MountPoint {
			path = prefix + path
			break
		}
		path = parent.Name + "/" + path
		parent = parent.Parent
	}
	return path
}

// just to get unique paths
func (d *Directory) getDirPath() string {
	path := d.Name + "/"
	parent := d.Parent

	for parent != nil {
		if parent.Name == MountPoint {
			break
		}
		path = parent.Name + "/" + path
		parent = parent.Parent
	}
	return path
}

// Fetches the compressed file from an open UCAS file and returns the data decompressed
func (f *FileInfo) decompress(openUCASfile *os.File) (*[]byte, error) {
	var data []byte
	for _, b := range f.blocks {
		_, err := openUCASfile.Seek(int64(b.GetOffset()), 0)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, b.GetCompressedSize())
		readBytes, err := openUCASfile.Read(buf)
		if err != nil {
			return nil, err
		}
		if uint32(readBytes) != b.GetCompressedSize() {
			return nil, errors.New("could not read the correct size")
		}
		// now decompress
		switch b.CompressionMethod {
		case 0:
			data = append(data, buf...)
		case 1:
			// decompress with zlib
			r, err := zlib.NewReader(bytes.NewBuffer(buf))
			defer r.Close()
			if err != nil {
				return nil, err
			}
			uncompressed, err := ioutil.ReadAll(r)
			if err != nil {
				return nil, err
			}
			if len(uncompressed) != int(b.GetUncompressedSize()) {
				return nil, errors.New("didn't decompress correctly")
			}
			data = append(data, uncompressed...)
		default:
			return nil, errors.New("unknown compression method")
		}

	}

	return &data, nil
}

type Directory struct {
	Name      string
	ChildDirs []*Directory
	Files     []*FileInfo
	Parent    *Directory
}

// goes through all the directories and returns the list of files
// This is useful for compressing the files in order and keeping that structure for UserData
func (d *Directory) flatten() (*[]*FileInfo, *[]*Directory) {
	finfo := []*FileInfo{}
	dirs := []*Directory{}

	dirStack := []*Directory{d}
	for len(dirStack) != 0 {
		currDir := dirStack[0]
		dirStack = dirStack[1:]
		dirStack = append(dirStack, currDir.ChildDirs...)
		dirs = append(dirs, currDir)
		finfo = append(finfo, currDir.Files...)
	}
	return &finfo, &dirs
}

func (d *Directory) constructStringTable() *[]string {
	strTable := []string{}
	unique := make(map[string]bool)

	dirStack := []*Directory{d}
	for len(dirStack) != 0 {
		currDir := dirStack[0]
		dirStack = dirStack[1:]
		dirStack = append(dirStack, currDir.ChildDirs...)
		if _, ok := unique[currDir.Name]; !ok {
			if currDir.Name == MountPoint {
				continue
			}
			strTable = append(strTable, currDir.Name)
			unique[currDir.Name] = true
		}
		for _, file := range currDir.Files {
			// there can actually not be duplicate names for files
			if _, ok := unique[file.Name]; !ok {
				strTable = append(strTable, file.Name)
				unique[file.Name] = true
			}
		}
	}
	return &strTable
}

func (d *Directory) getFirstSibling() *Directory {
	p := d.Parent
	if p == nil {
		return nil
	}
	dirs := p.ChildDirs
	myIndex := 0
	for i, v := range dirs {
		if d == v {
			myIndex = i
			break
		}
	}
	if len(dirs)-1 > myIndex {
		return dirs[myIndex+1]
	}
	return nil
}
func (d *Directory) getFirstChildDir() *Directory {
	if len(d.ChildDirs) == 0 {
		return nil
	}
	return d.ChildDirs[0]
}
func (d *Directory) getFirstChildFile() *FileInfo {
	if len(d.Files) == 0 {
		return nil
	}
	return d.Files[0]
}
func (f *FileInfo) getFileSibling() *FileInfo {
	p := f.Parent
	files := p.Files
	myIndex := 0
	for i, file := range files {
		if file == f {
			myIndex = i
			break
		}
	}
	if len(files)-1 > myIndex {
		return files[myIndex+1]
	}
	return nil
}
func (d *Directory) constructDirectoryIndex() *[]byte {
	var dirIndex FIoDirectoryIndexResource

	dirIndex.MountPoint = FString(MountPoint)
	strTable := d.constructStringTable()
	flattenedFiles, flattenedDirs := d.flatten()

	nameToIdx := make(map[string]uint32)
	dirToIdx := make(map[string]uint32)
	strToIdx := make(map[string]uint32)

	for i, finfo := range *flattenedFiles {
		nameToIdx[finfo.Name] = uint32(i)
	}
	for i, dir := range *flattenedDirs {
		dirToIdx[dir.getDirPath()] = uint32(i)
	}
	for i, str := range *strTable {
		strToIdx[str] = uint32(i)
		dirIndex.StringTable = append(dirIndex.StringTable, FString(str))
	}
	// iterate through directories and create entries
	for _, dir := range *flattenedDirs {
		dentry := FIoDirectoryIndexEntry{NoneEntry, NoneEntry, NoneEntry, NoneEntry}
		if dir.Name != MountPoint {
			dentry.Name = strToIdx[dir.Name]
		}
		fcd := dir.getFirstChildDir()
		if fcd != nil {
			dentry.FirstChildEntry = dirToIdx[fcd.getDirPath()]
		}
		fsd := dir.getFirstSibling()
		if fsd != nil {
			dentry.NextSiblingEntry = dirToIdx[fsd.getDirPath()]
		}
		ffe := dir.getFirstChildFile()
		if ffe != nil {
			dentry.FirstFileEntry = nameToIdx[ffe.Name]
		}
		dirIndex.DirectoryEntries = append(dirIndex.DirectoryEntries, dentry)
	}
	for _, file := range *flattenedFiles {
		fentry := FIoFileIndexEntry{strToIdx[file.Name], NoneEntry, nameToIdx[file.Name]}
		sibling := file.getFileSibling()
		if sibling != nil {
			fentry.NextFileEntry = nameToIdx[sibling.Name]
		}
		dirIndex.FileEntries = append(dirIndex.FileEntries, fentry)
	}
	return dirIndex.getBytes()
}

type CasToc struct {
	Root        *Directory
	ContainerID uint64
}

// four components that make up a unique identifier.
// Seems a bit overkill, but: https://github.com/EpicGames/UnrealEngine/blob/release/Engine/Source/Runtime/Core/Public/Misc/Guid.h
type FGuid struct {
	A, B, C, D uint32
}

// useful: https://github.com/jashking/UnrealPakViewer/blob/master/PakAnalyzer/Private/IoStoreDefines.h
// Unreal Docs: https://github.com/EpicGames/UnrealEngine/blob/99b6e203a15d04fc7bbbf554c421a985c1ccb8f1/Engine/Source/Runtime/Core/Private/IO/IoStore.h
type UTocHeader struct {
	Magic                       [16]byte
	Version                     uint32 // they state it's uint8, but I doubt it
	HeaderSize                  uint32
	EntryCount                  uint32
	CompressedBlockEntryCount   uint32
	CompressedBlockEntrySize    uint32 // they say "For sanity checking"
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
	IDToChunk            map[uint64]FIoChunkID
	ChunkIDToIndex       map[FIoChunkID]uint32
	ChunkIDToOfflengths  map[FIoChunkID]FIoOffsetAndLength
	CompressionBlocks    []FIoStoreTocCompressedBlockEntry
	ChunkMetas           []FIoStoreTocEntryMeta
	ChunkBlockSignatures []FSHAHash
	CompressionMethods   []FName
	DirectoryResource    FIoDirectoryIndexResource
	// DirectoryIndexBuffer []byte
	Offlengths []FIoOffsetAndLength
}

type FIoDirectoryIndexResource struct {
	MountPoint       FString
	DirectoryEntries []FIoDirectoryIndexEntry
	FileEntries      []FIoFileIndexEntry
	StringTable      []FString
}

type UTocData struct {
	IDs        []FIoChunkID
	Offlengths []FIoOffsetAndLength
	CBlocks    []FIoStoreTocCompressedBlockEntry
	Metas      []FIoStoreTocEntryMeta
	BlockSize  uint32
	Deps       *Dependencies
	FNameToID  map[string]uint64
}

func (u *UTocData) writeUtocFile(f *os.File, root *Directory) {
	// first, construct the directory index, as the size
	// is required for the header
	dirIndex := root.constructDirectoryIndex()

	// header
	hdr := UTocHeader{
		ContainerID:               FIoContainerID(u.Deps.ThisPackageID),
		DirectoryIndexSize:        uint32(len(*dirIndex)),
		EntryCount:                uint32(len(u.IDs)),
		CompressedBlockEntryCount: uint32(len(u.CBlocks)),
	}
	hdr.setConstants()

	// hdr is all set, now write whole utoc file
	binary.Write(f, binary.LittleEndian, hdr)

	// chunk IDs
	for _, chid := range u.IDs {
		binary.Write(f, binary.LittleEndian, chid)
	}

	// offsets and lengths
	for _, offlength := range u.Offlengths {
		binary.Write(f, binary.LittleEndian, offlength)
	}

	// compressionBlocks
	for _, block := range u.CBlocks {
		binary.Write(f, binary.LittleEndian, block)
	}

	// compression Methods (always the same)
	f.Write(getBytesCompressionMethod())

	// directory index
	f.Write(*dirIndex)

	// chunk Metas
	for _, meta := range u.Metas {
		binary.Write(f, binary.LittleEndian, meta)
	}
	// file was written in full
}

func recursiveExplorer(parent *Directory, pDir uint32, strTable *[]string, dirs *[]FIoDirectoryIndexEntry, files *[]FIoFileIndexEntry, dat *UTocData) {
	dirIdx := (*dirs)[pDir].FirstChildEntry
	fileIdx := (*dirs)[pDir].FirstFileEntry
	if dirIdx == NoneEntry && fileIdx == NoneEntry { // base case
		return
	}

	for dirIdx != NoneEntry {
		dirEntry := (*dirs)[dirIdx]
		newDirectory := Directory{Name: (*strTable)[dirEntry.Name], Parent: parent}
		parent.ChildDirs = append(parent.ChildDirs, &newDirectory)
		recursiveExplorer(&newDirectory, dirIdx, strTable, dirs, files, dat)
		dirIdx = dirEntry.NextSiblingEntry
	}
	for fileIdx != NoneEntry {
		fileEntry := (*files)[fileIdx]
		startBlock := dat.Offlengths[fileEntry.UserData].GetOffset() / uint64(dat.BlockSize)
		endBlock := dat.Offlengths[fileEntry.UserData+1].GetOffset() / uint64(dat.BlockSize)
		blocks := dat.CBlocks[startBlock:endBlock]

		newFile := FileInfo{
			Name:          (*strTable)[fileEntry.Name],
			ID:            dat.IDs[fileEntry.UserData].ID,
			chunkType:     dat.IDs[fileEntry.UserData].Type,
			Parent:        parent,
			offset:        dat.Offlengths[fileEntry.UserData].GetOffset(),
			length:        dat.Offlengths[fileEntry.UserData].GetLength(),
			blocks:        blocks,
			Dependencies:  dat.Deps.ChunkIDToDependencies[dat.IDs[fileEntry.UserData].ID].Dependencies,
			ExportObjects: dat.Deps.ChunkIDToDependencies[dat.IDs[fileEntry.UserData].ID].ExportObjects,
		}
		dat.FNameToID[newFile.getFilePath("")] = newFile.ID
		parent.Files = append(parent.Files, &newFile)
		fileIdx = fileEntry.NextFileEntry
	}
}

func parseDirectoryIndex(r *bytes.Reader, dat *UTocData) (root *Directory) {
	rt := Directory{Parent: nil}
	root = &rt
	var size, dirCount, fileCount, stringCount uint32
	var dirEntry FIoDirectoryIndexEntry
	var fileEntry FIoFileIndexEntry
	dirEntries := []FIoDirectoryIndexEntry{}
	fileEntries := []FIoFileIndexEntry{}
	strTable := []string{}

	// starts with the root mount string length
	binary.Read(r, binary.LittleEndian, &size)

	mountPoint := make([]byte, size)
	binary.Read(r, binary.LittleEndian, &mountPoint)
	root.Name = string(mountPoint[:len(mountPoint)-1])

	binary.Read(r, binary.LittleEndian, &dirCount)

	for i := 0; uint32(i) < dirCount; i++ {
		binary.Read(r, binary.LittleEndian, &dirEntry)
		dirEntries = append(dirEntries, dirEntry)
	}

	binary.Read(r, binary.LittleEndian, &fileCount)

	for i := 0; uint32(i) < fileCount; i++ {
		binary.Read(r, binary.LittleEndian, &fileEntry)
		fileEntries = append(fileEntries, fileEntry)
	}

	binary.Read(r, binary.LittleEndian, &stringCount)

	for i := 0; uint32(i) < stringCount; i++ {
		binary.Read(r, binary.LittleEndian, &size)
		newString := make([]byte, size)
		binary.Read(r, binary.LittleEndian, &newString)
		strTable = append(strTable, string(newString[:len(newString)-1]))
	}
	recursiveExplorer(root, 0, &strTable, &dirEntries, &fileEntries, dat)
	return root
}

func getBytesCompressionMethod() []byte {
	// must be only one, which is zlib
	buff := make([]byte, 32)
	zlib := []byte("Zlib")
	copy(buff, zlib)
	return buff
}

func (d *FIoDirectoryIndexResource) getBytes() *[]byte {
	buf := bytes.NewBuffer([]byte{})

	dirCount := uint32(len(d.DirectoryEntries))
	fileCount := uint32(len(d.FileEntries))
	strCount := uint32(len(d.StringTable))
	// mount point string
	mountPointStr := stringToFString(string(d.MountPoint))
	buf.Write(mountPointStr)

	// directory entries
	buf.Write(*uint32ToBytes(&dirCount))

	for _, directoryEntry := range d.DirectoryEntries {
		binary.Write(buf, binary.LittleEndian, directoryEntry)
	}
	buf.Write(*uint32ToBytes(&fileCount))
	for _, fileEntry := range d.FileEntries {
		binary.Write(buf, binary.LittleEndian, fileEntry)
	}
	buf.Write(*uint32ToBytes(&strCount))
	for _, str := range d.StringTable {
		buf.Write(stringToFString(string(str)))
	}
	data := buf.Bytes()
	return &data
}

func readFileInfo(filepath string, parent *Directory, dat *UTocData) *FileInfo {
	splitted := strings.Split(filepath, "/")
	fname := splitted[len(splitted)-1]
	chType := uint8(EIoChunkTypeBulkData)
	if !strings.Contains(fname, ".uasset") {
		chType = EIoChunkTypeOptionalBulkData
	}
	tmpFinfo := FileInfo{Name: fname, Parent: parent}

	fileID := (*dat).FNameToID[tmpFinfo.getFilePath("")]
	dependency := (*dat).Deps.ChunkIDToDependencies[fileID]
	deps := dependency.Dependencies
	exports := dependency.ExportObjects

	fdata, err := os.Stat(filepath)
	if err != nil {
		fmt.Println("err:", err)
		return nil
	}

	newFile := FileInfo{
		Name:          fname,
		ID:            fileID,
		chunkType:     chType,
		Dependencies:  deps,
		ExportObjects: exports,
		someValue:     dependency.MostlyOne,
		randomIndex:   dependency.SomeIndex,
		Parent:        parent,
		length:        uint64(fdata.Size()),
	}

	return &newFile
}

func constructFileTree(dirPath string, parent *Directory, data *UTocData) *Directory {
	splitted := strings.Split(dirPath, "/")
	thisDir := Directory{Name: splitted[len(splitted)-1], Parent: parent}

	fs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		fmt.Println("err:", err)
		return nil
	}
	for _, file := range fs {
		if file.IsDir() {
			thisDir.ChildDirs = append(thisDir.ChildDirs, constructFileTree(dirPath+"/"+file.Name(), &thisDir, data))
		} else {
			// must be file
			thisDir.Files = append(thisDir.Files, readFileInfo(dirPath+"/"+file.Name(), &thisDir, data))
		}
	}
	return &thisDir
}

// PackDirectory builds the directory structure recursively of the entire directory
// Then, it packs the retrieved structure into a UCAS/UTOC file combination.
func PackDirectory(dirPath string) {
	utocPath := dirPath + "../" + PackFileName + ".utoc"
	ucasPath := dirPath + "../" + PackFileName + ".ucas"

	// load the data first
	data, err := loadParsedData()
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	// the pacakge ID of the new package must be unique
	// if not, it overrides the original resulting in errors.
	data.Deps.ThisPackageID = 0xAAAAAAAAAA
	// create root first
	root := Directory{Name: MountPoint, Parent: nil}
	fs, err := ioutil.ReadDir(dirPath)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	for _, file := range fs {
		if file.IsDir() {
			root.ChildDirs = append(root.ChildDirs, constructFileTree(dirPath+"/"+file.Name(), &root, data))
		} else {
			// must be file
			root.Files = append(root.Files, readFileInfo(dirPath+"/"+file.Name(), &root, data))
		}
	}

	dirInfo := CasToc{ContainerID: data.Deps.ThisPackageID, Root: &root}
	f, _ := os.OpenFile(ucasPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	flattened, _ := dirInfo.Root.flatten()

	fmt.Println("compressing files into .ucas file")
	fmt.Println("number of files:", len(*flattened))
	compressFilesToUcas(flattened, dirPath, f)

	keepTheseDependencies := make(map[uint64]FileDependency)
	for _, v := range *flattened {
		keepTheseDependencies[v.ID] = data.Deps.ChunkIDToDependencies[v.ID]
	}
	data.Deps.ChunkIDToDependencies = keepTheseDependencies
	// next up, the dependency file
	depFile := data.Deps.Deparse()
	depFileLength := uint64(len(*depFile))
	depFileHash := *sha1Hash(depFile)

	depFileBlocks := []FIoStoreTocCompressedBlockEntry{}
	// now perform compression, write per compressed block
	depFileData := *depFile
	// now perform compression, write per compressed block
	for len(depFileData) != 0 {
		var chunk []byte
		var block FIoStoreTocCompressedBlockEntry
		chunkLen := len(depFileData)
		if chunkLen > CompSize {
			chunkLen = CompSize
		}
		chunk = depFileData[:chunkLen]
		block.CompressionMethod = 1 // for zlib compression
		currOffset, _ := f.Seek(0, os.SEEK_CUR)
		block.SetOffset(uint64(currOffset))
		block.SetUncompressedSize(uint32(chunkLen))

		// actual compression
		// This may not be 100% correct;
		// comparing to the original ucas files, the zlib compression size differs slightly.
		// the original files seem to always be an even number of bytes long
		var b bytes.Buffer
		w := zlib.NewWriter(&b)
		w.Write(chunk)
		w.Close()
		compressedChunk := b.Bytes()
		block.SetCompressedSize(uint32(len(compressedChunk)))
		// align compessedChunk to 0x10
		compressedChunk = append(compressedChunk, getRandomBytes((0x10-(len(compressedChunk)%0x10))&(0x10-1))...)
		depFileData = depFileData[chunkLen:]
		depFileBlocks = append(depFileBlocks, block)

		f.Write(compressedChunk)
	}
	// ucas file has been fully written
	f.Close()

	var chid FIoChunkID
	var offlen FIoOffsetAndLength
	var meta FIoStoreTocEntryMeta

	for i, entry := range *flattened {
		chid.ID = entry.ID
		chid.Type = entry.chunkType

		if i == 0 {
			offlen.SetOffset(0)
		} else {
			offset := data.Offlengths[i-1].GetOffset() + data.Offlengths[i-1].GetLength()
			offset = ((offset + CompSize - 1) / CompSize) * CompSize // align to 0x1000
			offlen.SetOffset(offset)

		}
		// offlen.SetOffset(entry.offset)
		offlen.SetLength(entry.length)
		meta.ChunkHash = entry.FileHash
		meta.Flags = 1

		data.IDs = append(data.IDs, chid)
		data.Offlengths = append(data.Offlengths, offlen)
		data.CBlocks = append(data.CBlocks, entry.blocks...)
		data.Metas = append(data.Metas, meta)
	}
	// add data for the dependencies
	chid.ID = data.Deps.ThisPackageID
	chid.Type = 10 // special type!
	offset := data.Offlengths[len(data.Offlengths)-1].GetOffset() + data.Offlengths[len(data.Offlengths)-1].GetLength()
	offset = ((offset + CompSize - 1) / CompSize) * CompSize // align to 0x1000
	offlen.SetOffset(offset)
	offlen.SetLength(depFileLength)
	meta.ChunkHash = depFileHash
	meta.Flags = 1

	data.IDs = append(data.IDs, chid)
	data.Offlengths = append(data.Offlengths, offlen)
	data.Metas = append(data.Metas, meta)

	data.CBlocks = append(data.CBlocks, depFileBlocks...)

	// all data is present, now ready for writing UTOC file
	f, _ = os.OpenFile(utocPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	data.writeUtocFile(f, dirInfo.Root)
	f.Close()

}

// unpacks the directory inside the unpackPath; does NOT include the entire path to the root
func (d *Directory) UnpackDirectory(openUCASfile *os.File, unpackPath string) {
	os.MkdirAll(unpackPath, 0700)
	for _, f := range d.Files {
		b, err := f.decompress(openUCASfile)
		if err != nil {
			fmt.Println("err:", err)
		}
		err = os.WriteFile(unpackPath+"/"+f.Name, *b, 0644)
		if err != nil {
			fmt.Println("err:", err)
		}
	}
	for _, dir := range d.ChildDirs {
		// create child directory and unpack there
		dir.UnpackDirectory(openUCASfile, unpackPath+"/"+dir.Name)
	}
}

// traverses the path starting from the mountpoint (../../../) and returns the directory pointer
func (ct *CasToc) PathToDirectory(path string) (*Directory, error) {
	path = strings.TrimPrefix(path, MountPoint)
	pathDirs := strings.Split(path, "/")

	currDir := ct.Root
	for _, dirName := range pathDirs {
		if dirName == "" {
			continue
		}
		found := false
		for _, child := range currDir.ChildDirs {
			if child.Name == dirName {
				currDir = child
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("could not find this path")
		}
	}

	return currDir, nil
}

func ParseUTocFile(filepath string) (ct CasToc, err error) {
	filepath = strings.TrimSuffix(filepath, ".utoc")
	filepath = strings.TrimSuffix(filepath, ".ucas")
	dat := UTocData{}
	dat.FNameToID = make(map[string]uint64)
	// read utoc file to be parsed
	b, err := ioutil.ReadFile((filepath + ".utoc"))
	if err != nil {
		return ct, err
	}
	r := bytes.NewReader(b)

	// read the header of the .utoc file
	var hdr UTocHeader
	err = binary.Read(r, binary.LittleEndian, &hdr)
	if err != nil {
		return ct, err
	}
	if string(hdr.Magic[:]) != MagicUtoc {
		return ct, ErrWrongMagic
	}
	if hdr.Version < VersionUtoc {
		fmt.Println("warning: this version is not supported by the packer/unpacker and may not work as intended")
	}
	if hdr.Version != VersionUtoc && hdr.Version != LegacyUtoc {
		return ct, ErrUnknownUtocVersion
	}
	if hdr.CompressedBlockEntrySize != 12 { // must be sizeof FIoStoreTocCompressedBlockEntry
		return ct, ErrUnknownUtocVersion
	}
	ct.ContainerID = uint64(hdr.ContainerID)

	dat.BlockSize = hdr.CompressionBlockSize

	// read Chunk IDs
	var chunkID FIoChunkID
	for i := 0; i < int(hdr.EntryCount); i++ {
		binary.Read(r, binary.LittleEndian, &chunkID)
		dat.IDs = append(dat.IDs, chunkID)
	}

	// read offlengths
	var offlen FIoOffsetAndLength
	for i := 0; i < int(hdr.EntryCount); i++ {
		binary.Read(r, binary.LittleEndian, &offlen)
		dat.Offlengths = append(dat.Offlengths, offlen)
	}

	// read compression blocks
	var cBlock FIoStoreTocCompressedBlockEntry
	for i := 0; i < int(hdr.CompressedBlockEntryCount); i++ {
		binary.Read(r, binary.LittleEndian, &cBlock)
		dat.CBlocks = append(dat.CBlocks, cBlock)
	}

	// compression methods
	var compressionMethods []string
	compressionMethods = append(compressionMethods, "None")

	method := make([]byte, hdr.CompressionMethodNameLength)
	for i := 0; i < int(hdr.CompressionMethodNameCount); i++ {
		binary.Read(r, binary.LittleEndian, &method)
		compressionMethods = append(compressionMethods, string(bytes.Trim([]byte(method[:]), "\x00")))
	}
	// chunk block signatures;
	indexedFlag := uint8(1 << 3)
	// compressedFlag := uint8(1 << 0)
	if hdr.ContainerFlags != 0 && uint8(hdr.ContainerFlags)&indexedFlag == 0 {
		// the reference project may contain flags here, but no idea what it should do...
		err = ErrContainerFlag
		return
	}

	// define dependency index "file" and parse it to use in the directoryIndex
	if hdr.Version != VersionUtoc {
		fmt.Println("There is a problem with reading the dependency file, please contact menv at the Grounded Modding server on Discord!")
		return
	}

	depfileIndex := len(dat.IDs) - 1 // In VersionUtoc, this is the last chunk
	depfileBlocks := dat.CBlocks[dat.Offlengths[depfileIndex].GetOffset()/uint64(dat.BlockSize):]

	depfile := FileInfo{
		Name:      "dependency file",
		ID:        dat.IDs[depfileIndex].ID,
		chunkType: dat.IDs[depfileIndex].Type,
		offset:    dat.Offlengths[depfileIndex].GetOffset(),
		length:    dat.Offlengths[depfileIndex].GetLength(),
		blocks:    depfileBlocks,
	}
	f, err := os.Open(filepath + ".ucas")
	if err != nil {
		return
	}
	defer f.Close()
	data, err := depfile.decompress(f)
	if err != nil {
		return
	}
	dat.Deps = ParseDependencies(*data)
	ct.Root = parseDirectoryIndex(r, &dat)
	// Chunk Metas
	// These metas are in the utoc file, but they aren't really used for unpacking
	var meta FIoStoreTocEntryMeta
	metas := []FIoStoreTocEntryMeta{}

	for i := 0; i < int(hdr.EntryCount); i++ {
		binary.Read(r, binary.LittleEndian, &meta)
		metas = append(metas, meta)
	}

	// save additional data
	saveParsedData(&dat)
	return
}
